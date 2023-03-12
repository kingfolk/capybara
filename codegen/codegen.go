package codegen

/*
#include <stdio.h>

int f1() {
	return 135;
}
*/
import "C"

import (
	"fmt"
	"strconv"
	"unsafe"

	"github.com/kingfolk/capybara/ir"
	"github.com/kingfolk/capybara/types"

	"github.com/llvm/llvm-project/bindings/go/llvm"
)

var ff1 unsafe.Pointer = unsafe.Pointer(C.f1)

// var ff2 unsafe.Pointer = unsafe.Pointer(C.testFunc)

type phiContext struct {
	v   llvm.Value
	ins *ir.Phi
	blk *ir.Block
}

type buildContext struct {
	curBlk     *ir.Block
	blkMap     map[int]llvm.BasicBlock
	phiPending []phiContext
	retVal     llvm.Value
	retBlk     llvm.BasicBlock
}

type blockBuilder struct {
	debug     bool
	env       *types.Env
	builder   llvm.Builder
	registers map[string]llvm.Value
	buildCtx  *buildContext
}

type ExtGlobal struct {
	Name string
	Tp   types.ValType
	// eleTp 所有的global对象都是指针，这里的eleTp为指针指向的类型
	EleTp llvm.Type
	Data  unsafe.Pointer
	Reg   llvm.Value
}

var context = llvm.GlobalContext()
var rootModule = llvm.NewModule("root")
var rootFuncPassMgr = llvm.NewFunctionPassManagerForModule(rootModule)
var globalTable = map[string]llvm.Value{}
var targetData llvm.TargetData

func GetRootModule() llvm.Module {
	return rootModule
}

func BuildFunc(fn *ir.Func, debug bool, globals ...*ExtGlobal) llvm.Value {
	builder := newBlockBuilder(&types.Env{DeclTable: fn.DeclTable}, debug)
	for _, global := range globals {
		builder.registers[global.Name] = global.Reg
	}
	return builder.buildFunc(fn.Body.Name, fn)
}

func BuildModule(mod *ir.Module, debug bool) llvm.Value {
	for _, fn := range mod.Funcs {
		builder := newBlockBuilder(&types.Env{DeclTable: fn.DeclTable}, debug)
		builder.buildFunc(fn.Body.Name, fn)
	}
	rootFnTp := &types.Func{
		Ret: mod.Root.Ins[len(mod.Root.Ins)-1].Type(),
	}
	// TODO 这一块逻辑非常生硬。mod.Funcs需挪出来事先编译，然后在对Root里的Func指令删除之后编译。
	// 这样做是因为混在一块编译，会出现llvm里函数套函数的情况，最后codegen不成功。
	// 需要良好的工程化来改善
	var rootBlk = *mod.Root
	rootBlk.Ins = make([]*ir.Instr, 0)
	for _, i := range mod.Root.Ins {
		if _, ok := i.Val.(*ir.Func); !ok {
			rootBlk.Ins = append(rootBlk.Ins, i)
		}
	}
	rootFn := &ir.Func{
		Body:      &rootBlk,
		Tp:        rootFnTp,
		DeclTable: mod.Env.DeclTable,
	}
	builder := newBlockBuilder(mod.Env, debug)
	return builder.buildFunc("root_anon", rootFn)
}

func RunJit(val llvm.Value, globals []*ExtGlobal, args ...llvm.GenericValue) llvm.GenericValue {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()
	// var execEngine, err = llvm.NewExecutionEngine(rootModule)
	// https://stackoverflow.com/questions/5988444/why-is-the-llvm-execution-engine-faster-than-compiled-code
	// llvm execute engine 可能会比编译到文件然后执行还快
	options := llvm.NewMCJITCompilerOptions()
	options.SetMCJITOptimizationLevel(2)
	execEngine, err := llvm.NewMCJITCompiler(rootModule, options)
	if err != nil {
		panic(err)
	}

	for _, global := range globals {
		execEngine.AddGlobalMapping(global.Reg, global.Data)
	}

	return execEngine.RunFunction(val, args)
}

func newBlockBuilder(env *types.Env, debug bool) *blockBuilder {
	return &blockBuilder{
		debug:     debug,
		env:       env,
		builder:   context.NewBuilder(),
		registers: map[string]llvm.Value{},
		buildCtx:  &buildContext{blkMap: make(map[int]llvm.BasicBlock)},
	}
}

var (
	boolT llvm.Type = context.Int1Type()
	intT  llvm.Type = context.Int32Type()
	// TODO FLOAT?
	floatT llvm.Type = context.DoubleType()
)

type OptLevel int

const (
	// OptimizeNone is equivalent to -O0
	OptimizeNone OptLevel = iota
	// OptimizeLess is equivalent to -O1
	OptimizeLess
	// OptimizeDefault is equivalent to -O2
	OptimizeDefault
	// OptimizeAggressive is equivalent to -O3
	OptimizeAggressive
)

// RunOptimizationPasses passes optimizations on generated LLVM IR module following specified optimization level.
func RunOptimizationPasses(opt OptLevel) {
	if opt == OptimizeNone {
		return
	}
	level := int(opt)

	builder := llvm.NewPassManagerBuilder()
	defer builder.Dispose()
	builder.SetOptLevel(level)

	// Threshold magic numbers came from computeThresholdFromOptLevels() in llvm/lib/Analysis/InlineCost.cpp
	threshold := uint(225) // O2
	if opt == OptimizeAggressive {
		// -O1 is the same inline level as -O2
		threshold = 275
	}
	builder.UseInlinerWithThreshold(threshold)

	funcPasses := llvm.NewFunctionPassManagerForModule(rootModule)
	defer funcPasses.Dispose()
	builder.PopulateFunc(funcPasses)
	for fun := rootModule.FirstFunction(); fun.C != nil; fun = llvm.NextFunction(fun) {
		if fun.IsDeclaration() {
			continue
		}
		funcPasses.InitializeFunc()
		funcPasses.RunFunc(fun)
		funcPasses.FinalizeFunc()
	}

	modPasses := llvm.NewPassManager()
	defer modPasses.Dispose()
	builder.Populate(modPasses)
	modPasses.Run(rootModule)
}

func (b *blockBuilder) build(block *ir.Block) llvm.Value {
	res := b.buildBlock(block)
	return res
}

func (b *blockBuilder) finalizePhi() {
	for _, phi := range b.buildCtx.phiPending {
		var edgeVars []llvm.Value
		var edgeBlks []llvm.BasicBlock
		for i, edge := range phi.ins.Edges {
			if ir.IsDangle(edge) {
				continue
			}
			edgeVars = append(edgeVars, b.registers[edge])
			src := phi.blk.Src[i]
			edgeBlks = append(edgeBlks, b.buildCtx.blkMap[src.Id])
		}
		phi.v.AddIncoming(edgeVars, edgeBlks)
	}
}

func createEntryBlockAlloca(f llvm.Value, tp llvm.Type, name string) llvm.Value {
	var tmpB = llvm.NewBuilder()
	tmpB.SetInsertPoint(f.EntryBasicBlock(), f.EntryBasicBlock().FirstInstruction())
	return tmpB.CreateAlloca(tp, name)
}

func (b *blockBuilder) buildFunc(name string, f *ir.Func) llvm.Value {
	tpDef := f.Type()
	funcType := buildFuncType(tpDef.(*types.Func))
	theFunction := llvm.AddFunction(rootModule, name, funcType)

	if theFunction.IsNil() {
		panic("theFunction.IsNil")
	}

	entry := llvm.AddBasicBlock(theFunction, "entry")
	b.builder.SetInsertPointAtEnd(entry)
	b.buildCtx.blkMap[f.Body.Id] = entry

	args := theFunction.Params()
	for i, paramName := range f.Params {
		args[i].SetName(paramName)
		argType := buildType(b.env.DeclTable[paramName])
		alloca := createEntryBlockAlloca(theFunction, argType, paramName)
		b.builder.CreateStore(args[i], alloca)
		b.registers[paramName] = args[i]
	}

	b.buildBlock(f.Body)
	retVal := b.buildCtx.retVal

	if retVal.IsNil() {
		theFunction.EraseFromParentAsFunction()
		panic("retVal.IsNil")
	}

	b.builder.SetInsertPointAtEnd(b.buildCtx.retBlk)
	b.builder.CreateRet(retVal)

	b.finalizePhi()

	if b.debug {
		fmt.Println("--- theFunction.Dump()")
		theFunction.Dump()
		fmt.Println("--- theFunction.Dump() done")
	}
	if llvm.VerifyFunction(theFunction, llvm.PrintMessageAction) != nil {
		theFunction.EraseFromParentAsFunction()
		panic("function verifiction failed")
	}

	rootFuncPassMgr.RunFunc(theFunction)
	return theFunction
}

func (b *blockBuilder) buildArrMake(ident string, am *ir.ArrMake) llvm.Value {
	t, ok := b.env.DeclTable[ident]
	if !ok {
		panic("ident not found: " + ident)
	}
	elemTy := buildType(t.(*types.Arr).Ele)
	sizeVal := llvm.ConstInt(intT, uint64(len(am.Args)), false /*signed*/)
	alloca := b.builder.CreateArrayAlloca(elemTy, sizeVal, ident)

	for i, elem := range am.Args {
		elemVal := b.resolve(elem)
		al := b.builder.CreateInBoundsGEP(alloca, []llvm.Value{llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}, "tmparr")
		b.builder.CreateStore(elemVal, al)
	}
	return alloca
}

func (b *blockBuilder) buildArrGet(ident string, ag *ir.ArrGet) llvm.Value {
	arrVal := b.resolve(ag.Arr)
	indexVal := b.resolve(ag.Index)
	elemPtr := b.builder.CreateInBoundsGEP(arrVal, []llvm.Value{indexVal}, "")
	return b.builder.CreateLoad(elemPtr, "arrload")
}

func (b *blockBuilder) buildArrPut(ident string, ap *ir.ArrPut) llvm.Value {
	arrVal := b.resolve(ap.Arr)
	indexVal := b.resolve(ap.Index)
	rightVal := b.resolve(ap.Right)
	elemPtr := b.builder.CreateInBoundsGEP(arrVal, []llvm.Value{indexVal}, "")
	return b.builder.CreateStore(rightVal, elemPtr)
}

func (b *blockBuilder) buildCall(c *ir.Call) llvm.Value {
	args := make([]llvm.Value, len(c.Args))
	for i, arg := range c.Args {
		args[i] = b.resolve(arg)
	}

	f := rootModule.NamedFunction(c.Name)
	if f.C == nil {
		panic("function " + c.Name + " not found in llvm module")
	}
	ret := b.builder.CreateCall(f, args, "")
	return ret
}

func (b *blockBuilder) buildPhi(it *ir.Phi) llvm.Value {
	tp := buildType(it.Tp)
	phiVal := b.builder.CreatePHI(tp, it.Orig)
	b.buildCtx.phiPending = append(b.buildCtx.phiPending, phiContext{
		v:   phiVal,
		ins: it,
		blk: b.buildCtx.curBlk,
	})
	return phiVal
}

func (b *blockBuilder) buildIf(ident string, it *ir.If) llvm.Value {
	cond := b.resolve(it.Cond)
	if cond.IsNil() {
		panic("cond is nil")
	}

	parentFunc := b.builder.GetInsertBlock().Parent()
	thenBlk := llvm.AddBasicBlock(parentFunc, "then")
	elseBlk := llvm.AddBasicBlock(parentFunc, "else")
	b.builder.CreateCondBr(cond, thenBlk, elseBlk)
	b.buildCtx.blkMap[it.Then.Id] = thenBlk
	b.buildCtx.blkMap[it.Else.Id] = elseBlk

	// generate 'then' block
	b.builder.SetInsertPointAtEnd(thenBlk)
	thenv := b.buildBlock(it.Then)
	if thenv.IsNil() {
		panic("then is nil")
	}

	// generate 'else' block
	// C++ unknown eq: TheFunction->getBasicBlockList().push_back(ElseBB);
	b.builder.SetInsertPointAtEnd(elseBlk)
	elsev := b.buildBlock(it.Else)
	if elsev.IsNil() {
		panic("else is nil")
	}

	if !ir.IsDangle(ident) {
		if b.typeOf(ident) == nil {
			return llvm.ConstNull(intT)
		}
		tp := buildType(b.typeOf(ident))
		// 创建的phi所在的llvm block在上面buildBlock即已挂当前builder
		bb := b.builder.GetInsertBlock()
		b.builder.SetInsertPointBefore(bb.FirstInstruction())
		PhiNode := b.builder.CreatePHI(tp, "ifmerge")
		PhiNode.AddIncoming([]llvm.Value{thenv, elsev}, []llvm.BasicBlock{thenBlk, elseBlk})
		b.builder.SetInsertPointAtEnd(bb)
		return PhiNode
	}
	return llvm.ConstNull(intT)
}

func buildType(tp types.ValType) llvm.Type {
	switch tp.Code() {
	case types.TpBool:
		return boolT
	case types.TpInt:
		return intT
	case types.TpFloat:
		return floatT
	case types.TpArr:
		arrTp := tp.(*types.Arr)
		return context.StructType([]llvm.Type{
			llvm.PointerType(buildType(arrTp.Ele), 0),
			intT,
		}, false)
	case types.TpFunc:
		return buildFuncType(tp.(*types.Func))
	default:
		panic("unsupported type: " + tp.String())
	}
}

func (b *blockBuilder) typeOf(ident string) types.ValType {
	if t, ok := b.env.DeclTable[ident]; ok {
		return t
	}
	panic("Type was not found for ident: " + ident)
}

func buildFuncType(tp *types.Func) llvm.Type {
	params := make([]llvm.Type, len(tp.Params))
	for i, param := range tp.Params {
		params[i] = buildType(param)
	}
	return llvm.FunctionType(buildType(tp.Ret), params, false)
}

func (b *blockBuilder) resolve(ident string) llvm.Value {
	if reg, ok := globalTable[ident]; ok {
		return reg
	}
	if reg, ok := b.registers[ident]; ok {
		return reg
	}
	panic("No value was found for identifier: " + ident)
}

func (b *blockBuilder) buildBlock(block *ir.Block) llvm.Value {
	b.buildCtx.curBlk = block

	var v llvm.Value
	for _, i := range block.Ins {
		v = b.buildInsn(i)
	}
	buildBlk := b.builder.GetInsertBlock()
	last := block.Ins[len(block.Ins)-1]
	if _, ok := last.Val.(*ir.If); !ok && len(block.Dest) > 0 {
		dest := block.Dest[0]
		destBlk, visited := b.buildCtx.blkMap[dest.Id]
		if !visited {
			parentFunc := b.builder.GetInsertBlock().Parent()
			destBlk := llvm.AddBasicBlock(parentFunc, dest.Name)
			b.builder.CreateBr(destBlk)
			b.builder.SetInsertPointAtEnd(destBlk)
			b.buildCtx.blkMap[dest.Id] = destBlk
			b.buildBlock(dest)
		} else {
			b.builder.CreateBr(destBlk)
			b.builder.SetInsertPointAtEnd(destBlk)
		}
	}
	// TODO DOC 无dest，则为最后一个block
	if len(block.Dest) == 0 {
		b.buildCtx.retVal = v
		b.buildCtx.retBlk = buildBlk
	}
	return v
}

func (b *blockBuilder) buildVal(ident string, v ir.Val) llvm.Value {
	switch expr := v.(type) {
	case *ir.Ref:
		reg, ok := b.registers[expr.Ident]
		if !ok {
			panic("Value not found for ref: " + expr.Ident)
		}
		return reg
	case *ir.Const:
		switch expr.Type() {
		case types.Unit:
			return llvm.ConstNull(intT)
		case types.Int:
			ival, err := strconv.ParseInt(string(expr.Raw()), 10, 64)
			if err != nil {
				panic(err)
			}
			return llvm.ConstInt(intT, uint64(ival), true)
		case types.Float:
			fval, err := strconv.ParseFloat(string(expr.Raw()), 64)
			if err != nil {
				panic(err)
			}
			return llvm.ConstFloat(floatT, fval)
		default:
			panic("unsupported")
		}
	case *ir.Expr:
		regs := make([]llvm.Value, len(expr.Args))
		for i, arg := range expr.Args {
			regs[i] = b.resolve(arg)
		}
		switch expr.Op {
		case ir.ADD:
			switch expr.Type() {
			case types.Int:
				return b.builder.CreateAdd(regs[0], regs[1], "add")
			case types.Float:
				return b.builder.CreateFAdd(regs[0], regs[1], "fadd")
			}
		case ir.SUB:
			switch expr.Type() {
			case types.Int:
				return b.builder.CreateSub(regs[0], regs[1], "sub")
			case types.Float:
				return b.builder.CreateFSub(regs[0], regs[1], "fsub")
			}
		case ir.MUL:
			switch expr.Type() {
			case types.Int:
				return b.builder.CreateMul(regs[0], regs[1], "mul")
			case types.Float:
				return b.builder.CreateFMul(regs[0], regs[1], "fmul")
			}
		case ir.DIV:
			switch expr.Type() {
			case types.Float:
				return b.builder.CreateFDiv(regs[0], regs[1], "fdiv")
			}
		case ir.EQ:
			argTp := b.typeOf(expr.Args[0])
			switch argTp {
			case types.Int:
				return b.builder.CreateICmp(llvm.IntEQ, regs[0], regs[1], "==")
			case types.Float:
				return b.builder.CreateFCmp(llvm.FloatOEQ, regs[0], regs[1], "==")
			}
		case ir.GT:
			argTp := b.typeOf(expr.Args[0])
			switch argTp {
			case types.Int:
				return b.builder.CreateICmp(llvm.IntSGT, regs[0], regs[1], ">")
			case types.Float:
				return b.builder.CreateFCmp(llvm.FloatOGT, regs[0], regs[1], ">")
			}
		case ir.GTE:
			argTp := b.typeOf(expr.Args[0])
			switch argTp {
			case types.Int:
				return b.builder.CreateICmp(llvm.IntSGE, regs[0], regs[1], ">=")
			case types.Float:
				return b.builder.CreateFCmp(llvm.FloatOGE, regs[0], regs[1], ">=")
			}
		case ir.LT:
			argTp := b.typeOf(expr.Args[0])
			switch argTp {
			case types.Int:
				return b.builder.CreateICmp(llvm.IntSLT, regs[0], regs[1], "<")
			case types.Float:
				return b.builder.CreateFCmp(llvm.FloatOLT, regs[0], regs[1], "<")
			}
		case ir.LTE:
			argTp := b.typeOf(expr.Args[0])
			switch argTp {
			case types.Int:
				return b.builder.CreateICmp(llvm.IntSLE, regs[0], regs[1], "<")
			case types.Float:
				return b.builder.CreateFCmp(llvm.FloatOLE, regs[0], regs[1], "<")
			}
		case ir.AND:
			return b.builder.CreateAnd(regs[0], regs[1], "&&")
		case ir.OR:
			return b.builder.CreateOr(regs[0], regs[1], "||")
		}
	case *ir.Block:
		return b.buildBlock(expr)
	case *ir.If:
		// TODO: a = if ... then to if {}
		return b.buildIf(ident, expr)
	case *ir.ArrMake:
		return b.buildArrMake(ident, expr)
	case *ir.ArrGet:
		return b.buildArrGet(ident, expr)
	case *ir.ArrPut:
		return b.buildArrPut(ident, expr)
	case *ir.Func:
		// TODO: i.Ident应该是需要隐藏的，函数名应该放在ir.Fun里
		// TODO 到底用ident还是expr.Body.Name
		return b.buildFunc(expr.Body.Name, expr)
	case *ir.Call:
		return b.buildCall(expr)
	case *ir.Phi:
		return b.buildPhi(expr)
	}
	panic(fmt.Sprintf("unsupported val: %s", v.String()))
}

func (b *blockBuilder) buildInsn(insn *ir.Instr) llvm.Value {
	v := b.buildVal(insn.Ident, insn.Val)
	b.registers[insn.Ident] = v
	return v
}
