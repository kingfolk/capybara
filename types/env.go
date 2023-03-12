package types

import (
	"strconv"
	"strings"
)

type (
	Env struct {
		DeclTable map[string]ValType
	}

	ValType interface {
		String() string
		Code() int
	}

	primitiveType struct {
		tp int
	}

	Func struct {
		Ret    ValType
		Params []ValType
	}

	Arr struct {
		Ele  ValType
		Size int
	}
)

const (
	TpUnit = iota
	TpBool
	TpInt
	TpFloat
	TpFunc
	TpArr
)

var (
	Unit  = &primitiveType{tp: TpUnit}
	Int   = &primitiveType{tp: TpInt}
	Float = &primitiveType{tp: TpFloat}
	Bool  = &primitiveType{tp: TpBool}
)

var _ ValType = (*primitiveType)(nil)
var _ ValType = (*Func)(nil)

func (t *primitiveType) String() string {
	switch t.tp {
	case TpInt:
		return "int"
	case TpFloat:
		return "double"
	case TpBool:
		return "bool"
	default:
		panic("unsupported type: " + strconv.Itoa(int(t.tp)))
	}
}

func (t *primitiveType) Code() int {
	return t.tp
}

func (t *Arr) String() string {
	return "vec<" + t.Ele.String() + ", " + strconv.Itoa(t.Size) + ">"
}

func (t *Arr) Code() int {
	return TpArr
}

func (t *Func) String() string {
	params := make([]string, len(t.Params))
	for i, p := range t.Params {
		params[i] = p.String()
	}
	return "(" + strings.Join(params, ", ") + ")" + "->" + t.Ret.String()
}

func (t *Func) Code() int {
	return TpFunc
}
