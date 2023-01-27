package ir

import (
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
	Block struct {
		Name string
		Ins  []*Instr
	}

	// TODO: Val 增加一个例如Code接口，用于区分不同的Val
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

	Loop struct {
		ItIdent  string
		From, To string
		Body     *Block
	}

	Fun struct {
		Params      []string
		Body        *Block
		Tp          types.ValType
		IsRecursive bool
	}

	Call struct {
		Name string
		Tp   types.ValType
		Args []string
	}

	Instr struct {
		Ident string
		Kind  int
		Val   Val
	}

	ArrMake struct {
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
)

const (
	ConstKind = iota
	RefKind
	RValKind
	BlockKind
	IfKind
	FuncKind
	CallKind
)

var _ Val = (*Const)(nil)
var _ Val = (*Expr)(nil)

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
	return "If " + e.Cond + " Then\n" + e.Then.String() + "\nElse\n" + e.Else.String()
}

func (e *Loop) Kind() int {
	return IfKind
}

func (e *Loop) Type() types.ValType {
	return e.Body.Type()
}

func (e *Loop) String() string {
	return "Loop " + e.To + " Then\n" + e.Body.String()
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

func (e *ArrMake) Kind() int {
	return CallKind
}

func (e *ArrMake) Type() types.ValType {
	return &types.Arr{Ele: e.Tp, Size: len(e.Args)}
}

func (e *ArrMake) String() string {
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

func (e *Fun) Kind() int {
	return FuncKind
}

func (e *Fun) Type() types.ValType {
	return e.Tp
}

func (e *Fun) String() string {
	return "Fun(" + strings.Join(e.Params, ", ") + ") " + e.Body.String()
}

func (it *Instr) String() string {
	return it.Ident + " = " + it.Val.String()
}
