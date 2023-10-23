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
	case TpUnit, TpBool, TpInt, TpFloat:
		if t2.Code() == t1.Code() && t1 == t2 {
			return nil
		}
		return errors.NewError(errors.TYPE_INCOMPATIBLE_PRIMITIVE, t1.String()+" and "+t2.String()+" not compatible")
	case TpRec:
		if t2.Code() != t1.Code() || t1.(*Rec).Uid != t2.(*Rec).Uid || len(t1.(*Rec).Substs) != len(t2.(*Rec).Substs) {
			return errors.NewError(errors.TYPE_INCOMPATIBLE_RECORD, "record "+t1.String()+" and "+t2.String()+" not compatible")
		}
		t1r, t2r := t1.(*Rec), t2.(*Rec)
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
	case TpTrait:
		if t1.Code() == t2.Code() && t1.(*Trait).Uid == t2.(*Trait).Uid {
			return nil
		}
		var impls map[string]*Func
		if t2.Code() == TpTrait {
			impls = map[string]*Func{}
			for i, k := range t2.(*Trait).Keys {
				impls[k] = t2.(*Trait).Fns[i]
			}
		} else {
			impls = t2.Impls().Fns
		}
		tt := t1.(*Trait)
		if len(impls) < len(tt.Fns) {
			return errors.NewError(errors.TYPE_INCOMPATIBLE_TRAIT, "trait "+t1.String()+" and "+t2.String()+" not compatible")
		}
		for i, k := range tt.Keys {
			traitFn := tt.Fns[i]
			rightFn, ok := impls[k]
			if !ok {
				return errors.NewError(errors.TYPE_INCOMPATIBLE_TRAIT, "trait "+t1.String()+" and "+t2.String()+" not compatible. missing fun: "+k)
			}
			if len(traitFn.Params) != len(rightFn.Params) {
				return errors.NewError(errors.TYPE_INCOMPATIBLE_TRAIT, "trait "+t1.String()+" and "+t2.String()+" not compatible. fun "+k+" params not compatible")
			}
			for i := 1; i < len(traitFn.Params); i++ {
				if err := TypeCompatible(traitFn.Params[i], rightFn.Params[i]); err != nil {
					return err
				}
			}
			if err := TypeCompatible(traitFn.Ret, rightFn.Ret); err != nil {
				return err
			}
		}
		return nil
	case TpArr:
		// TODO not handle array type check for the moment
		return nil
	}
	return errors.NewError(errors.INTERNAL_ERROR, "unhandled type compatible check left: "+t1.String()+". right: "+t2.String())
}

func HasTpVar(t ValType) bool {
	if t.Code() == TpVar {
		return true
	}
	return HasPartialTpVar(t)
}

func CollectTpVar(t ValType) map[string]*TypeVar {
	set := map[string]*TypeVar{}
	var walk func(tt ValType)
	walk = func(tt ValType) {
		switch tp := tt.(type) {
		case *TypeVar:
			set[tp.Name] = tp
		case *Func:
			for _, arg := range tp.Params {
				walk(arg)
			}
			walk(tp.Ret)
		case *Rec:
			for _, arg := range tp.MemTps {
				walk(arg)
			}
		case *Enum:
			for _, arg := range tp.Tps {
				walk(arg)
			}
		case *Trait:
			for _, arg := range tp.Fns {
				walk(arg)
			}
		}
	}
	walk(t)
	return set
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
			if len(tp.TpVars) > 0 {
				return true
			}
			for _, arg := range tp.MemTps {
				if walk(arg) {
					return true
				}
			}
		case *Trait:
			if len(tp.TpVars) > 0 {
				return true
			}
			for _, fn := range tp.Fns {
				for _, arg := range fn.Params {
					if arg.Code() == TpTrait && arg.(*Trait).Uid == tp.Uid {
						continue
					}
					if walk(arg) {
						return true
					}
				}
				if fn.Ret.Code() == TpTrait && fn.Ret.(*Trait).Uid == tp.Uid {
					return false
				}
				return walk(fn.Ret)
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
	case *Trait:
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

	return Subst(t, set)
}

func Subst(t ValType, set map[string]ValType) (ValType, error) {
	if set == nil {
		set = map[string]ValType{}
	}
	switch tp := t.(type) {
	case *TypeVar:
		s, ok := set[tp.Name]
		if ok {
			if tp.Lower != nil {
				if err := TypeCompatible(tp.Lower, s); err != nil {
					return nil, err
				}
			}
			return s, nil
		}
		return t, nil
	case *Func:
		ret, err := Subst(tp.Ret, set)
		if err != nil {
			return nil, err
		}
		tps, err := SubstList(tp.Params, set)
		if err != nil {
			return nil, err
		}
		return &Func{
			Uid:    tp.Uid,
			Ret:    ret,
			Params: tps,
		}, nil
	case *Rec:
		var substs []ValType
		var tpVars []*TypeVar
		for _, tv := range tp.TpVars {
			subst := set[tv.Name]
			substs = append(substs, subst)
			if subst.Code() == TpVar {
				tpVars = append(tpVars, subst.(*TypeVar))
			} else {
				tpVars = append(tpVars, tv)
			}
		}
		tps, err := SubstList(tp.MemTps, set)
		if err != nil {
			return nil, err
		}
		tr := &Rec{
			ImplBundle: tp.ImplBundle,
			Uid:        tp.Uid,
			Keys:       tp.Keys,
			MemTps:     tps,
			TpVars:     tpVars,
			Substs:     substs,
		}
		return tr, nil
	case *Enum:
		tps, err := SubstList(tp.Tps, set)
		if err != nil {
			return nil, err
		}
		return &Enum{
			ImplBundle: tp.ImplBundle,
			Uid:        tp.Uid,
			Simple:     tp.Simple,
			Tokens:     tp.Tokens,
			TpVars:     tp.TpVars,
			Tps:        tps,
		}, nil
	case *Trait:
		var tpVars []*TypeVar
		for _, tv := range tp.TpVars {
			subst := set[tv.Name]
			if subst == nil {
				panic("todo")
			}
			if tv.Lower != nil {
				if err := TypeCompatible(tv.Lower, subst); err != nil {
					return nil, err
				}
			}
			if subst.Code() == TpVar {
				tpVars = append(tpVars, subst.(*TypeVar))
			} else {
				tpVars = append(tpVars, tv)
			}
		}
		trait := &Trait{
			Uid:    tp.Uid,
			Keys:   tp.Keys,
			TpVars: tpVars,
		}
		var fns []*Func
		for _, fn := range tp.Fns {
			ret, err := Subst(fn.Ret, set)
			if err != nil {
				return nil, err
			}
			ps, err := SubstList(fn.Params[1:], set)
			if err != nil {
				return nil, err
			}
			fn = &Func{
				Uid:    fn.Uid,
				Ret:    ret,
				Params: append([]ValType{trait}, ps...),
			}
			fns = append(fns, fn)
		}
		trait.Fns = fns
		return trait, nil
	}
	return t, nil
}

func SubstList(ts []ValType, set map[string]ValType) ([]ValType, error) {
	res := make([]ValType, len(ts))
	var err error
	for i, t := range ts {
		res[i], err = Subst(t, set)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}
