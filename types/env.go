package types

import (
	"strconv"
)

type (
	TpBox []bool

	Env struct {
		Types map[string]ValType
		Defs  map[string]ValType
	}

	ImplBundle struct {
		Prefix string
		Fns    map[string]*Func
	}

	VoidImplBundle struct{}

	ValType interface {
		String() string
		Code() int
		Impls() *ImplBundle
	}

	primitiveType struct {
		VoidImplBundle
		tp int
	}

	Func struct {
		VoidImplBundle
		Uid    uint64
		Ret    ValType
		Params []ValType
		TpVars []*TypeVar
	}

	Arr struct {
		VoidImplBundle
		Ele  ValType
		Size int
	}

	Rec struct {
		ImplBundle
		Uid    uint64
		Keys   []string
		MemTps []ValType
		TpVars []*TypeVar
		Substs []ValType
	}

	Enum struct {
		ImplBundle
		Uid    uint64
		Simple bool
		Tokens []string
		TpVars []*TypeVar
		Tps    []ValType
	}

	Trait struct {
		VoidImplBundle
		Uid    uint64
		Keys   []string
		Fns    []*Func
		TpVars []*TypeVar
	}

	Symbol struct {
		VoidImplBundle
		Uid  uint64
		Name string
	}

	TypeVar struct {
		VoidImplBundle
		Name  string
		Lower ValType
	}

	App struct {
		VoidImplBundle
		TpCon  ValType
		TpArgs []ValType
		Args   []ValType
	}
)

const (
	TpUnit = iota
	TpBool
	TpInt
	TpFloat
	TpVoidPtr
	TpVar
	TpArr
	TpRec
	TpEnum
	TpTrait
	TpSym
	TpFunc
	TpApp
)

var (
	VoidP = &primitiveType{tp: TpVoidPtr}
	Unit  = &primitiveType{tp: TpUnit}
	Int   = &primitiveType{tp: TpInt}
	Float = &primitiveType{tp: TpFloat}
	Bool  = &primitiveType{tp: TpBool}

	TpUidCounter uint64
)

var _ ValType = (*primitiveType)(nil)
var _ ValType = (*Func)(nil)
var _ ValType = (*App)(nil)
var _ ValType = (*TypeVar)(nil)
var _ ValType = (*Arr)(nil)
var _ ValType = (*Rec)(nil)
var _ ValType = (*Enum)(nil)
var _ ValType = (*Trait)(nil)
var _ ValType = (*Symbol)(nil)

func IsPrimitive(t ValType) bool {
	_, ok := t.(*primitiveType)
	return ok
}

func (e *Env) GetDefTrusted(ident string) ValType {
	t, ok := e.GetDef(ident)
	if !ok {
		panic("undefined ident: " + ident)
	}
	return t
}

func (e *Env) GetDef(ident string) (ValType, bool) {
	t, ok := e.Defs[ident]
	return t, ok
}

func (t *ImplBundle) Impls() *ImplBundle {
	return t
}

func (t VoidImplBundle) Impls() *ImplBundle {
	return nil
}

func (t *primitiveType) String() string {
	switch t.tp {
	case TpUnit:
		return "unit"
	case TpInt:
		return "int"
	case TpFloat:
		return "float"
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
	return "arr<" + t.Ele.String() + ", " + strconv.Itoa(t.Size) + ">"
}

func (t *Arr) Code() int {
	return TpArr
}

func (t *Rec) String() string {
	var str string
	if len(t.TpVars) > 0 {
		str += "<"
		for i, tpVar := range t.TpVars {
			if i > 0 {
				str += ", "
			}
			str += tpVar.String()
		}
		str += ">"
	}
	str += "{"
	for i, key := range t.Keys {
		if i > 0 {
			str += ", "
		}
		str += key + ":" + t.MemTps[i].String()
	}
	return "rec" + str + "}"
}

func (t *Rec) Code() int {
	return TpRec
}

func (t *Rec) KeyIndex(key string) int {
	for i, k := range t.Keys {
		if k == key {
			return i
		}
	}
	return -1
}

func (t *Enum) Code() int {
	return TpEnum
}

func (t *Enum) String() string {
	var str string
	if len(t.TpVars) > 0 {
		str += "<"
		for i, tpVar := range t.TpVars {
			if i > 0 {
				str += ", "
			}
			str += tpVar.String()
		}
		str += ">"
	}
	str += "("
	for i, tp := range t.Tps {
		if i > 0 {
			str += ", "
		}
		str += tp.String()
	}
	return "enum" + str + ")"
}

func (t *Enum) KeyIndex(key string) (int, bool) {
	for i, k := range t.Tokens {
		if k == key {
			return i, true
		}
	}
	return -1, false
}

func (t *Trait) String() string {
	str := "trait"
	if len(t.TpVars) > 0 {
		str += "<"
		for i, tpVar := range t.TpVars {
			if i > 0 {
				str += ", "
			}
			str += tpVar.String()
		}
		str += ">"
	}
	str += "{"
	for i, k := range t.Keys {
		if i > 0 {
			str += ", "
		}
		str += k
	}
	return str + "}"
}

func (t *Trait) Code() int {
	return TpTrait
}

func (t *Symbol) Code() int {
	return TpSym
}

func (t *Symbol) String() string {
	return "sym(" + t.Name + ")"
}

func (t *Func) String() string {
	var str string
	if len(t.TpVars) > 0 {
		str += "<"
		for i, tpVar := range t.TpVars {
			if i > 0 {
				str += ", "
			}
			str += tpVar.String()
		}
		str += ">"
	}
	str += "("
	for i, p := range t.Params {
		if i > 0 {
			str += ", "
		}
		str += p.String()
	}
	return str + ")" + "->" + t.Ret.String()
}

func (t *Func) Code() int {
	return TpFunc
}

func (t *TypeVar) String() string {
	str := "'" + t.Name
	if t.Lower == nil {
		return str
	}
	return str + "<:" + t.Lower.String()
}

func (t *TypeVar) Code() int {
	return TpVar
}

func (t *App) String() string {
	var str string
	if len(t.TpArgs) > 0 {
		str += "<"
		for i, tpVar := range t.TpArgs {
			if i > 0 {
				str += ", "
			}
			str += tpVar.String()
		}
		str += ">"
	}
	for i, p := range t.Args {
		if i > 0 {
			str += ", "
		}
		str += p.String()
	}
	return "(" + t.TpCon.String() + ")" + str
}

func (t *App) Code() int {
	return TpApp
}

func (t *App) Impls() *ImplBundle {
	return nil
}
