package ir

import (
	"strconv"
	"strings"

	"github.com/kingfolk/capybara/types"
)

type OperatorKind int

const (
	ADD OperatorKind = iota
	SUB
	MUL
	DIV
	MOD
	LT
	LTE
	GT
	GTE
	AND
	OR
	EQ
	NEQ
)

var OpKindString = map[OperatorKind]string{
	ADD: "+",
	SUB: "-",
	MUL: "*",
	DIV: "/",
	MOD: "%",
	LT:  "<",
	LTE: "<=",
	GT:  ">",
	GTE: ">=",
	AND: "&&",
	OR:  "||",
	EQ:  "==",
	NEQ: "!=",
}

type (
	Module struct {
		Root *Block
		// Env root scope env
		Env   *types.Env
		Funcs []*Func
	}

	Block struct {
		Id   int
		Name string
		Ins  []*Instr

		Src  []*Block
		Dest []*Block
		dom  domInfo
	}

	Val interface {
		Kind() int
		Type() types.ValType
		String() string
	}

	Expr struct {
		tp   types.ValType
		Op   OperatorKind
		Args []string
	}

	Const struct {
		tp  types.ValType
		val []byte
	}

	Ref struct {
		tp    types.ValType
		Ident string
	}

	If struct {
		Cond string
		Then *Block
		Else *Block
	}

	Func struct {
		Params      []string
		Body        *Block
		Tp          types.ValType
		IsRecursive bool
		Defs        map[string]types.ValType
	}

	Call struct {
		Name  string
		Tp    types.ValType
		Args  []string
		Boxes []types.ValType
	}

	Phi struct {
		Orig  string
		Tp    types.ValType
		Edges []string
	}

	Instr struct {
		Ident string
		Kind  int
		Val   Val
	}

	ArrLit struct {
		Tp   types.ValType
		Args []string
	}

	ArrGet struct {
		Tp         types.ValType
		Arr, Index string
	}

	ArrPut struct {
		Arr, Index, Right string
	}

	RecLit struct {
		Tp   *types.Rec
		Args []string
	}

	RecAcs struct {
		Tp  types.ValType
		Rec string
		Idx int
	}
)

const (
	ConstKind = iota
	RefKind
	RValKind
	BlockKind
	IfKind
	FuncKind
	CallKind
	PhiKind
)

var _ Val = (*Const)(nil)
var _ Val = (*Expr)(nil)

func NewUnit() *Const {
	return &Const{
		tp: types.Unit,
	}
}

func NewConst(tp types.ValType, val []byte) *Const {
	return &Const{
		tp:  tp,
		val: val,
	}
}

func NewRef(tp types.ValType, ident string) *Ref {
	return &Ref{
		tp:    tp,
		Ident: ident,
	}
}

func NewBinary(op OperatorKind, left, right string, tp types.ValType) *Expr {
	return &Expr{
		Op:   op,
		Args: []string{left, right},
		tp:   tp,
	}
}

func NewBlock(blockId *int, name string) *Block {
	b := &Block{
		Id:   *blockId,
		Name: name,
	}
	*blockId++
	return b
}

func (e *Block) Kind() int {
	return BlockKind
}

func (e *Block) Type() types.ValType {
	return e.Ins[len(e.Ins)-1].Type()
}

func (e *Block) String() string {
	ins := []string{"{"}
	for _, i := range e.Ins {
		ins = append(ins, "  "+i.String())
	}
	ins = append(ins, "}")
	return strings.Join(ins, "\n")
}

func (e *Instr) Type() types.ValType {
	return e.Val.Type()
}

func (e *Expr) Kind() int {
	return RValKind
}

func (e *Expr) Type() types.ValType {
	return e.tp
}

func (e *Expr) String() string {
	if len(e.Args) == 1 {
		return OpKindString[e.Op] + e.Args[0]
	} else if len(e.Args) == 2 {
		return e.Args[0] + OpKindString[e.Op] + e.Args[1]
	}
	return OpKindString[e.Op] + "(" + strings.Join(e.Args, ", ") + ")"
}

func (e *Const) Kind() int {
	return ConstKind
}

func (e *Const) Raw() []byte {
	return e.val
}

func (e *Const) Type() types.ValType {
	return e.tp
}

func (e *Const) String() string {
	if e.tp == types.Unit {
		return "()"
	}
	return string(e.val)
}

func (e *Ref) Kind() int {
	return RefKind
}

func (e *Ref) Type() types.ValType {
	return e.tp
}

func (e *Ref) String() string {
	return e.Ident
}

func (e *If) Kind() int {
	return IfKind
}

func (e *If) Type() types.ValType {
	return e.Then.Type()
}

func (e *If) String() string {
	thenBB := "#bb" + strconv.Itoa(e.Then.Id)
	elseBB := "#bb" + strconv.Itoa(e.Else.Id)
	return "If " + e.Cond + " Then " + thenBB + " Else " + elseBB
}

func (e *Call) Kind() int {
	return CallKind
}

func (e *Call) Type() types.ValType {
	return e.Tp
}

func (e *Call) String() string {
	return e.Name + "(" + strings.Join(e.Args, ", ") + ") "
}

func (e *ArrLit) Kind() int {
	return CallKind
}

func (e *ArrLit) Type() types.ValType {
	return &types.Arr{Ele: e.Tp, Size: len(e.Args)}
}

func (e *ArrLit) String() string {
	return "ArrMake<" + e.Tp.String() + ">(" + strings.Join(e.Args, ", ") + ") "
}

func (e *ArrGet) Kind() int {
	return CallKind
}

func (e *ArrGet) Type() types.ValType {
	return e.Tp
}

func (e *ArrGet) String() string {
	return e.Arr + "[" + e.Index + "]"
}

func (e *ArrPut) Kind() int {
	return CallKind
}

func (e *ArrPut) Type() types.ValType {
	return nil
}

func (e *ArrPut) String() string {
	return e.Arr + "[" + e.Index + "] <- " + e.Right
}

func (e *RecLit) Kind() int {
	return CallKind
}

func (e *RecLit) Type() types.ValType {
	return e.Tp
}

func (e *RecLit) String() string {
	return "RecLit(" + strings.Join(e.Args, ", ") + ") "
}

func (e *RecAcs) Kind() int {
	return CallKind
}

func (e *RecAcs) Type() types.ValType {
	return e.Tp
}

func (e *RecAcs) String() string {
	return e.Rec + "." + strconv.Itoa(e.Idx)
}

func (e *Func) Kind() int {
	return FuncKind
}

func (e *Func) Type() types.ValType {
	return e.Tp
}

func (e *Func) String() string {
	return e.Body.Name + "(" + strings.Join(e.Params, ",") + ")"
}

func (e *Phi) Kind() int {
	return PhiKind
}

func (e *Phi) Type() types.ValType {
	return e.Tp
}

func (e *Phi) String() string {
	edges := ""
	for i, e := range e.Edges {
		if i != 0 {
			edges += ", "
		}
		edges += e
	}
	return "Phi(" + edges + ")"
}

func (it *Instr) String() string {
	return it.Ident + " = " + it.Val.String()
}

func CFGFuncString(fn *Func) string {
	res := CFGString(fn.Body)
	res = strings.ReplaceAll(res, "\n", "\n  ")
	res = "  " + res
	res = fn.String() + "{\n" + res
	res = strings.TrimSpace(res)
	res += "\n}"
	return res
}

func CFGString(bb *Block) string {
	var bbNames = func(prefix string, blocks []*Block) string {
		if len(blocks) == 0 {
			return ""
		}
		var res string
		for i, b := range blocks {
			if i > 0 {
				res += " ,"
			}
			res += "#bb" + strconv.Itoa(b.Id)
		}
		return prefix + res
	}

	visited := map[int]bool{}
	stack := []*Block{bb}
	visited[bb.Id] = true
	var res string
	for len(stack) > 0 {
		top := stack[0]
		stack = stack[1:]
		res += "#bb" + strconv.Itoa(top.Id) + ":" + top.Name + bbNames("; from ", top.Src) + "\n"
		res += top.String() + bbNames("; to ", top.Dest) + "\n\n"

		for _, b := range top.Dest {
			if !visited[b.Id] {
				visited[b.Id] = true
				stack = append(stack, b)
			}
		}
	}

	return res
}
