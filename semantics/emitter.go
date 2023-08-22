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

	retTp := blk.Ins[len(blk.Ins)-1].Type()
	e.insertReturn(blk, retTp)
	maker := ir.NewDominatorMaker(blk, e.debug)
	declTable := maker.Lift(e.env.Defs)
	root = e.module
	root.Env = e.env
	root.Env.Defs = declTable
	em = e

	if e.debug {
		for _, f := range root.Funcs {
			fmt.Println("--- lifted bb " + f.String() + "---")
			fmt.Println(ir.CFGFuncString(f))
			fmt.Println("--- lifted bb end ---")
		}
	}

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
	case *ast.DotAcs:
		return e.emitDotAcsInsn(n)
	case *ast.Apply:
		return e.emitAppInsn(n)
	case *ast.If:
		return e.emitIfInsn(n)
	case *ast.Loop:
		return e.emitLoopInsn(n)
	case *ast.Match:
		return e.emitMatchInsn(n)
	case *ast.Let:
		return e.emitLetInsn(n)
	case *ast.Mutate:
		return e.emitMutateInsn(n)
	case *ast.LetRec:
		return e.emitFuncInsn(n)
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

func (e *Emitter) registerDecl(name, bound string) {
	_, ok := e.scope.vars[name]
	if ok {
		panic("TypeError: re-declaration of " + name)
	}
	e.scope.vars[name] = bound
}

func (e *Emitter) emitLetInsn(node *ast.Let) *ir.Instr {
	if node.Bound == nil {
		bound := e.emitInsn(&ast.Unit{})
		e.registerDecl(node.Symbol.Name, bound.Ident)
		tp := e.emitType(node.Type)
		e.env.Defs[node.Symbol.Name] = tp
		e.env.Defs[bound.Ident] = tp
		return bound
	}
	bound := e.emitInsn(node.Bound)
	e.registerDecl(node.Symbol.Name, bound.Ident)
	if i, ok := bound.Val.(*ir.If); ok {
		e.mutateIdentEndOfBlock(bound.Ident, i.Then, i.Else)
	}

	if node.Type != nil {
		tp := e.emitType(node.Type)
		e.env.Defs[node.Symbol.Name] = tp
		if tp.Code() == types.TpTrait {
			bound = e.emitBoxTrait(bound.Ident, tp.(*types.Trait))
		}
		right := e.env.GetDefTrusted(bound.Ident)
		if err := types.TypeCompatible(tp, right); err != nil {
			panic(err)
		}
	}
	return bound
}

func (e *Emitter) emitBox(target string, tp, boxTp types.ValType) (*ir.Instr, *ir.Box) {
	val := &ir.Box{
		Tp:     tp,
		BoxTp:  boxTp,
		Target: target,
	}
	return e.rvalInstr(val), val
}

func (e *Emitter) emitBoxTrait(target string, tp *types.Trait) *ir.Instr {
	val := &ir.BoxTrait{
		Tp:     tp,
		Target: target,
	}
	return e.rvalInstr(val)
}

func (e *Emitter) emitUnbox(target string, tp, boxTp types.ValType) *ir.Instr {
	unbox := &ir.Unbox{
		Tp:     tp,
		BoxTp:  boxTp,
		Target: target,
	}
	return e.rvalInstr(unbox)
}

func (e *Emitter) emitMutateInsn(node *ast.Mutate) *ir.Instr {
	right := e.emitInsn(node.Right)
	ident, ok := e.scope.vars[node.Ref.Symbol.Name]
	if !ok {
		panic("TypeError: undeclared of " + node.Ref.Symbol.Name)
	}
	tp := e.env.GetDefTrusted(ident)
	rightTp := e.env.GetDefTrusted(right.Ident)
	if err := types.TypeCompatible(tp, rightTp); err != nil {
		panic(err)
	}
	if tp.Code() == types.TpTrait {
		right = e.emitBoxTrait(right.Ident, tp.(*types.Trait))
	}
	if i, ok := right.Val.(*ir.If); ok {
		e.mutateIdentEndOfBlock(right.Ident, i.Then, i.Else)
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

func (e *Emitter) emitMatchInsn(n *ast.Match) *ir.Instr {
	target := e.emitInsn(n.Target)
	targetTp := target.Type().(*types.Enum)

	var unwrap *ir.Instr
	if !targetTp.Simple {
		targetUnwrap := &ir.RecAcs{
			Tp:     targetTp,
			Target: target.Ident,
			Idx:    1,
		}
		unwrap = e.rvalInstr(targetUnwrap)
	}

	targetIr := e.formDiscriminant(target, targetTp)

	var emitThenBlock = func(tp types.ValType, cv *ast.DotAcs, body []ast.Expr) {
		varTp, ok := tp.(*types.Rec)
		if ok {
			dot := cv.Dot.(*ast.Apply)
			if len(varTp.Keys) != len(dot.Args) {
				panic(errors.NewError(errors.TYPE_RECORD_NOT_FULFILLED, "enum match literal not fulfilled."))
			}
			unboxIr := e.emitUnbox(unwrap.Ident, tp, &types.TypeVar{Name: "dummy"})
			for i, arg := range dot.Args {
				v, ok := arg.(*ast.VarRef)
				if !ok {
					panic(errors.NewError(errors.TYPE_ENUM_DESTRUCT_ILLEGAL, "enum match destruct illegal"))
				}
				innerAcs := &ir.RecAcs{
					Tp:     varTp.MemTps[i],
					Target: unboxIr.Ident,
					Idx:    i,
				}
				destruct := e.rvalInstr(innerAcs)
				e.registerDecl(v.Symbol.Name, destruct.Ident)
			}
		}
		for _, node := range body {
			e.emitInsn(node)
		}
	}

	var firstIf *ir.Instr
	var prevIf *ir.If
	var condBlk *ir.Block = e.scope.blk
	var hasOther bool
	var allBlk []*ir.Block
	for i, c := range n.Cases {
		switch cv := c.Cond.(type) {
		case *ast.DotAcs:
			enumTp, idx, ok := e.resolveEnum(cv)
			if !ok {
				panic(errors.NewError(errors.TYPE_ENUM_UNDEFINED, "enum match undefined"))
			}

			caseBlk := ir.NewBlock(&e.scope.blockId, "case-then-"+strconv.Itoa(i))
			allBlk = append(allBlk, caseBlk)

			e.scope.blk = caseBlk
			emitThenBlock(enumTp.Tps[idx], cv, c.Body)

			ifBlk := ir.NewBlock(&e.scope.blockId, "case-if-"+strconv.Itoa(i))
			linkBB(condBlk, ifBlk)
			linkBB(ifBlk, caseBlk)
			condBlk = ifBlk
			if prevIf != nil {
				prevIf.Else = ifBlk
			}
			e.scope.blk = ifBlk
			discr := ir.NewConst(types.Int, []byte(strconv.FormatInt(int64(idx), 10)))
			discrIr := e.rvalInstr(discr)
			cond := e.rvalInstr(ir.NewBinary(ir.EQ, targetIr.Ident, discrIr.Ident, types.Bool))
			prevIf = &ir.If{
				Cond: cond.Ident,
				Then: caseBlk,
			}
			ifi := e.instr(prevIf, e.genID(), ir.IfKind)
			if firstIf == nil {
				firstIf = ifi
			}
		case *ast.VarRef:
			if cv.Symbol.Name == "_" {
				hasOther = true
				otherBlk := e.emitBlock("case-other", c.Body...)
				allBlk = append(allBlk, otherBlk)
				linkBB(e.scope.blk, otherBlk)
				if prevIf != nil {
					prevIf.Else = otherBlk
				} else {
					panic(errors.NewError(errors.TYPE_ENUM_OTHER_ILLEGAL, "match other should not place at first"))
				}
			} else {
				panic(errors.NewError(errors.TYPE_ENUM_UNDEFINED, "enum match undefined"))
			}
		}
	}

	if !hasOther {
		panic(errors.NewError(errors.TYPE_ENUM_OTHER_ILLEGAL, "missing case _"))
	}

	e.scope.blk = ir.NewBlock(&e.scope.blockId, "match "+target.Ident+" after")
	for _, b := range allBlk {
		linkBB(b, e.scope.blk)
	}
	return firstIf
}

func (e *Emitter) emitFuncInsn(node *ast.LetRec) *ir.Instr {
	origScope := e.scope
	e.scope = NewScope()
	for k := range e.globals {
		e.scope.vars[k] = k
	}

	name := node.Func.Symbol.Name
	paramDefs := node.Func.Params
	var funTp *types.Func
	if node.Func.Rcv != nil {
		paramDefs = append([]ast.Param{*node.Func.Rcv}, paramDefs...)
		rcv, ok := node.Func.Rcv.Type.(*ast.CtorType)
		if !ok {
			panic("unreachable. receiver type must be CtorType")
		}
		tpName := rcv.Ctor.Name
		fnName := name
		name = tpName + "$" + fnName
		rcvTp, ok := e.env.Types[tpName]
		if !ok {
			panic(errors.NewError(errors.TYPE_METHOD_ILLEGAL, "receiver type not found: "+tpName))
		}
		impl := rcvTp.Impls()
		if rcvTp.Impls() == nil {
			panic(errors.NewError(errors.TYPE_METHOD_ILLEGAL, "receiver type cannot be have method: "+tpName))
		}
		defer func() {
			impl.Fns[fnName] = funTp
			if tRec, ok := rcvTp.(*types.Rec); ok {
				funTp.TpVars = append(funTp.TpVars, tRec.TpVars...)
			}
		}()
		impl.Prefix = tpName
	}

	params := make([]string, 0, len(paramDefs))
	paramTypes := make([]types.ValType, len(paramDefs))
	tpVars := make([]*types.TypeVar, len(node.Func.TpParams))
	for i, param := range paramDefs {
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

	types.TpUidCounter++
	funTp = &types.Func{
		Uid:    types.TpUidCounter,
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
	e.insertReturn(blk, funTp.Ret)

	maker := ir.NewDominatorMaker(blk, e.debug, params...)
	defs := maker.Lift(e.env.Defs)
	val.Params = maker.LiftParams
	val.Defs = defs

	e.scope = origScope
	return e.instr(val, name, ir.FuncKind)
}

func (e *Emitter) insertReturn(blk *ir.Block, retTp types.ValType) {
	visited := map[int]bool{}
	stack := []*ir.Block{blk}
	visited[blk.Id] = true
	for len(stack) > 0 {
		top := stack[0]
		visited[top.Id] = true
		stack = stack[1:]
		if len(top.Dest) == 0 {
			e.scope.blk = top
			target := top.Ins[len(top.Ins)-1].Ident
			targetTp := e.env.GetDefTrusted(target)
			if retTp.Code() == types.TpTrait {
				rt := retTp.(*types.Trait)
				tt, ok := targetTp.(*types.Trait)
				if !ok || tt.Uid != rt.Uid {
					boxed := e.emitBoxTrait(target, rt)
					target = boxed.Ident
				}
			}

			ret := &ir.Ret{
				Tp:     retTp,
				Target: target,
			}
			e.rvalInstr(ret)
		}
		for _, d := range top.Dest {
			if !visited[d.Id] {
				stack = append(stack, d)
				visited[d.Id] = true
			}
		}
	}
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
		i := e.emitInsn(arg.Type)
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

var EnumBox *types.Rec

func init() {
	types.TpUidCounter++
	EnumBox = &types.Rec{
		Uid:    types.TpUidCounter,
		Keys:   []string{"0", "1"},
		MemTps: []types.ValType{types.Int, types.VoidP},
	}
}

func (e *Emitter) resolveEnum(node *ast.DotAcs) (*types.Enum, int, bool) {
	if v, ok := node.Expr.(*ast.VarRef); ok {
		if t, ok := e.env.Types[v.Symbol.Name]; ok && t.Code() == types.TpEnum {
			enumTp := t.(*types.Enum)
			var idx int
			switch d := node.Dot.(type) {
			case *ast.VarRef:
				idx, ok = enumTp.KeyIndex(d.Symbol.Name)
				if !ok {
					panic(errors.NewError(errors.TYPE_ENUM_ELE_UNDEFINED, "enum match undefined "+d.Symbol.Name))
				}
			case *ast.Apply:
				vr, ok := d.Callee.(*ast.VarRef)
				if !ok {
					panic("unreachable. Apply after dot can only be VarRef")
				}
				idx, ok = enumTp.KeyIndex(vr.Symbol.Name)
				if !ok {
					panic(errors.NewError(errors.TYPE_ENUM_ELE_UNDEFINED, "enum match undefined "+vr.Symbol.Name))
				}
				var tpArgs []types.ValType
				for _, ta := range d.TpArgs {
					tpArgs = append(tpArgs, e.emitType(ta))
				}
				tp, err := types.SubstRoot(enumTp, tpArgs)
				if err != nil {
					panic(err)
				}
				enumTp = tp.(*types.Enum)
			default:
				panic("unreachable. DotAcs ast illegal")
			}
			return enumTp, idx, true
		}
	}
	return nil, 0, false
}

func (e *Emitter) emitDotAcsInsn(node *ast.DotAcs) *ir.Instr {
	if enumTp, idx, ok := e.resolveEnum(node); ok {
		var op string
		if !enumTp.Simple {
			variant := enumTp.Tps[idx]
			switch variant.(type) {
			case *types.Rec:
				i := e.emitInsn(node.Dot)
				op = i.Ident
			}
		}
		val := &ir.EnumVar{
			Tp:  enumTp,
			Tok: node.Dot.Name(),
			Idx: idx,
			Box: op,
		}
		return e.rvalInstr(val)
	}

	target := e.emitInsn(node.Expr)
	t := e.env.GetDefTrusted(target.Ident)
	var val ir.Val
	switch tp := t.(type) {
	case *types.Rec:
		if vr, ok := node.Dot.(*ast.VarRef); ok {
			idx := tp.KeyIndex(vr.Symbol.Name)
			val = &ir.RecAcs{
				Tp:     tp.MemTps[idx],
				Target: target.Ident,
				Idx:    idx,
			}
		} else if ap, ok := node.Dot.(*ast.Apply); ok {
			vr, ok := ap.Callee.(*ast.VarRef)
			if !ok {
				panic(errors.NewError(errors.TYPE_RECORD_ACS_ILLEGAL, "record access syntax error"))
			}
			var name string
			for k, t := range e.env.Types {
				if tRec, ok := t.(*types.Rec); ok {
					if tRec.Uid == tp.Uid {
						name = k
						break
					}
				}
			}
			if name == "" {
				panic("unreachable. ")
			}
			args := append([]ast.Expr{node.Expr}, ap.Args...)
			tFun, ok := tp.Impls().Fns[vr.Symbol.Name]
			if !ok {
				panic(errors.NewError(errors.TYPE_RECORD_ACS_ILLEGAL, "illegal record access. undefined method"))
			}
			var tpArgs []types.ValType
			for _, tpArg := range ap.TpArgs {
				tpArgs = append(tpArgs, e.emitType(tpArg))
			}
			return e.emitCall(tFun, name+"$"+vr.Symbol.Name, args, append(tpArgs, tp.Substs...))
		} else {
			panic(errors.NewError(errors.TYPE_RECORD_ACS_ILLEGAL, "record access syntax error"))
		}
	case *types.Trait:
		if _, ok := node.Dot.(*ast.VarRef); ok {
			panic(errors.NewError(errors.TYPE_TRAIT_ACS_ILLEGAL, "illegal trait access. missing parenthesis?"))
		} else if ap, ok := node.Dot.(*ast.Apply); ok {
			vr, ok := ap.Callee.(*ast.VarRef)
			if !ok {
				panic(errors.NewError(errors.TYPE_TRAIT_ACS_ILLEGAL, "illegal trait access. trait func call syntax error"))
			}
			args := append([]ast.Expr{node.Expr}, ap.Args...)
			return e.emitTraitAppInsn(vr.Symbol.Name, tp, args)
		} else {
			panic(errors.NewError(errors.TYPE_TRAIT_ACS_ILLEGAL, "illegal trait access. trait func call syntax error"))
		}
	case *types.Enum:
		vr, ok := node.Dot.(*ast.VarRef)
		if !ok {
			panic(errors.NewError(errors.TYPE_ENUM_ELE_UNDEFINED, "enum element illegal"))
		}
		switch vr.Symbol.Name {
		case "discriminant":
			return e.formDiscriminant(target, tp)
		default:
			panic(errors.NewError(errors.TYPE_ENUM_ELE_UNDEFINED, "enum element undefined: "+vr.Symbol.Name))
		}
	default:
		panic("unsupported dot operation for type: " + t.String())
	}

	return e.rvalInstr(val)
}

func (e *Emitter) emitTraitAppInsn(fnName string, t *types.Trait, argNodes []ast.Expr) *ir.Instr {
	var tFun *types.Func
	for i, k := range t.Keys {
		if k == fnName {
			tFun = t.Fns[i]
		}
	}
	if tFun == nil {
		panic(errors.NewError(errors.TYPE_TRAIT_ACS_ILLEGAL, "trait func undefined: "+fnName))
	}
	if len(argNodes) != len(tFun.Params) {
		panic(errors.NewError(errors.TYPE_PARAM_COUNT_WRONG, "call arg count not aligned"))
	}

	args := make([]string, len(argNodes))
	argTps := make([]types.ValType, len(argNodes))
	for i, arg := range argNodes {
		arg := e.emitInsn(arg)
		args[i] = arg.Ident
		argTps[i] = e.env.GetDefTrusted(arg.Ident)
	}

	val := &ir.TraitCall{
		Name:  fnName,
		Trait: t,
		Tp:    tFun.Ret,
		Args:  args,
	}

	return e.rvalInstr(val)
}

func (e *Emitter) emitAppInsn(node *ast.Apply) *ir.Instr {
	ref, ok := node.Callee.(*ast.VarRef)
	if !ok {
		panic("unsupported apply instr")
	}
	tp, ok := e.env.Types[ref.Symbol.Name]
	if ok {
		args := []ast.Param{}
		tr := tp.(*types.Rec)
		if len(node.Args) != len(tr.Keys) {
			panic(errors.NewError(errors.TYPE_RECORD_NOT_FULFILLED, "literal not fulfilled"))
		}
		for i, arg := range node.Args {
			args = append(args, ast.Param{
				Ident: ast.NewSymbol(tr.Keys[i]),
				Type:  arg,
			})
		}
		recLit := &ast.RecLit{
			Ref:    ref,
			TpArgs: node.TpArgs,
			Args:   args,
		}
		return e.emitRecLitInsn(recLit)
	}

	t := e.env.GetDefTrusted(ref.Symbol.Name)
	tFun, ok := t.(*types.Func)
	if !ok {
		panic("APPLY not to func type: " + ref.Symbol.Name)
	}
	var tpArgs []types.ValType
	for _, tpArg := range node.TpArgs {
		tpArgs = append(tpArgs, e.emitType(tpArg))
	}

	return e.emitCall(tFun, ref.Symbol.Name, node.Args, tpArgs)
}

func (e *Emitter) emitCall(tFun *types.Func, fname string, argNodes []ast.Expr, tpArgs []types.ValType) *ir.Instr {
	if len(argNodes) != len(tFun.Params) {
		panic(errors.NewError(errors.TYPE_PARAM_COUNT_WRONG, "call arg count not aligned"))
	}

	args := make([]string, len(argNodes))
	argTps := make([]types.ValType, len(argNodes))
	boxes := make([]*ir.Box, len(argNodes))
	for i, arg := range argNodes {
		arg := e.emitInsn(arg)
		args[i] = arg.Ident
		argTp := e.env.GetDefTrusted(arg.Ident)
		argTps[i] = argTp
		paramTp := tFun.Params[i]
		if paramTp.Code() == types.TpTrait {
			right := e.emitBoxTrait(args[i], paramTp.(*types.Trait))
			args[i] = right.Ident
		} else if types.HasTpVar(paramTp) {
			right, b := e.emitBox(args[i], nil, paramTp)
			args[i] = right.Ident
			boxes[i] = b
		}
	}
	var boxRet types.ValType
	if types.HasTpVar(tFun.Ret) {
		boxRet = tFun.Ret
	}

	tFun, err := types.TypeCheckApp(tFun, tpArgs, argTps)
	if err != nil {
		panic(err)
	}
	for i, box := range boxes {
		if box != nil {
			box.Tp = tFun.Params[i]
		}
	}

	val := &ir.StaticCall{
		Name: fname,
		Tp:   tFun.Ret,
		Args: args,
	}

	fir := e.rvalInstr(val)
	if boxRet != nil {
		fir = e.emitUnbox(fir.Ident, tFun.Ret, boxRet)
	}
	return fir
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

func (e *Emitter) formDiscriminant(target *ir.Instr, tp *types.Enum) *ir.Instr {
	if tp.Simple {
		return target
	}
	val := &ir.RecAcs{
		Tp:     types.Int,
		Target: target.Ident,
		Idx:    0,
	}
	return e.rvalInstr(val)
}

func (e *Emitter) mutateIdentEndOfBlock(ident string, bs ...*ir.Block) {
	for _, b := range bs {
		last := b.Last()
		if i, ok := last.Val.(*ir.If); ok {
			e.mutateIdentEndOfBlock(ident, i.Then)
			e.mutateIdentEndOfBlock(ident, i.Else)
		} else {
			it := &ir.Instr{
				Ident: ident,
				Kind:  ir.RValKind,
				Val:   ir.NewRef(last.Type(), last.Ident),
			}
			b.Ins = append(b.Ins, it)
		}
	}
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
				ImplBundle: types.ImplBundle{
					Fns: map[string]*types.Func{},
				},
				Uid:    types.TpUidCounter,
				Keys:   keys,
				MemTps: memTps,
				TpVars: typeVars,
			}
		case "tup":
			var typeVars []*types.TypeVar
			var keys []string
			var memTps []types.ValType
			for _, a := range n.TpParams {
				typeVars = append(typeVars, &types.TypeVar{Name: a.Name})
			}
			for i, p := range n.ParamTypes {
				keys = append(keys, strconv.Itoa(i))
				memTps = append(memTps, e.emitType(p))
			}
			types.TpUidCounter++
			return &types.Rec{
				ImplBundle: types.ImplBundle{
					Fns: map[string]*types.Func{},
				},
				Uid:    types.TpUidCounter,
				Keys:   keys,
				MemTps: memTps,
				TpVars: typeVars,
			}
		case "enum":
			var typeVars []*types.TypeVar
			var tps []types.ValType
			tvMap := map[string]bool{}
			for _, a := range n.TpParams {
				typeVars = append(typeVars, &types.TypeVar{Name: a.Name})
				tvMap[a.Name] = true
			}
			simple := true
			var tokens []string
			for _, p := range n.ParamTypes {
				ctor, ok := p.(*ast.CtorType)
				if !ok {
					panic("unreachable. enum element must have Ctor type")
				}
				if ctor.Ctor.Name == "rec" || ctor.Ctor.Name == "array" || ctor.Ctor.Name == "tup" {
					panic(errors.NewErrorWithTk(errors.TYPE_ENUM_ELE_ILLEGAL, ctor.Ctor.Name+" not allowed", ctor.StartToken))
				}
				tokens = append(tokens, ctor.Ctor.Name)
				tp := e.emitType(p)
				if tv, ok := tp.(*types.TypeVar); ok && !tvMap[tv.Name] {
					types.TpUidCounter++
					tp = &types.Symbol{
						Uid:  types.TpUidCounter,
						Name: tv.Name,
					}
				} else {
					simple = false
				}
				tps = append(tps, tp)
			}
			types.TpUidCounter++
			return &types.Enum{
				Uid:    types.TpUidCounter,
				Simple: simple,
				Tokens: tokens,
				TpVars: typeVars,
				Tps:    tps,
			}
		case "trait":
			var keys []string
			var fns []*types.Func
			var tpVars []*types.TypeVar
			tpVarSet := map[string]*types.TypeVar{}
			types.TpUidCounter++
			trait := &types.Trait{
				Uid: types.TpUidCounter,
			}
			for _, tpParam := range n.TpParams {
				tpVar := &types.TypeVar{Name: tpParam.Name}
				tpVars = append(tpVars, tpVar)
				tpVarSet[tpVar.Name] = tpVar
			}
			for _, p := range n.ParamTypes {
				ft := p.(ast.Param)
				keys = append(keys, ft.Ident.Name)
				fnTp := ft.Type.(*ast.FuncType)
				paramTps := []types.ValType{trait}
				for _, param := range fnTp.Params {
					paramTp := e.emitType(param.Type)
					paramTps = append(paramTps, paramTp)
				}
				types.TpUidCounter++
				funTp := &types.Func{
					Uid:    types.TpUidCounter,
					Params: paramTps,
					Ret:    e.emitType(fnTp.RetType),
				}
				fnTpVars := types.CollectTpVar(funTp)
				for _, ftv := range fnTpVars {
					if _, ok := tpVarSet[ftv.Name]; !ok {
						panic(errors.NewError(errors.TYPE_TRAIT_TYPE_VAR_UNDEFINED, "undefined type parameter"))
					}
					funTp.TpVars = append(funTp.TpVars, ftv)
				}

				fns = append(fns, funTp)
			}
			trait.Keys = keys
			trait.Fns = fns
			trait.TpVars = tpVars

			return trait
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
