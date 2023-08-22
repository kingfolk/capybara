package types

import (
	"github.com/kingfolk/capybara/errors"
)

func TypeCheckApp(t *Func, tpArgs []ValType, args []ValType) (*Func, error) {
	tr, err := SubstRoot(t, tpArgs)
	if err != nil {
		panic(err)
	}
	t = tr.(*Func)
	var params []ValType = t.Params
	for i, argTp := range args {
		if err := TypeCompatible(params[i], argTp); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func TypeCheckRecLit(t *Rec, tpArgs []ValType, args []ValType) (*Rec, error) {
	tr, err := SubstRoot(t, tpArgs)
	if err != nil {
		panic(err)
	}
	t = tr.(*Rec)
	var params []ValType = t.MemTps
	for i, argTp := range args {
		if err := TypeCompatible(params[i], argTp); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func TypeCheckMutate(left, right ValType) error {
	right, err := SubstRoot(right, nil)
	if err != nil {
		return err
	}
	return TypeCompatible(left, right)
}

// TypeCompatible mainly test if t1 can as a container to receive t2
func TypeCompatible(t1, t2 ValType) error {
	// int and simple enum are compatible
	if (t1.Code() == TpInt && t2.Code() == TpEnum && t2.(*Enum).Simple) || (t2.Code() == TpInt && t1.Code() == TpEnum && t1.(*Enum).Simple) {
		return nil
	}

	switch t1.Code() {
	case TpVar:
		// if t1 is TpVar, a universal container for any other type
		if t2.Code() == t1.Code() && t1.(*TypeVar).Name == t2.(*TypeVar).Name {
			return nil
		}
		return errors.NewError(errors.TYPE_INCOMPATIBLE_TPVAR, "type var "+t1.String()+" and "+t2.String()+" not compatible")
	case TpBool, TpInt, TpFloat:
		if t2.Code() == t1.Code() && t1 == t2 {
			return nil
		}
		return errors.NewError(errors.TYPE_INCOMPATIBLE_PRIMITIVE, t1.String()+" and "+t2.String()+" not compatible")
	case TpRec:
		t1r, t2r := t1.(*Rec), t2.(*Rec)
		if t2.Code() != t1.Code() || t1r.Uid != t2r.Uid || len(t1r.Substs) != len(t2r.Substs) {
			return errors.NewError(errors.TYPE_INCOMPATIBLE_RECORD, "record "+t1.String()+" and "+t2.String()+" not compatible")
		}
		for i, s1 := range t1r.Substs {
			if err := TypeCompatible(s1, t2r.Substs[i]); err != nil {
				return errors.NewError(errors.TYPE_INCOMPATIBLE_RECORD, "record "+t1.String()+" and "+t2.String()+" invoke with incompatible type argument")
			}
		}
		return nil
	case TpEnum:
		if t1.Code() != t2.Code() {
			return errors.NewError(errors.TYPE_INCOMPATIBLE_ENUM, "enum "+t1.String()+" and "+t2.String()+" not compatible")
		}
		if t1.(*Enum).Uid != t2.(*Enum).Uid {
			return errors.NewError(errors.TYPE_INCOMPATIBLE_ENUM, "enum "+t1.String()+" and "+t2.String()+" not compatible")
		}
	case TpArr:
		// TODO not handle array type check for the moment
		return nil
	}
	return errors.NewError(errors.INTERNAL_ERROR, "unhandled type compatible check")
}

func HasTpVar(t ValType) bool {
	if t.Code() == TpVar {
		return true
	}
	return HasPartialTpVar(t)
}

func HasPartialTpVar(t ValType) bool {
	var walk func(tt ValType) bool
	walk = func(tt ValType) bool {
		switch tp := tt.(type) {
		case *TypeVar:
			return true
		case *Func:
			for _, arg := range tp.Params {
				if walk(arg) {
					return true
				}
			}
			return walk(tp.Ret)
		case *Rec:
			for _, arg := range tp.MemTps {
				if walk(arg) {
					return true
				}
			}
		}
		return false
	}
	if t.Code() == TpVar {
		return false
	}
	return walk(t)
}

func SubstRoot(t ValType, tpArgs []ValType) (ValType, error) {
	var tpVars []*TypeVar
	switch tp := t.(type) {
	case *Func:
		tpVars = tp.TpVars
	case *Rec:
		tpVars = tp.TpVars
	case *Enum:
		tpVars = tp.TpVars
	}
	if len(tpArgs) != len(tpVars) {
		return nil, errors.NewError(errors.TYPE_SUBSTITUTE_NUM_MISMATCH, "invoke type arguments more or less than defined type parameters")
	}
	// nothing to substitute
	if len(tpArgs) == 0 {
		return t, nil
	}
	set := map[string]ValType{}
	for i, tpArg := range tpArgs {
		set[tpVars[i].Name] = tpArg
	}

	return Subst(t, set, tpArgs), nil
}

func Subst(t ValType, set map[string]ValType, tpArgs []ValType) ValType {
	if set == nil {
		set = map[string]ValType{}
	}
	switch tp := t.(type) {
	case *TypeVar:
		s, ok := set[tp.Name]
		if ok {
			return s
		}
		return t
	case *Func:
		return &Func{
			Ret:    Subst(tp.Ret, set, tpArgs),
			Params: SubstList(tp.Params, set, tpArgs),
		}
	case *Rec:
		tpVars := tp.TpVars
		tr := &Rec{
			Uid:    tp.Uid,
			Keys:   tp.Keys,
			MemTps: SubstList(tp.MemTps, set, tpArgs),
			TpVars: tpVars,
			Substs: tpArgs,
		}
		return tr
	case *Enum:
		return &Enum{
			Uid:    tp.Uid,
			Simple: tp.Simple,
			Tokens: tp.Tokens,
			TpVars: tp.TpVars,
			Tps:    SubstList(tp.Tps, set, tpArgs),
		}
	}
	return t
}

func SubstList(ts []ValType, set map[string]ValType, tpArgs []ValType) []ValType {
	res := make([]ValType, len(ts))
	for i, t := range ts {
		res[i] = Subst(t, set, tpArgs)
	}
	return res
}
