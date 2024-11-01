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
	"github.com/kingfolk/capybara/semantics"
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

var (
	boolT    llvm.Type
	unitT    llvm.Type
	intT     llvm.Type
	floatT   llvm.Type
	voidPtrT llvm.Type
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

func init() {
	Reset()
}

func GetRootModule() llvm.Module {
	return rootModule
}

func Reset() {
	rootModule = llvm.NewModule("root")
	context = llvm.GlobalContext()
	unitT = context.VoidType()
	boolT = context.Int1Type()
	intT = context.Int32Type()
	floatT = context.FloatType()
	voidPtrT = llvm.PointerType(llvm.Int8Type(), 0)
}

func BuildFunc(fn *ir.Func, debug bool, globals ...*ExtGlobal) llvm.Value {
	builder := newBlockBuilder(&types.Env{Defs: fn.Defs}, debug)
	for _, global := range globals {
		builder.registers[global.Name] = global.Reg
	}
	return builder.buildFunc(fn.Body.Name, fn)
}

func BuildModule(mod *ir.Module, debug bool) llvm.Value {
	for _, fn := range mod.Funcs {
		builder := newBlockBuilder(&types.Env{Defs: fn.Defs}, debug)
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
		Body: &rootBlk,
		Tp:   rootFnTp,
		Defs: mod.Env.Defs,
	}
	builder := newBlockBuilder(mod.Env, debug)
	fmt.Println("--- anon ---", len(rootBlk.Ins))
	for _, i := range rootBlk.Ins {
		fmt.Println(i.String())
	}
	fmt.Println("--- anon end ---")
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
		buildCtx: &buildContext{
			blkMap: make(map[int]llvm.BasicBlock),
		},
	}
}

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
				switch phi.ins.Type().Code() {
				case types.TpBool:
					edgeVars = append(edgeVars, llvm.ConstNull(boolT))
				case types.TpUnit, types.TpInt:
					edgeVars = append(edgeVars, llvm.ConstNull(intT))
				default:
					panic("TODO: phi " + phi.ins.String() + ". type:" + phi.ins.Type().String())
				}
			} else {
				edgeVars = append(edgeVars, b.registers[edge])
			}
			src := phi.blk.Src[i]
			edgeBlks = append(edgeBlks, b.buildCtx.blkMap[src.Id])
		}
		phi.v.AddIncoming(edgeVars, edgeBlks)
	}
}

func (b *blockBuilder) buildFunc(name string, f *ir.Func) llvm.Value {
	tpDef := f.Type()
	funcType := b.buildFuncType(tpDef.(*types.Func), true)
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
		b.registers[paramName] = args[i]
	}

	b.buildBlock(f.Body)
	b.finalizePhi()

	if b.debug {
		fmt.Println("--- theFunction.Dump() " + name)
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

func (b *blockBuilder) buildRet(ident string, v *ir.Ret) llvm.Value {
	if v.Target == "" {
		return b.builder.CreateRetVoid()
	}
	ret := b.resolve(v.Target)
	if ret.IsNil() {
		panic("retVal.IsNil")
	}

	b.builder.CreateRet(ret)
	return ret
}

func (b *blockBuilder) buildArrLit(ident string, al *ir.ArrLit) llvm.Value {
	t := b.env.GetDefTrusted(ident)
	elemTy := b.buildType(t.(*types.Arr).Ele)
	sizeVal := llvm.ConstInt(intT, uint64(len(al.Args)), false /*signed*/)
	alloca := b.builder.CreateArrayAlloca(elemTy, sizeVal, ident)

	for i, elem := range al.Args {
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

func (b *blockBuilder) buildRecLit(ident string, rl *ir.RecLit) llvm.Value {
	t := b.env.GetDefTrusted(ident)
	tp := b.buildType(t)
	alloca := b.builder.CreateAlloca(tp, ident)

	for i, elem := range rl.Args {
		elemVal := b.resolve(elem)
		b.buildRecStore(alloca, elemVal, i)
	}
	return alloca
}

func (b *blockBuilder) buildRecAcs(ident string, ra *ir.RecAcs) llvm.Value {
	recVal := b.resolve(ra.Target)
	return b.buildRecLoad(recVal, ra.Idx)
}

func (b *blockBuilder) buildRecStore(recVal, elemVal llvm.Value, idx int) {
	ptr := b.builder.CreateStructGEP(recVal, idx, "rec")
	b.builder.CreateStore(elemVal, ptr)
}

func (b *blockBuilder) buildRecLoad(recVal llvm.Value, idx int) llvm.Value {
	elemPtr := b.builder.CreateStructGEP(recVal, idx, "")
	return b.builder.CreateLoad(elemPtr, "recaccess")
}

func (b *blockBuilder) buildEnumVar(ident string, ev *ir.EnumVar) llvm.Value {
	if ev.Tp.Simple {
		return llvm.ConstInt(intT, uint64(ev.Idx), false)
	}

	tp := b.buildType(semantics.EnumBox)
	idxVal := llvm.ConstInt(intT, uint64(ev.Idx), false)
	alloca := b.builder.CreateAlloca(tp, ident)
	b.buildRecStore(alloca, idxVal, 0)
	if ev.Box != "" {
		elemVal := b.resolve(ev.Box)
		ptr := b.boxWhole(elemVal, ev.Tp.Tps[ev.Idx])
		b.buildRecStore(alloca, ptr, 1)
	}
	return alloca
}

func (b *blockBuilder) buildDiscriminant(ident string, dc *ir.Discriminant) llvm.Value {
	v := b.resolve(dc.Target)
	if dc.Simple {
		return v
	}
	return b.buildRecLoad(v, 0)
}

func (b *blockBuilder) buildBox(ident string, bx *ir.Box) llvm.Value {
	v := b.resolve(bx.Target)
	tp, boxTp := bx.Tp, bx.BoxTp
	if boxTp.Code() == types.TpVar {
		return b.boxWhole(v, tp)
	}
	return b.boxRec(v, tp, boxTp)
}

func (b *blockBuilder) boxWhole(v llvm.Value, tp types.ValType) llvm.Value {
	v = b.buildPtr(v, tp)
	return b.builder.CreateBitCast(v, voidPtrT, "")
}

func (b *blockBuilder) buildPtr(v llvm.Value, tp types.ValType) llvm.Value {
	switch tp {
	case types.Int:
		return b.builder.CreateIntToPtr(v, llvm.PointerType(intT, 0), "")
	case types.Float:
		v = b.builder.CreateBitCast(v, intT, "")
		v = b.builder.CreateIntToPtr(v, llvm.PointerType(intT, 0), "")
		return b.builder.CreateBitCast(v, llvm.PointerType(floatT, 0), "")
	default:
		return v
	}
}

func (b *blockBuilder) boxRec(v llvm.Value, tp, boxedTp types.ValType) llvm.Value {
	t := b.buildType(boxedTp)
	boxed := b.builder.CreateAlloca(t, "")

	for i, t := range boxedTp.(*types.Rec).MemTps {
		unboxedTp := tp.(*types.Rec).MemTps[i]
		ele := b.buildRecLoad(v, i)
		var arg llvm.Value
		switch tt := t.(type) {
		case *types.TypeVar:
			if tt.Lower != nil {
				arg = b.boxTrait("form boxrec", tt.Lower.(*types.Trait), ele, unboxedTp)
			} else {
				arg = b.boxWhole(ele, unboxedTp)
			}
		case *types.Rec:
			arg = b.boxRec(ele, unboxedTp, t)
		default:
			panic("TODO: " + t.String())
		}
		b.buildRecStore(boxed, arg, i)
	}
	return boxed
}

func (b *blockBuilder) buildUnbox(ident string, ub *ir.Unbox) llvm.Value {
	v := b.resolve(ub.Target)
	if ub.BoxTp.Code() == types.TpVar {
		// unboxTrait occurs only for trait lower bound, which is box to trait at first, need to unbox trait operation
		if t, ok := ub.BoxTp.(*types.TypeVar); ok && t.Lower != nil && t.Lower.Code() == types.TpTrait {
			return b.unboxTrait(v, ub.Tp)
		}
		return b.unboxWhole(v, ub.Tp)
	}
	return b.unboxRec(v, ub.Tp, ub.BoxTp)
}

func (b *blockBuilder) unboxWhole(v llvm.Value, tp types.ValType) llvm.Value {
	t := b.buildType(tp)
	if tp.Code() == types.TpVar {
		t = intT
	}

	ptrTp := llvm.PointerType(t, 0)
	v = b.builder.CreateBitCast(v, ptrTp, "")
	switch tp.Code() {
	case types.TpInt, types.TpVar:
		return b.builder.CreatePtrToInt(v, t, "")
	case types.TpFloat:
		v = b.builder.CreateBitCast(v, llvm.PointerType(intT, 0), "")
		v = b.builder.CreatePtrToInt(v, intT, "")
		return b.builder.CreateBitCast(v, floatT, "")
	default:
		return v
	}
}

func (b *blockBuilder) unboxTrait(v llvm.Value, tp types.ValType) llvm.Value {
	vv := b.buildRecLoad(v, 0)
	return b.unboxWhole(vv, tp)
}

func (b *blockBuilder) unboxRec(v llvm.Value, tp, boxedTp types.ValType) llvm.Value {
	allPrimitive := true
	for _, t := range boxedTp.(*types.Rec).MemTps {
		if !types.IsPrimitive(t) {
			allPrimitive = false
		}
	}
	if allPrimitive {
		return v
	}

	t := b.buildType(tp)
	unboxed := b.builder.CreateAlloca(t, "")
	for i, t := range boxedTp.(*types.Rec).MemTps {
		unboxedTp := tp.(*types.Rec).MemTps[i]
		ele := b.buildRecLoad(v, i)
		var arg llvm.Value
		switch t.(type) {
		case *types.TypeVar:
			arg = b.unboxWhole(ele, unboxedTp)
		case *types.Rec:
			arg = b.unboxRec(ele, unboxedTp, t)
		default:
			panic("TODO")
		}
		b.buildRecStore(unboxed, arg, i)
	}
	return unboxed
}

func (b *blockBuilder) buildBoxTrait(ident string, tb *ir.BoxTrait) llvm.Value {
	targetTp := b.typeOf(tb.Target)
	target := b.resolve(tb.Target)
	return b.boxTrait(ident, tb.Tp, target, targetTp)
}

func (b *blockBuilder) boxTrait(ident string, tt *types.Trait, target llvm.Value, targetTp types.ValType) llvm.Value {
	imp := targetTp.Impls()

	tp := b.buildType(tt)
	alloca := b.builder.CreateAlloca(tp, ident)

	fnPart := b.builder.CreateStructGEP(alloca, 1, "rec")
	for i, k := range tt.Keys {
		_, ok := imp.Fns[k]
		if !ok {
			panic("unreachable. impl fn not found: " + k)
		}
		fnName := imp.Prefix + "$" + k
		f := rootModule.NamedFunction(fnName)
		fnptr := b.buildRecLoad(fnPart, i)
		// pointer cast impl fun type to trait fun type
		// See buildFuncType related `expandTrait` doc. trait func is correctly built but first param. Here actual func
		// generated by call NamedFunction first param is different from `fnptr` func. The two funcs represent the same
		// thing, but due to recursive type building issue, they have slightly different type. Thus here make a pointer cast.
		f = b.builder.CreatePointerCast(f, fnptr.Type(), "fncast")
		b.buildRecStore(fnPart, f, i)
	}

	target = b.boxWhole(target, targetTp)
	b.buildRecStore(alloca, target, 0)
	return alloca
}

func (b *blockBuilder) buildCall(c *ir.StaticCall) llvm.Value {
	args := make([]llvm.Value, len(c.Args))
	for i, arg := range c.Args {
		v := b.resolve(arg)
		args[i] = v
	}

	f := rootModule.NamedFunction(c.Name)
	if f.C == nil {
		panic("function " + c.Name + " not found in llvm module")
	}
	ret := b.builder.CreateCall(f, args, "")
	return ret
}

func (b *blockBuilder) buildTraitCall(c *ir.TraitCall) llvm.Value {
	target := b.resolve(c.Args[0])
	fnIdx := -1
	for i, k := range c.Trait.Keys {
		if k == c.Name {
			fnIdx = i
		}
	}
	if fnIdx == -1 {
		panic("unreachable. trait fn not found: " + c.Name)
	}

	args := make([]llvm.Value, len(c.Args))
	for i, arg := range c.Args {
		v := b.resolve(arg)
		args[i] = v
	}

	fns := b.builder.CreateStructGEP(target, 1, "")
	fn := b.buildRecLoad(fns, fnIdx)
	for i, arg := range args {
		if i == 0 {
			// load data part of index 0
			args[i] = b.buildRecLoad(arg, 0)
		}
	}
	return b.builder.CreateCall(fn, args, "trait call")
}

func (b *blockBuilder) buildPhi(it *ir.Phi) llvm.Value {
	tp := b.buildType(it.Tp)
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

	return llvm.ConstNull(intT)
}

func (b *blockBuilder) buildTypePtr(tp types.ValType) llvm.Type {
	if tp.Code() == types.TpRec || tp.Code() == types.TpArr || tp.Code() == types.TpTrait {
		return llvm.PointerType(b.buildType(tp), 0)
	}
	if t, ok := tp.(*types.TypeVar); ok && t.Lower != nil {
		return b.buildTypePtr(t.Lower)
	}
	return b.buildType(tp)
}

// TODO rec类型在llvm类型推断系统里应该被pointer封装，但是在alloca时则不需要，所以独立了一个buildTypePtr
// 这部分的区分逻辑需要优化的写法
func (b *blockBuilder) buildType(tp types.ValType) llvm.Type {
	switch tp.Code() {
	case types.TpUnit:
		return unitT
	case types.TpBool:
		return boolT
	case types.TpInt:
		return intT
	case types.TpFloat:
		return floatT
	case types.TpVoidPtr:
		return voidPtrT
	case types.TpVar:
		if tp.(*types.TypeVar).Lower != nil {
			return b.buildType(tp.(*types.TypeVar).Lower)
		}
		return voidPtrT
	case types.TpArr:
		arrTp := tp.(*types.Arr)
		return context.StructType([]llvm.Type{
			llvm.PointerType(b.buildType(arrTp.Ele), 0),
			intT,
		}, false)
	case types.TpRec:
		recTp := tp.(*types.Rec)
		tps := []llvm.Type{}
		for _, tp := range recTp.MemTps {
			memTp := b.buildType(tp)
			if tp.Code() == types.TpRec || tp.Code() == types.TpTrait || (tp.Code() == types.TpVar && tp.(*types.TypeVar).Lower != nil) {
				memTp = llvm.PointerType(memTp, 0)
			}
			tps = append(tps, memTp)
		}
		return context.StructType(tps, false)
	case types.TpFunc:
		return b.buildFuncType(tp.(*types.Func), false)
	case types.TpTrait:
		traitTp := tp.(*types.Trait)
		return b.buildTraitType(traitTp)
	default:
		panic("unsupported type: " + tp.String())
	}
}

func (b *blockBuilder) typeOf(ident string) types.ValType {
	if t, ok := b.env.Defs[ident]; ok {
		return t
	}
	panic("Type was not found for ident: " + ident)
}

func (b *blockBuilder) buildFuncType(tp *types.Func, expandTrait bool) llvm.Type {
	params := make([]llvm.Type, len(tp.Params))
	for i, param := range tp.Params {
		// if param is trait type and expandTrait is false, then give llvm void* type.
		// there is a paradox if expandTrait set to true for trait function:
		// type trait_b = trait { f1():int } desugar -> type b = trait {f1(self:trait_b):int}. trait_b type is a recursive type.
		// To build trait_b type, f1 must be built beforehand and f1's first param trait_b must be built, this forms a recursive
		// building process infinitely.
		// Recursive type might not be a program if you are using c/c++ llvm api, but there is no good way of golang llvm api to
		// implement it. For c/c++ llvm api, you can just create a empty trait llvm type and put pointer of trait as first param
		// of f1 func. After all trait funcs are handled, fill trait's func types. But golang llvm api doest not directly expose
		// pointer or reference of a llvm type. A type is created as an immutable form and is different to change after create.
		// Thus `expandTrait` is introduced to disable build trait func's first param, which disable infinite trait type building
		// process.
		if param.Code() == types.TpTrait && !expandTrait {
			params[i] = voidPtrT
		} else {
			params[i] = b.buildTypePtr(param)
		}
	}
	retTp := b.buildTypePtr(tp.Ret)
	return llvm.FunctionType(retTp, params, false)
}

func (b *blockBuilder) buildTraitType(tp *types.Trait) llvm.Type {
	var fnTps []llvm.Type
	for _, traitFn := range tp.Fns {
		fnTp := b.buildFuncType(traitFn, false)
		fnTps = append(fnTps, llvm.PointerType(fnTp, 0))
	}
	memTps := []llvm.Type{
		voidPtrT,
		context.StructType(fnTps, false),
	}
	return context.StructType(memTps, false)
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
			if argTp.Code() == types.TpEnum && argTp.(*types.Enum).Simple {
				return b.builder.CreateICmp(llvm.IntEQ, regs[0], regs[1], "==")
			}
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
	case *ir.Ret:
		return b.buildRet(ident, expr)
	case *ir.If:
		// TODO: a = if ... then to if {}
		return b.buildIf(ident, expr)
	case *ir.ArrLit:
		return b.buildArrLit(ident, expr)
	case *ir.ArrGet:
		return b.buildArrGet(ident, expr)
	case *ir.ArrPut:
		return b.buildArrPut(ident, expr)
	case *ir.RecLit:
		return b.buildRecLit(ident, expr)
	case *ir.RecAcs:
		return b.buildRecAcs(ident, expr)
	case *ir.EnumVar:
		return b.buildEnumVar(ident, expr)
	case *ir.Discriminant:
		return b.buildDiscriminant(ident, expr)
	case *ir.Box:
		return b.buildBox(ident, expr)
	case *ir.BoxTrait:
		return b.buildBoxTrait(ident, expr)
	case *ir.Unbox:
		return b.buildUnbox(ident, expr)
	case *ir.Func:
		// TODO: i.Ident应该是需要隐藏的，函数名应该放在ir.Fun里
		// TODO 到底用ident还是expr.Body.Name
		return b.buildFunc(expr.Body.Name, expr)
	case *ir.StaticCall:
		return b.buildCall(expr)
	case *ir.TraitCall:
		return b.buildTraitCall(expr)
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
