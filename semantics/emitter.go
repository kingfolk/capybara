package semantics

import (
	"fmt"
	"runtime/debug"
	"strconv"

	"github.com/kingfolk/capybara/ast"
	"github.com/kingfolk/capybara/errors"
	"github.com/kingfolk/capybara/ir"
	"github.com/kingfolk/capybara/types"
)

type Scope struct {
	// TODO nested scope
	// parent *Scope
	blockId int
	vars    map[string]string
	blk     *ir.Block
}

type Emitter struct {
	debug   bool
	count   int
	env     *types.Env
	globals map[string]types.ValType
	scope   *Scope
	module  *ir.Module
}

const (
	rootBlock = "$root$"
)

func NewScope() *Scope {
	return &Scope{
		vars: map[string]string{},
	}
}

type GlobalDef struct {
	Name string
	Tp   types.ValType
}

// EmitIR converts given AST into MIR with type environment
func EmitIR(mod *ast.AST, debugMode bool, globals ...GlobalDef) (root *ir.Module, em *Emitter, err error) {
	e := &Emitter{
		debug: debugMode,
		count: 0,
		env: &types.Env{
			Types: map[string]types.ValType{},
			Defs:  map[string]types.ValType{},
		},
		globals: map[string]types.ValType{},
		scope:   NewScope(),
		module:  &ir.Module{},
	}

	defer func() {
		if message := recover(); message != nil {
			debug.PrintStack()
			if er, ok := message.(error); ok {
				err = er
			} else {
				err = fmt.Errorf("%s", message)
			}
		}
	}()

	for _, g := range globals {
		e.globals[g.Name] = g.Tp
		e.env.Defs[g.Name] = g.Tp
		e.scope.vars[g.Name] = g.Name
	}

	for _, tDecl := range mod.TypeDecls {
		tp := e.emitType(tDecl.Type)
		e.env.Types[tDecl.Ident.Name] = tp
	}

	blk := e.emitBlock(rootBlock, mod.Root...)
	e.module.Root = blk
	for _, ins := range blk.Ins {
		if f, ok := ins.Val.(*ir.Func); ok {
			e.module.Funcs = append(e.module.Funcs, f)
		}
	}

	if e.debug {
		fmt.Println("--- original anon bb ---")
		fmt.Println(ir.CFGString(blk))
		fmt.Println("--- original anon bb end ---")
	}

	maker := ir.NewDominatorMaker(blk, e.debug)
	declTable := maker.Lift(e.env.Defs)
	root = e.module
	root.Env = e.env
	root.Env.Defs = declTable
	em = e

	return
}

// EmitIRWithGlobal converts given AST into MIR with type environment
func EmitIRWithGlobal(mod *ast.AST, globalVars map[string]types.ValType) (*ir.Block, *types.Env) {
	e := &Emitter{
		count: 0,
		env: &types.Env{
			Defs: map[string]types.ValType{},
		},
		scope: NewScope(),
	}
	for k, t := range globalVars {
		e.env.Defs[k] = t
		e.scope.vars[k] = k
	}
	return e.emitBlock(rootBlock, mod.Root...), e.env
}

func (e *Emitter) emitBlock(name string, nodes ...ast.Expr) *ir.Block {
	reserved := e.scope.blk
	defer func() {
		if name != rootBlock {
			e.scope.blk = reserved
		}
	}()

	blk := ir.NewBlock(&e.scope.blockId, name)
	e.scope.blk = blk
	for _, node := range nodes {
		e.emitInsn(node)
	}
	return blk
}

func (e *Emitter) GetDeclVars() map[string]string {
	return e.scope.vars
}

func (e *Emitter) emitInsn(node ast.Expr) *ir.Instr {
	switch n := node.(type) {
	case *ast.Unit:
		// TODO TEST
		c := ir.NewUnit()
		return e.rvalInstr(c)
	case *ast.Int:
		c := ir.NewConst(types.Int, []byte(strconv.FormatInt(n.Value, 10)))
		return e.rvalInstr(c)
	case *ast.Float:
		c := ir.NewConst(types.Float, []byte(strconv.FormatFloat(n.Value, 'g', -1, 64)))
		return e.rvalInstr(c)
	case *ast.VarRef:
		// TODO NESTED SCOPE
		if ident, ok := e.scope.vars[n.Symbol.Name]; ok {
			tp := e.env.GetDefTrusted(ident)
			insn := e.rvalInstr(ir.NewRef(tp, ident))
			return insn
		}
		panic("undefined identifiers: " + n.Symbol.Name)
	case *ast.Add:
		return e.emitArithInsn(ir.ADD, n.Left, n.Right, node)
	case *ast.Sub:
		return e.emitArithInsn(ir.SUB, n.Left, n.Right, node)
	case *ast.Mul:
		return e.emitArithInsn(ir.MUL, n.Left, n.Right, node)
	case *ast.Div:
		return e.emitArithInsn(ir.DIV, n.Left, n.Right, node)
	case *ast.Less:
		return e.emitCompareInsn(ir.LT, n.Left, n.Right, node)
	case *ast.LessEq:
		return e.emitCompareInsn(ir.LTE, n.Left, n.Right, node)
	case *ast.Greater:
		return e.emitCompareInsn(ir.GT, n.Left, n.Right, node)
	case *ast.GreaterEq:
		return e.emitCompareInsn(ir.GTE, n.Left, n.Right, node)
	case *ast.Eq:
		return e.emitCompareInsn(ir.EQ, n.Left, n.Right, node)
	case *ast.NotEq:
		return e.emitCompareInsn(ir.NEQ, n.Left, n.Right, node)
	case *ast.And:
		return e.emitLogicalInsn(ir.AND, n.Left, n.Right, node)
	case *ast.Or:
		return e.emitLogicalInsn(ir.OR, n.Left, n.Right, node)
	case *ast.ArrayLit:
		return e.emitArrLitInsn(n)
	case *ast.ApplyBracket:
		return e.emitArrGetInsn(n)
	case *ast.ArrayPut:
		return e.emitArrPutInsn(n)
	case *ast.RecLit:
		return e.emitRecLitInsn(n)
	case *ast.RecAcs:
		return e.emitRecAcsInsn(n)
	case *ast.Apply:
		return e.emitAppInsn(n)
	case *ast.If:
		return e.emitIfInsn(n)
	case *ast.Loop:
		return e.emitLoopInsn(n)
	case *ast.Let:
		return e.emitLetInsn(n)
	case *ast.Mutate:
		return e.emitMutateInsn(n)
	case *ast.LetRec:
		return e.emitFunInsn(n)
	default:
		panic(fmt.Sprintf("unsupported instr %s: %+v", node.Name(), node))
	}
}

func (e *Emitter) emitArithInsn(op ir.OperatorKind, lhs, rhs, node ast.Expr) *ir.Instr {
	l := e.emitInsn(lhs)
	r := e.emitInsn(rhs)
	TypeCheckEqual(l.Type(), r.Type())
	TypeCheckNumeric(l.Type())
	TypeCheckNumeric(r.Type())
	return e.rvalInstr(ir.NewBinary(op, l.Ident, r.Ident, l.Type()))
}

func (e *Emitter) emitCompareInsn(op ir.OperatorKind, lhs, rhs, node ast.Expr) *ir.Instr {
	l := e.emitInsn(lhs)
	r := e.emitInsn(rhs)
	TypeCheckEqual(l.Type(), r.Type())
	return e.rvalInstr(ir.NewBinary(op, l.Ident, r.Ident, types.Bool))
}

func (e *Emitter) emitLogicalInsn(op ir.OperatorKind, lhs, rhs, node ast.Expr) *ir.Instr {
	l := e.emitInsn(lhs)
	r := e.emitInsn(rhs)
	TypeCheckEqual(l.Type(), types.Bool)
	TypeCheckEqual(l.Type(), types.Bool)
	return e.rvalInstr(ir.NewBinary(op, l.Ident, r.Ident, types.Bool))
}

func TypeCheckEqual(l, r types.ValType) {
	if l != r {
		panic(fmt.Sprintf("TypeError: type mismatch. %s and %s", l, r))
	}
}

func TypeCheckNumeric(t types.ValType) {
	if t != types.Int && t != types.Float {
		panic("TypeError: operands is not numeric type")
	}
}

func (e *Emitter) emitLetInsn(node *ast.Let) *ir.Instr {
	bound := e.emitInsn(node.Bound)
	_, ok := e.scope.vars[node.Symbol.Name]
	if ok {
		panic("TypeError: re-declaration of " + node.Symbol.Name)
	}
	e.scope.vars[node.Symbol.Name] = bound.Ident

	if node.Type != nil {
		tp := e.emitType(node.Type)
		e.env.Defs[node.Symbol.Name] = tp
		right := e.env.GetDefTrusted(bound.Ident)
		if err := types.TypeCompatible(tp, right); err != nil {
			panic(err)
		}
	}
	return bound
}

func (e *Emitter) emitMutateInsn(node *ast.Mutate) *ir.Instr {
	right := e.emitInsn(node.Right)
	ident, ok := e.scope.vars[node.Ref.Symbol.Name]
	if !ok {
		panic("TypeError: undeclared of " + node.Ref.Symbol.Name)
	}
	it := &ir.Instr{
		Ident: ident,
		Kind:  ir.RValKind,
		Val:   ir.NewRef(right.Type(), right.Ident),
	}
	e.scope.blk.Ins = append(e.scope.blk.Ins, it)
	return it
}

func (e *Emitter) emitIfInsn(n *ast.If) *ir.Instr {
	// TODO: prev muse be bool type
	prev := e.emitInsn(n.Cond)
	thenBlk := e.emitBlock("if "+prev.Ident+" then", n.Then...)
	elseBlk := e.emitBlock("if "+prev.Ident+" else", n.Else...)
	linkBB(e.scope.blk, thenBlk)
	linkBB(e.scope.blk, elseBlk)
	val := &ir.If{
		Cond: prev.Ident,
		Then: thenBlk,
		Else: elseBlk,
	}
	i := e.instr(val, e.genID(), ir.IfKind)

	e.scope.blk = ir.NewBlock(&e.scope.blockId, "if "+prev.Ident+" after")
	linkBB(thenBlk, e.scope.blk)
	linkBB(elseBlk, e.scope.blk)

	return i
}

func (e *Emitter) emitLoopInsn(n *ast.Loop) *ir.Instr {
	origBlk := e.scope.blk

	loopStartBlk := e.emitBlock("loop start")
	loopBodyBlk := e.emitBlock("loop body", n.Body...)
	afterBlk := e.emitBlock("loop after")

	e.scope.blk = loopStartBlk
	cond := e.emitInsn(n.Cond)
	val := &ir.If{
		Cond: cond.Ident,
		Then: loopBodyBlk,
		Else: afterBlk,
	}
	e.instr(val, ir.DangleIdent(), ir.IfKind)
	linkBB(origBlk, loopStartBlk)
	linkBB(loopStartBlk, loopBodyBlk)
	linkBB(loopStartBlk, afterBlk)
	linkBB(loopBodyBlk, loopStartBlk)

	e.scope.blk = afterBlk
	return e.emitInsn(&ast.Unit{})
}

func (e *Emitter) emitFunInsn(node *ast.LetRec) *ir.Instr {
	origScope := e.scope
	e.scope = NewScope()
	for k := range e.globals {
		e.scope.vars[k] = k
	}

	name := node.Func.Symbol.Name
	// ty, ok := e.env.DeclTable[name]
	// if !ok {
	// 	panic("FATAL: Unknown function: " + name)
	// }

	params := make([]string, 0, len(node.Func.Params))
	paramTypes := make([]types.ValType, len(node.Func.Params))
	tpVars := make([]*types.TypeVar, len(node.Func.TpParams))
	for i, param := range node.Func.Params {
		paramName := param.Ident.Name
		ident := e.genID()
		params = append(params, ident)
		tp := e.emitType(param.Type)
		paramTypes[i] = tp
		e.env.Defs[ident] = tp
		e.scope.vars[paramName] = ident
	}
	for i, tpParam := range node.Func.TpParams {
		tpVars[i] = &types.TypeVar{Name: tpParam.Name}
	}
	blkName := name
	blk := e.emitBlock(blkName, node.Func.Body...)
	if e.debug {
		fmt.Println("--- original bb ---")
		fmt.Println(ir.CFGString(blk))
		fmt.Println("--- original bb end ---")
	}

	funTp := &types.Func{
		Params: paramTypes,
		Ret:    e.emitType(node.Func.RetType),
		TpVars: tpVars,
	}

	stack := []*ir.Block{blk}
	visited := map[int]bool{}
	for len(stack) > 0 {
		top := stack[0]
		visited[top.Id] = true
		if top.Dest == nil && len(top.Ins) > 0 {
			last := top.Ins[len(top.Ins)-1]
			retTp := e.env.GetDefTrusted(last.Ident)
			if err := types.TypeCompatible(funTp.Ret, retTp); err != nil {
				panic(err)
			}
		}
		stack = stack[1:]
		for _, b := range top.Dest {
			if visited[b.Id] {
				continue
			}
			stack = append(stack, b)
		}
	}

	val := &ir.Func{
		Params: params,
		Body:   blk,
		Tp:     funTp,
	}

	maker := ir.NewDominatorMaker(blk, e.debug, params...)
	defs := maker.Lift(e.env.Defs)
	val.Defs = defs

	e.scope = origScope
	return e.instr(val, name, ir.FuncKind)
}

func (e *Emitter) emitArrLitInsn(node *ast.ArrayLit) *ir.Instr {
	args := make([]string, len(node.Elems))
	for i, ele := range node.Elems {
		arg := e.emitInsn(ele)
		args[i] = arg.Ident
	}
	tp := e.env.GetDefTrusted(args[0])
	val := &ir.ArrLit{
		Tp:   tp,
		Args: args,
	}
	return e.rvalInstr(val)
}

func (e *Emitter) emitArrGetInsn(node *ast.ApplyBracket) *ir.Instr {
	arr := e.emitInsn(node.Expr)
	if len(node.Args) != 1 {
		panic("unreachable. parser should have handled more than one subscript arg")
	}
	index := e.emitInsn(node.Args[0])
	tp := e.env.GetDefTrusted(arr.Ident)
	eleTp := tp.(*types.Arr).Ele
	val := &ir.ArrGet{
		Tp:    eleTp,
		Arr:   arr.Ident,
		Index: index.Ident,
	}
	return e.rvalInstr(val)
}

func (e *Emitter) emitArrPutInsn(node *ast.ArrayPut) *ir.Instr {
	arr := e.emitInsn(node.Array)
	index := e.emitInsn(node.Index)
	right := e.emitInsn(node.Assignee)

	val := &ir.ArrPut{
		Arr:   arr.Ident,
		Index: index.Ident,
		Right: right.Ident,
	}
	return e.instr(val, e.genID(), ir.CallKind)
}

func (e *Emitter) emitRecLitInsn(node *ast.RecLit) *ir.Instr {
	tp, ok := e.env.Types[node.Ref.Symbol.Name]
	if !ok {
		panic("TypeError: undeclared type of " + node.Ref.Symbol.Name)
	}
	tRec := tp.(*types.Rec)
	args := make([]string, len(tRec.Keys))
	argTps := make([]types.ValType, len(tRec.Keys))
	if len(node.Args) != len(tRec.Keys) {
		panic(errors.NewError(errors.TYPE_RECORD_NOT_FULFILLED, "struct literal not fulfilled"))
	}
	for _, arg := range node.Args {
		idx := tRec.KeyIndex(arg.Ident.Name)
		if idx == -1 {
			panic(errors.NewError(errors.TYPE_RECORD_KEY_NOTFOUND, "struct key "+arg.Ident.Name+" not found"))
		}
		if entry := args[idx]; entry != "" {
			panic(errors.NewError(errors.TYPE_RECORD_NOT_FULFILLED, "struct literal not fulfilled"))
		}
		i := e.emitInsn(arg.Arg)
		args[idx] = i.Ident
		tpArg := e.env.GetDefTrusted(i.Ident)
		argTps[idx] = tpArg
	}
	var tpArgs []types.ValType
	for _, tpArg := range node.TpArgs {
		tpArgs = append(tpArgs, e.emitType(tpArg))
	}
	tRec, err := types.TypeCheckRecLit(tRec, tpArgs, argTps)
	if err != nil {
		panic(err)
	}
	val := &ir.RecLit{
		Tp:   tRec,
		Args: args,
	}
	return e.rvalInstr(val)
}

func (e *Emitter) emitRecAcsInsn(node *ast.RecAcs) *ir.Instr {
	rec := e.emitInsn(node.Expr)
	t := e.env.GetDefTrusted(rec.Ident)
	tRec, ok := t.(*types.Rec)
	if !ok {
		panic("rec access apply on non-rec type: " + t.String())
	}
	idx := tRec.KeyIndex(node.Acs.Name)
	val := &ir.RecAcs{
		Tp:  tRec.MemTps[idx],
		Rec: rec.Ident,
		Idx: idx,
	}
	return e.rvalInstr(val)
}

func (e *Emitter) emitAppInsn(node *ast.Apply) *ir.Instr {
	ref, ok := node.Callee.(*ast.VarRef)
	if !ok {
		panic("unsupported apply instr")
	}
	t := e.env.GetDefTrusted(ref.Symbol.Name)
	tFun, ok := t.(*types.Func)
	if !ok {
		panic("APPLY not to func type: " + ref.Symbol.Name)
	}

	args := make([]string, len(node.Args))
	argTps := make([]types.ValType, len(node.Args))
	boxes := make([]types.ValType, len(node.Args)+1)
	for i, arg := range node.Args {
		arg := e.emitInsn(arg)
		args[i] = arg.Ident
		if types.HasTpVar(tFun.Params[i]) {
			boxes[i] = tFun.Params[i]
		}
		argTp := e.env.GetDefTrusted(arg.Ident)
		argTps[i] = argTp
	}
	if types.HasTpVar(tFun.Ret) {
		boxes[len(boxes)-1] = tFun.Ret
	}

	var tpArgs []types.ValType
	for _, tpArg := range node.TpArgs {
		tpArgs = append(tpArgs, e.emitType(tpArg))
	}

	tFun, err := types.TypeCheckApp(tFun, tpArgs, argTps)
	if err != nil {
		panic(err)
	}

	val := &ir.Call{
		Name:  ref.Symbol.Name,
		Tp:    tFun.Ret,
		Args:  args,
		Boxes: boxes,
	}

	return e.rvalInstr(val)
}

func (e *Emitter) rvalInstr(val ir.Val) *ir.Instr {
	return e.instr(val, e.genID(), ir.RValKind)
}

func (e *Emitter) instr(val ir.Val, ident string, kind int) *ir.Instr {
	e.env.Defs[ident] = val.Type()
	it := &ir.Instr{
		Ident: ident,
		Kind:  kind,
		Val:   val,
	}
	e.scope.blk.Ins = append(e.scope.blk.Ins, it)
	return it
}

func (e *Emitter) genID() string {
	e.count++
	return ir.GenVarIdent(e.count)
}

func (e *Emitter) emitType(node ast.Expr) types.ValType {
	primitiveMap := map[string]types.ValType{
		"unit":  types.Unit,
		"int":   types.Int,
		"uint":  types.Unit,
		"float": types.Float,
		"bool":  types.Bool,
	}

	if v, ok := node.(*ast.VarRef); ok {
		if t, ok := primitiveMap[v.Symbol.Name]; ok {
			return t
		}
		return &types.TypeVar{Name: v.Symbol.Name}
	}

	switch n := node.(type) {
	case *ast.CtorType:
		switch n.Ctor.Name {
		case "array":
			ele := e.emitType(n.ParamTypes[0].(ast.Expr))
			size := n.ParamTypes[1].(*ast.Int).Value
			return &types.Arr{
				Ele:  ele,
				Size: int(size),
			}
		case "rec":
			var typeVars []*types.TypeVar
			var keys []string
			var memTps []types.ValType
			for _, a := range n.TpParams {
				typeVars = append(typeVars, &types.TypeVar{Name: a.Name})
			}
			for _, p := range n.ParamTypes {
				param := p.(ast.Param)
				keys = append(keys, param.Ident.Name)
				memTps = append(memTps, e.emitType(param.Type))
			}
			types.TpUidCounter++
			return &types.Rec{
				Uid:    types.TpUidCounter,
				Keys:   keys,
				MemTps: memTps,
				TpVars: typeVars,
			}
		default:
			if t, ok := e.env.Types[n.Ctor.Name]; ok {
				var tpArgs []types.ValType
				for _, p := range n.ParamTypes {
					tpArgs = append(tpArgs, e.emitType(p))
				}
				t, err := types.SubstRoot(t, tpArgs)
				if err != nil {
					panic(err)
				}
				return t
			}
			if t, ok := primitiveMap[n.Ctor.Name]; ok {
				return t
			}
			return &types.TypeVar{Name: n.Ctor.Name}
		}
	}
	panic(fmt.Sprintf("unsupported type: %+v\n", node))
}

func linkBB(src, dest *ir.Block) {
	src.Dest = append(src.Dest, dest)
	dest.Src = append(dest.Src, src)
}
