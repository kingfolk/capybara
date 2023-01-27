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

type blockBuilder struct {
	env       *types.Env
	builder   llvm.Builder
	registers map[string]llvm.Value
}

var context = llvm.GlobalContext()
var rootModule = llvm.NewModule("root")
var rootFuncPassMgr = llvm.NewFunctionPassManagerForModule(rootModule)
var globalTable = map[string]llvm.Value{}
var targetData llvm.TargetData

func newBlockBuilder(env *types.Env) *blockBuilder {
	return &blockBuilder{
		env:       env,
		builder:   context.NewBuilder(),
		registers: map[string]llvm.Value{},
	}
}

var (
	boolT  llvm.Type = context.Int1Type()
	intT   llvm.Type = context.Int32Type()
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
	return b.buildBlock(block)
}

func (b *blockBuilder) buildAndRun(block *ir.Block) llvm.GenericValue {
	lastIt := block.Ins[len(block.Ins)-1]
	block.Ins = block.Ins[:len(block.Ins)-1]
	b.buildBlock(block)
	return b.RunAnonVal(lastIt, []llvm.GenericValue{})
}

func (b *blockBuilder) buildAndRunFunc(block *ir.Block, args []llvm.GenericValue, globals []*ExtGlobal) llvm.GenericValue {
	if len(block.Ins) > 1 {
		panic("not right")
	}
	f := b.BuildFunc(block.Ins[0], globals)
	return b.RunFunc(f, args, globals)
}

func (b *blockBuilder) buildGlobal(ins *ir.Instr) llvm.Value {
	tpDef := b.env.DeclTable[ins.Ident]
	tp := buildType(tpDef)
	ptr := llvm.AddGlobal(rootModule, tp, ins.Ident)
	right := b.buildInsn(ins)
	b.builder.CreateStore(right, ptr)
	return right
}

func createEntryBlockAlloca(f llvm.Value, tp llvm.Type, name string) llvm.Value {
	var tmpB = llvm.NewBuilder()
	tmpB.SetInsertPoint(f.EntryBasicBlock(), f.EntryBasicBlock().FirstInstruction())
	return tmpB.CreateAlloca(tp, name)
}

func (b *blockBuilder) buildFunc(name string, f *ir.Fun) llvm.Value {
	tpDef := f.Type()
	// fmt.Println("-- b.env", b.env, name)
	funcType := buildFuncType(tpDef.(*types.Fun))
	// fmt.Println("-- funcType.String()", funcType.String())
	theFunction := llvm.AddFunction(rootModule, name, funcType)

	if theFunction.IsNil() {
		panic("theFunction.IsNil")
	}

	entry := llvm.AddBasicBlock(theFunction, "entry")
	b.builder.SetInsertPointAtEnd(entry)

	args := theFunction.Params()
	for i, paramName := range f.Params {
		args[i].SetName(paramName)
		argType := buildType(b.env.DeclTable[paramName])
		alloca := createEntryBlockAlloca(theFunction, argType, paramName)
		b.builder.CreateStore(args[i], alloca)
		b.registers[paramName] = args[i]
	}

	retVal := b.buildBlock(f.Body)
	if retVal.IsNil() {
		theFunction.EraseFromParentAsFunction()
		panic("retVal.IsNil")
	}

	b.builder.CreateRet(retVal)
	fmt.Println("--- theFunction.Dump()")
	theFunction.Dump()
	fmt.Println("--- theFunction.Dump() done")
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

	f := b.resolve(c.Name)
	fmt.Println("~~~ f, args", c.Name, f, args, f.Type())
	ret := b.builder.CreateCall(f, args, "")
	return ret
}

func (b *blockBuilder) buildIf(ident string, it *ir.If) llvm.Value {
	cond := b.resolve(it.Cond)
	if cond.IsNil() {
		panic("cond is nil")
	}

	parentFunc := b.builder.GetInsertBlock().Parent()
	thenBlk := llvm.AddBasicBlock(parentFunc, "then")
	elseBlk := llvm.AddBasicBlock(parentFunc, "else")
	mergeBlk := llvm.AddBasicBlock(parentFunc, "merge")
	b.builder.CreateCondBr(cond, thenBlk, elseBlk)

	// generate 'then' block
	b.builder.SetInsertPointAtEnd(thenBlk)
	thenv := b.buildBlock(it.Then)
	if thenv.IsNil() {
		panic("then is nil")
	}
	b.builder.CreateBr(mergeBlk)
	// Codegen of 'Then' can change the current block, update ThenBB for the PHI.
	thenBlk = b.builder.GetInsertBlock()

	// generate 'else' block
	// C++ unknown eq: TheFunction->getBasicBlockList().push_back(ElseBB);
	b.builder.SetInsertPointAtEnd(elseBlk)
	elsev := b.buildBlock(it.Else)
	if elsev.IsNil() {
		panic("else is nil")
	}
	b.builder.CreateBr(mergeBlk)
	elseBlk = b.builder.GetInsertBlock()

	b.builder.SetInsertPointAtEnd(mergeBlk)
	if b.typeOf(ident) == nil {
		return llvm.ConstNull(intT)
	}
	tp := buildType(b.typeOf(ident))
	PhiNode := b.builder.CreatePHI(tp, "iftmp")
	PhiNode.AddIncoming([]llvm.Value{thenv, elsev}, []llvm.BasicBlock{thenBlk, elseBlk})
	return PhiNode
}

func (b *blockBuilder) buildLoop(ident string, it *ir.Loop) llvm.Value {
	start := b.resolve(it.From)
	cond := b.resolve(it.To)
	if start.IsNil() {
		panic("start is nil")
	}
	if cond.IsNil() {
		panic("cond is nil")
	}

	parentFunc := b.builder.GetInsertBlock().Parent()
	preheaderBlk := b.builder.GetInsertBlock()
	loopBlk := llvm.AddBasicBlock(parentFunc, "loop")
	afterBlk := llvm.AddBasicBlock(parentFunc, "after")

	b.builder.CreateBr(loopBlk)
	b.builder.SetInsertPointAtEnd(loopBlk)

	tp := buildType(b.typeOf(it.From))
	phiVar := b.builder.CreatePHI(tp, "")
	phiVar.AddIncoming([]llvm.Value{start}, []llvm.BasicBlock{preheaderBlk})
	b.registers[it.ItIdent] = phiVar

	b.buildBlock(it.Body)

	nextIt := b.builder.CreateAdd(phiVar, llvm.ConstInt(intT, uint64(1), true), "step")
	loopEndBlk := b.builder.GetInsertBlock()

	endCond := b.builder.CreateICmp(llvm.IntSLT, phiVar, cond, "<")
	b.builder.CreateCondBr(endCond, loopBlk, afterBlk)

	b.builder.SetInsertPointAtEnd(afterBlk)
	phiVar.AddIncoming([]llvm.Value{nextIt}, []llvm.BasicBlock{loopEndBlk})

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
	case types.TpFun:
		return buildFuncType(tp.(*types.Fun))
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

func buildFuncType(tp *types.Fun) llvm.Type {
	params := make([]llvm.Type, len(tp.Params))
	for i, param := range tp.Params {
		params[i] = buildType(param)
	}
	return llvm.FunctionType(buildType(tp.Ret), params, false)
}

func (b *blockBuilder) resolve(ident string) llvm.Value {
	fmt.Println("<<< resolve", globalTable, ident)
	if reg, ok := globalTable[ident]; ok {
		return reg
	}
	if reg, ok := b.registers[ident]; ok {
		return reg
	}
	panic("No value was found for identifier: " + ident)
}

func (b *blockBuilder) buildBlock(block *ir.Block) llvm.Value {
	var v llvm.Value
	for _, i := range block.Ins {
		// fmt.Println("!!!!!!!!! buildInsn", i)
		v = b.buildInsn(i)
	}
	return v
}

func (b *blockBuilder) buildVal(ident string, v ir.Val) llvm.Value {
	switch expr := v.(type) {
	case *ir.Ref:
		reg, ok := b.registers[expr.Ident]
		if !ok {
			fmt.Println("!! ident", ident)
			fmt.Println("  !! expr.Ident", expr.Ident)
			panic("Value not found for ref: " + expr.Ident)
		}
		return reg
	case *ir.Const:
		switch expr.Type() {
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
		// TODO: a = if ... then
		return b.buildIf(ident, expr)
	case *ir.Loop:
		return b.buildLoop(ident, expr)
	case *ir.ArrMake:
		return b.buildArrMake(ident, expr)
	case *ir.ArrGet:
		return b.buildArrGet(ident, expr)
	case *ir.ArrPut:
		return b.buildArrPut(ident, expr)
	case *ir.Fun:
		// TODO: i.Ident应该是需要隐藏的，函数名应该放在ir.Fun里
		return b.buildFunc(ident, expr)
	case *ir.Call:
		return b.buildCall(expr)
	}
	panic(fmt.Sprintf("unsupported val: %s", v.String()))
}

func (b *blockBuilder) buildInsn(insn *ir.Instr) llvm.Value {
	v := b.buildVal(insn.Ident, insn.Val)
	b.registers[insn.Ident] = v
	return v
}

func (b *blockBuilder) RunAnonVal(it *ir.Instr, args []llvm.GenericValue) llvm.GenericValue {
	name := "anon"
	fmt.Println("***** val.Type()", it.Type())
	itTp := buildType(it.Type())
	fmt.Println("***** itTp", itTp)
	funcTp := llvm.FunctionType(itTp, []llvm.Type{}, false)
	fmt.Println("***** funcTp", funcTp)
	theFunction := llvm.AddFunction(rootModule, name, funcTp)
	fmt.Println("***** theFunction", theFunction)

	if theFunction.IsNil() {
		panic("theFunction.IsNil")
	}

	entry := llvm.AddBasicBlock(theFunction, "entry")
	b.builder.SetInsertPointAtEnd(entry)

	retVal := b.buildInsn(it)
	if retVal.IsNil() {
		theFunction.EraseFromParentAsFunction()
		panic("retVal.IsNil")
	}

	b.builder.CreateRet(retVal)
	if llvm.VerifyFunction(theFunction, llvm.PrintMessageAction) != nil {
		theFunction.EraseFromParentAsFunction()
		panic("function verifiction failed")
	}

	rootFuncPassMgr.RunFunc(theFunction)

	// var execEngine, err = llvm.NewExecutionEngine(rootModule)
	// https://stackoverflow.com/questions/5988444/why-is-the-llvm-execution-engine-faster-than-compiled-code
	// llvm execute engine 可能会比编译到文件然后执行还快
	options := llvm.NewMCJITCompilerOptions()
	options.SetMCJITOptimizationLevel(2)
	execEngine, err := llvm.NewMCJITCompiler(rootModule, options)
	if err != nil {
		panic(err)
	}

	fmt.Println("***** start run")
	return execEngine.RunFunction(theFunction, args)
}

type ExtGlobal struct {
	name string
	// eleTp 所有的global对象都是指针，这里的eleTp为指针指向的类型
	eleTp llvm.Type
	data  unsafe.Pointer
}

func (b *blockBuilder) BuildFunc(funcIr *ir.Instr, globals []*ExtGlobal) llvm.Value {
	for _, global := range globals {
		v := llvm.AddGlobal(rootModule, global.eleTp, global.name)
		fmt.Println("*** t", v.Type(), global.name)
		b.registers[global.name] = v
	}

	return b.buildInsn(funcIr)
}

func (b *blockBuilder) RunFunc(f llvm.Value, args []llvm.GenericValue, globals []*ExtGlobal) llvm.GenericValue {
	options := llvm.NewMCJITCompilerOptions()
	options.SetMCJITOptimizationLevel(2)
	execEngine, err := llvm.NewMCJITCompiler(rootModule, options)
	if err != nil {
		panic(err)
	}

	for _, global := range globals {
		execEngine.AddGlobalMapping(b.registers[global.name], global.data)
	}

	// fmt.Println("***** start run")
	res := execEngine.RunFunction(f, args)
	return res
}

func (b *blockBuilder) PrepareRunFunc(f llvm.Value, args []llvm.GenericValue, globals []*ExtGlobal) llvm.ExecutionEngine {
	options := llvm.NewMCJITCompilerOptions()
	options.SetMCJITOptimizationLevel(2)
	execEngine, err := llvm.NewMCJITCompiler(rootModule, options)
	if err != nil {
		panic(err)
	}

	for _, global := range globals {
		execEngine.AddGlobalMapping(b.registers[global.name], global.data)
	}

	return execEngine
}
