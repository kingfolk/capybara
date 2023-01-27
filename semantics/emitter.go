package semantics

import (
	"fmt"
	"strconv"

	"github.com/kingfolk/capybara/ast"
	"github.com/kingfolk/capybara/ir"
	"github.com/kingfolk/capybara/types"
)

type Scope struct {
	// TODO nested scope
	// parent *Scope
	vars map[string]string
	blk  *ir.Block
}

type Emitter struct {
	count int
	env   *types.Env
	scope *Scope
}

const (
	rootBlock = "$root$"
)

func NewScope() *Scope {
	return &Scope{
		vars: map[string]string{},
		blk: &ir.Block{
			Name: "root",
		},
	}
}

// EmitIR converts given AST into MIR with type environment
func EmitIR(mod *ast.AST) (blk *ir.Block, env *types.Env, err error) {
	e := &Emitter{
		count: 0,
		env: &types.Env{
			DeclTable: map[string]types.ValType{},
		},
		scope: NewScope(),
	}

	defer func() {
		if message := recover(); message != nil {
			// debug.PrintStack()
			err = fmt.Errorf("Emit fail. %v", message)
		}
	}()

	e.emitBlock(rootBlock, mod.Root...)
	return e.scope.blk, e.env, err
}

// EmitIRWithGlobal converts given AST into MIR with type environment
func EmitIRWithGlobal(mod *ast.AST, globalVars map[string]types.ValType) (*ir.Block, *types.Env) {
	e := &Emitter{
		count: 0,
		env: &types.Env{
			DeclTable: map[string]types.ValType{},
		},
		scope: NewScope(),
	}
	for k, t := range globalVars {
		e.env.DeclTable[k] = t
		e.scope.vars[k] = k
	}
	e.emitBlock(rootBlock, mod.Root...)
	return e.scope.blk, e.env
}

func (e *Emitter) emitBlock(name string, nodes ...ast.Expr) *ir.Block {
	reserved := e.scope.blk
	defer func() {
		if name != rootBlock {
			e.scope.blk = reserved
		}
	}()

	e.scope.blk = &ir.Block{
		Name: name,
	}
	for _, node := range nodes {
		e.emitInsn(node)
	}
	return e.scope.blk
}

func (e *Emitter) emitInsn(node ast.Expr) *ir.Instr {
	// fmt.Println("<<< emitInsn", node.Name(), node)
	switch n := node.(type) {
	// case *ast.Unit:
	// 	return e.insn(mir.UnitVal, nil, node)
	case *ast.Int:
		c := ir.NewConst(types.Int, []byte(strconv.FormatInt(n.Value, 10)))
		return e.rvalInstr(c)
	case *ast.Float:
		c := ir.NewConst(types.Float, []byte(strconv.FormatFloat(n.Value, 'g', -1, 64)))
		return e.rvalInstr(c)
	case *ast.VarRef:
		if tp, ok := e.env.DeclTable[n.Symbol.Name]; ok {
			ident := e.scope.vars[n.Symbol.Name]
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
	case *ast.ArrayGet:
		return e.emitArrGetInsn(n)
	case *ast.ArrayPut:
		return e.emitArrPutInsn(n)
	case *ast.Apply:
		return e.emitAppInsn(n)
	case *ast.If:
		return e.emitIfInsn(n)
	case *ast.Loop:
		return e.emitLoopInsn(n)
	case *ast.Let:
		// fmt.Println("*** n.Symbol", n.Symbol)
		// fmt.Println("*** n.Bound", n.Bound, n.Bound.Name())
		// fmt.Println("*** n.Type", n.Type)
		return e.emitLetInsn(n)
	case *ast.LetRec:
		return e.emitFunInsn(n)
	default:
		fmt.Printf(".. node %T\n", node)
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
		panic("TypeError: type mismatch")
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
		e.env.DeclTable[node.Symbol.Name] = tp
	}
	return bound
}

func (e *Emitter) emitIfInsn(n *ast.If) *ir.Instr {
	// TODO: prev muse be bool type
	prev := e.emitInsn(n.Cond)
	thenBlk := e.emitBlock("then", n.Then...)
	elseBlk := e.emitBlock("else", n.Else...)
	val := &ir.If{
		Cond: prev.Ident,
		Then: thenBlk,
		Else: elseBlk,
	}
	i := e.instr(val, e.genID(), ir.IfKind)
	return i
}

func (e *Emitter) emitLoopInsn(n *ast.Loop) *ir.Instr {
	start := e.emitInsn(n.From)
	cond := e.emitInsn(n.To)
	body := e.emitBlock("body", n.Body...)
	val := &ir.Loop{
		ItIdent: n.ItSymbol.Name,
		From:    start.Ident,
		To:      cond.Ident,
		Body:    body,
	}
	return e.instr(val, e.genID(), ir.IfKind)
}

func (e *Emitter) emitFunInsn(node *ast.LetRec) *ir.Instr {
	name := node.Func.Symbol.Name
	// ty, ok := e.env.DeclTable[name]
	// if !ok {
	// 	panic("FATAL: Unknown function: " + name)
	// }

	params := make([]string, 0, len(node.Func.Params))
	paramTypes := make([]types.ValType, len(node.Func.Params))
	for i, param := range node.Func.Params {
		params = append(params, param.Ident.Name)
		tp := e.emitType(param.Type)
		paramTypes[i] = tp
		e.env.DeclTable[param.Ident.Name] = tp
		e.scope.vars[param.Ident.Name] = param.Ident.Name
	}

	blk := e.emitBlock(fmt.Sprintf("body (%s)", name), node.Func.Body...)

	funTp := &types.Fun{
		Params: paramTypes,
		Ret:    e.emitType(node.Func.RetType),
	}

	val := &ir.Fun{
		Params: params,
		Body:   blk,
		Tp:     funTp,
	}

	return e.instr(val, name, ir.FuncKind)

	// body := e.emitInsn(node.Body)
	// e.instr(blk, name, ir.FuncKind, body)

	// return body
}

func (e *Emitter) emitArrLitInsn(node *ast.ArrayLit) *ir.Instr {
	args := make([]string, len(node.Elems))
	for i, ele := range node.Elems {
		arg := e.emitInsn(ele)
		args[i] = arg.Ident
	}
	tp, ok := e.env.DeclTable[args[0]]
	if !ok {
		panic("arg0 not found: " + args[0])
	}
	val := &ir.ArrMake{
		Tp:   tp,
		Args: args,
	}
	return e.rvalInstr(val)
}

func (e *Emitter) emitArrGetInsn(node *ast.ArrayGet) *ir.Instr {
	arr := e.emitInsn(node.Array)
	index := e.emitInsn(node.Index)
	tp, ok := e.env.DeclTable[arr.Ident]
	if !ok {
		panic("arr not found: " + arr.Ident)
	}
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

func (e *Emitter) emitAppInsn(node *ast.Apply) *ir.Instr {
	fmt.Println("----- emitAppInsn")
	ref, ok := node.Callee.(*ast.VarRef)
	if !ok {
		panic("unsupported apply instr")
	}
	t, ok := e.env.DeclTable[ref.Symbol.Name]
	if !ok {
		panic("func not exists: " + ref.Symbol.Name)
	}

	args := make([]string, len(node.Args))
	for i, arg := range node.Args {
		arg := e.emitInsn(arg)
		args[i] = arg.Ident
	}
	val := &ir.Call{
		Name: ref.Symbol.Name,
		Tp:   t.(*types.Fun).Ret,
		Args: args,
	}

	return e.rvalInstr(val)
}

func (e *Emitter) rvalInstr(val ir.Val) *ir.Instr {
	return e.instr(val, e.genID(), ir.RValKind)
}

func (e *Emitter) instr(val ir.Val, ident string, kind int) *ir.Instr {
	e.env.DeclTable[ident] = val.Type()
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
	return "$v" + strconv.Itoa(e.count)
}

func (e *Emitter) emitType(node ast.Expr) types.ValType {
	switch n := node.(type) {
	case *ast.CtorType:
		switch n.Ctor.Name {
		case "array":
			ele := e.emitType(n.ParamTypes[0])
			size := n.ParamTypes[1].(*ast.Int).Value
			return &types.Arr{
				Ele:  ele,
				Size: int(size),
			}
		case "int":
			return types.Int
		case "float":
			return types.Float
		case "bool":
			return types.Bool
		}
	}
	panic(fmt.Sprintf("unsupported type: %+v\n", node))
}
