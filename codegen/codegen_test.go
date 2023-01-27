package codegen

import (
	"fmt"
	"math/rand"
	"testing"
	"unsafe"

	"github.com/kingfolk/capybara/ast"
	"github.com/kingfolk/capybara/semantics"
	"github.com/kingfolk/capybara/syntax"
	"github.com/kingfolk/capybara/types"

	"github.com/llvm/llvm-project/bindings/go/llvm"
	"github.com/rhysd/locerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO 变量值覆盖。SSA
func TestCodegenSupportReassign(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	raw := "fun f(): int = {let a:int = 1;let a = 2; a}; f()"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	val := builder.build(ir)

	fmt.Println("--- Dump ---")
	val.Dump()
	fmt.Println("\n--- Dump Done ---")

	// var execEngine, err = llvm.NewExecutionEngine(rootModule)
	// https://stackoverflow.com/questions/5988444/why-is-the-llvm-execution-engine-faster-than-compiled-code
	// llvm execute engine 可能会比编译到文件然后执行还快
	options := llvm.NewMCJITCompilerOptions()
	options.SetMCJITOptimizationLevel(2)
	execEngine, err := llvm.NewMCJITCompiler(rootModule, options)
	if err != nil {
		panic(err)
	}

	// args := []llvm.GenericValue{llvm.NewGenericValueFromInt(intT, 10, true)}
	args := []llvm.GenericValue{}
	retV := execEngine.RunFunction(val, args)
	fmt.Println("--- retV", retV)
}

func TestCodegenFuncSimple(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	raw := "fun f1(): int = {1 + 2}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	val := builder.build(ir)

	fmt.Println("--- Dump ---")
	val.Dump()
	fmt.Println("\n--- Dump Done ---")

	// var execEngine, err = llvm.NewExecutionEngine(rootModule)
	// https://stackoverflow.com/questions/5988444/why-is-the-llvm-execution-engine-faster-than-compiled-code
	// llvm execute engine 可能会比编译到文件然后执行还快
	options := llvm.NewMCJITCompilerOptions()
	options.SetMCJITOptimizationLevel(2)
	execEngine, err := llvm.NewMCJITCompiler(rootModule, options)
	if err != nil {
		panic(err)
	}

	// args := []llvm.GenericValue{llvm.NewGenericValueFromInt(intT, 10, true)}
	args := []llvm.GenericValue{}
	retV := execEngine.RunFunction(val, args)
	fmt.Println("--- retV", retV)
}

func TestCodegenCallFunc(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	raw := "fun f1(): int = {1 + 2}; f1()"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	val := builder.buildAndRun(ir)
	fmt.Println("-- result ", val.Int(true))

	assert.Equal(t, int64(3), int64(val.Int(true)))
}

func TestCodegenArraySubscript(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	raw := "fun f(): int = {let a:array<int,3> = array<int>(1,2,3);a[2]}; f()"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	val := builder.buildAndRun(ir)
	fmt.Println("-- result ", val.Int(true))

	assert.Equal(t, int64(3), int64(val.Int(true)))
}

func TestCodegenArrayAssignConst(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	raw := "fun f(): int = {let a:array<int,3> = array<int>(1,2,0);a[2]<-3;a[2]}; f()"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	val := builder.buildAndRun(ir)
	fmt.Println("-- result ", val.Int(true))

	assert.Equal(t, int64(3), int64(val.Int(true)))
}

func TestCodegenArrayAssignParam(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	raw := "fun f(b:int): int = {let a:array<int,3> = array<int>(1,2,0);a[2]<-b;a[2]}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	args := []llvm.GenericValue{llvm.NewGenericValueFromInt(intT, 3, true)}
	val := builder.buildAndRunFunc(ir, args, nil)
	fmt.Println("-- result ", val.Int(true))

	assert.Equal(t, int64(3), int64(val.Int(true)))
}

func TestCodegenArrayGlobal(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	globals := []*ExtGlobal{
		{
			name:  "globalarr",
			eleTp: intT,
			data:  unsafe.Pointer(&([]int32{1, 3, 5, 6, 9})[0]),
		},
	}
	globalVars := map[string]types.ValType{
		"globalarr": &types.Arr{
			Ele:  types.Int,
			Size: 5,
		},
	}

	raw := "fun f(b:int): int = {globalarr[3]}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)

	ir, env := semantics.EmitIRWithGlobal(node, globalVars)
	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	args := []llvm.GenericValue{llvm.NewGenericValueFromInt(intT, 3, true)}
	val := builder.buildAndRunFunc(ir, args, globals)
	fmt.Println("-- result ", val.Int(true))

	assert.Equal(t, int64(6), int64(val.Int(true)))
}

func TestCodegenLoopGlobalArrayModify(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	globals := []*ExtGlobal{
		{
			name:  "globalarr",
			eleTp: intT,
			data:  unsafe.Pointer(&([]int32{1, 3, 5, 6, 9})[0]),
		},
		{
			name:  "globalarr1",
			eleTp: intT,
			data:  unsafe.Pointer(&([]int32{0, 0, 0, 0, 0})[0]),
		},
	}
	globalVars := map[string]types.ValType{
		"globalarr": &types.Arr{
			Ele:  types.Int,
			Size: 5,
		},
		"globalarr1": &types.Arr{
			Ele:  types.Int,
			Size: 5,
		},
	}

	raw := "fun f(a:int): int = { for a = 0 .. 3 { globalarr1[a]<-globalarr[a]*10 }; globalarr1[3] }"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)

	ir, env := semantics.EmitIRWithGlobal(node, globalVars)
	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	args := []llvm.GenericValue{llvm.NewGenericValueFromInt(intT, 3, true)}
	val := builder.buildAndRunFunc(ir, args, globals)
	fmt.Println("-- result ", val.Int(true))

	assert.Equal(t, int64(60), int64(val.Int(true)))
}

func TestCodegenLoopGlobalArrayAddition(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	size := 1024
	globalArr1 := make([]int32, size)
	globalArr2 := make([]int32, size)
	globalArr := make([]int32, size)
	for i := 0; i < size; i++ {
		globalArr1[i] = int32(i)
		globalArr2[i] = int32(-i)
	}

	globals := []*ExtGlobal{
		{
			name:  "globalarr",
			eleTp: intT,
			data:  unsafe.Pointer(&globalArr[0]),
		},
		{
			name:  "globalarr1",
			eleTp: intT,
			data:  unsafe.Pointer(&globalArr1[0]),
		},
		{
			name:  "globalarr2",
			eleTp: intT,
			data:  unsafe.Pointer(&globalArr2[0]),
		},
	}
	globalVars := map[string]types.ValType{
		"globalarr": &types.Arr{
			Ele:  types.Int,
			Size: size,
		},
		"globalarr1": &types.Arr{
			Ele:  types.Int,
			Size: size,
		},
		"globalarr2": &types.Arr{
			Ele:  types.Int,
			Size: size,
		},
	}

	raw := "fun f(a:int): int = { for a = 0 .. 1024 { globalarr[a]<-globalarr1[a]+globalarr2[a] }; globalarr[3] }"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)

	ir, env := semantics.EmitIRWithGlobal(node, globalVars)
	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	args := []llvm.GenericValue{llvm.NewGenericValueFromInt(intT, 0, true)}
	f := builder.BuildFunc(ir.Ins[0], globals)

	val := builder.RunFunc(f, args, globals)
	fmt.Println("-- result ", val.Int(true))
	assert.Equal(t, int64(0), int64(val.Int(true)))
}

var ggv int = 11

// 表达式的if else逻辑是使用CASE WHEN进行的，有多个for loop，理论上应该要比如下写慢。
// TASK 1 将这个cici 代码库整理一下
// TASK 2 将demo逻辑搬到kundb。还有关联子查询的demo
// TASK 3 llvm是否可以回调到evalengine，使用那些预定义的表达式，而再不需要完全实现一边。参考gocaml的runtime
//        env可以提供，udf内绑定了env，input可以为空，因为pl udf内不需要处理列的逻辑，只需要处理result的内存分配问题，
//        可以类似gocaml一样runtime增加一个外部的GetTypedColumn运行时函数给llvm调得到result，然后传入VecEval中
func BenchmarkGoLoopIf(b *testing.B) {
	size := 10240
	globalArr1 := make([]int32, size)
	globalArr2 := make([]int32, size)
	globalArr := make([]int32, size)
	for i := 0; i < size; i++ {
		globalArr1[i] = rand.Int31()
		globalArr2[i] = rand.Int31()
	}

	b.Run("rrr", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for j := 0; j < size; j++ {
				if globalArr1[j] > 0 {
					globalArr[j] = globalArr1[j]
				} else {
					globalArr[j] = globalArr2[j]
				}
			}
		}
	})

	fmt.Println("globalArr[ggv]", globalArr[ggv])
}

func BenchmarkCodegenLoop(b *testing.B) {
	rootModule.Dispose()
	rootModule = llvm.NewModule("root")

	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	size := 102400000
	globalArr1 := make([]int32, size)
	globalArr2 := make([]int32, size)
	globalArr := make([]int32, size)
	for i := 0; i < size; i++ {
		globalArr1[i] = rand.Int31()
		globalArr2[i] = rand.Int31()
	}

	globals := []*ExtGlobal{
		{
			name:  "globalarr",
			eleTp: intT,
			data:  unsafe.Pointer(&globalArr[0]),
		},
		{
			name:  "globalarr1",
			eleTp: intT,
			data:  unsafe.Pointer(&globalArr1[0]),
		},
		{
			name:  "globalarr2",
			eleTp: intT,
			data:  unsafe.Pointer(&globalArr2[0]),
		},
	}
	globalVars := map[string]types.ValType{
		"globalarr": &types.Arr{
			Ele:  types.Int,
			Size: size,
		},
		"globalarr1": &types.Arr{
			Ele:  types.Int,
			Size: size,
		},
		"globalarr2": &types.Arr{
			Ele:  types.Int,
			Size: size,
		},
	}

	raw := "fun f(a:int): int = { for a = 0 .. 10240 { if globalarr1[a] > 0 then globalarr[a]<-globalarr1[a] else globalarr[a]<-globalarr2[a] }; globalarr[3] }"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		panic(err)
	}
	// ast.Println(node)

	ir, env := semantics.EmitIRWithGlobal(node, globalVars)
	// fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	args := []llvm.GenericValue{llvm.NewGenericValueFromInt(intT, 0, true)}
	f := builder.BuildFunc(ir.Ins[0], globals)
	execEngine := builder.PrepareRunFunc(f, args, globals)

	b.Run("rrr", func(b *testing.B) {
		b.ResetTimer()
		// val := builder.buildAndRunFunc(ir, args, globals)
		var val llvm.GenericValue
		for i := 0; i < b.N; i++ {
			val = execEngine.RunFunction(f, args)
		}
		fmt.Println("!! val", val)
	})
}

func TestCodegenIf(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	raw := "fun f1(): int = { if 2 > 1 then 10 else 20}; f1()"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	val := builder.buildAndRun(ir)
	fmt.Println("-- result ", val.Int(true))

	assert.Equal(t, int64(10), int64(val.Int(true)))
}

func TestCodegenAnd(t *testing.T) {
	llvm.LinkInMCJIT()
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	// raw := "fun f1(a: int): int = { if a + 3 > 1 && a > 2 then 10 else 20}"
	// raw := "fun f1(a: int): int = { if 1 < a && a > 2 then 10 else 20}"
	// raw := "fun f1(a: int): int = { if 4 <= a && a > 2 then 10 else 20}"
	// raw := "fun f1(a: float): int = { if a >= 4.0 && a > 2.0 then 10 else 20}"
	// raw := "fun f1(a: int)(b: int): int = { if 4 <= a && b > 3 && a > 2 then 10 else 20}"
	// raw := "fun f1(a: int): int = { if a > 1 && a < 1 then 10 else 20}"
	// raw := "fun f1(a: int)(b: int): int = { if a > b && a > b + 1 then 10 else 20}"
	// raw := "fun f1(a: int)(b: int): int = { if a+2 > b+1 && b+1 < a+2 then 10 else 20}"
	// raw := "fun f1(a: float): int = { if 1.0/a > 1.0 && 1.0/a < 1.0 then 10 else 20}"
	raw := "fun f1(a: float)(b: float): int = { if a > b || a == b then 10 else 20}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, env, err := semantics.EmitIR(node)
	require.NoError(t, err)

	fmt.Println("ir", ir)

	builder := newBlockBuilder(env)
	prog := builder.build(ir)

	fmt.Println("-- prog dump")
	// RunOptimizationPasses(OptimizeNone)
	RunOptimizationPasses(OptimizeLess)
	// RunOptimizationPasses(OptimizeAggressive)
	prog.Dump()

}
