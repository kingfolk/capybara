package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kingfolk/capybara/codegen"
	"github.com/kingfolk/capybara/ir"
	"github.com/kingfolk/capybara/syntax"
	"github.com/kingfolk/capybara/types"
	"github.com/llvm/llvm-project/bindings/go/llvm"

	"github.com/kingfolk/capybara/semantics"
	"github.com/rhysd/locerr"
)

func main() {
	args := os.Args[1:]
	if len(args) < 2 {
		panic("illegal argument. should have `cbentry $action \"fun f1(): int = {1 + 2}; f1()\"`, $action should be one of run, bb, llvm")
	}
	query := args[1]
	var result []byte
	switch strings.ToLower(args[0]) {
	case "run":
		output, err := run(query)
		if err != nil {
			result = []byte("Error: " + err.Error())
		} else {
			result = []byte(output)
		}
		fmt.Println("<<< output, err", output, err)
	case "bb":
		bbs, err := bbs(query)
		if err != nil {
			result = []byte("Error: " + err.Error())
		} else {
			result = []byte(bbs)
		}
		fmt.Println("<<< irs, err", bbs, err)
	case "llvm":
		irs, err := llvmir(query)
		if err != nil {
			result = []byte("Error: " + err.Error())
		} else {
			result = []byte(irs)
		}
		fmt.Println("<<< bbs, err", irs, err)
	}

	if len(args) == 3 {
		err := os.WriteFile(args[2], result, 0644)
		if err != nil {
			panic(err)
		}
	}
}

func emit(raw string) (*ir.Module, error) {
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		return nil, err
	}
	var globals []semantics.GlobalDef
	mod, _, err := semantics.EmitIR(node, false, globals...)
	return mod, err
}

func run(raw string) (string, error) {
	mod, err := emit(raw)
	if err != nil {
		return "", err
	}

	jitAnon := codegen.BuildModule(mod, false)
	res := codegen.RunJit(jitAnon, nil)
	if len(mod.Root.Ins) == 0 {
		return "nothing to run", nil
	}
	ins := mod.Root.Ins[len(mod.Root.Ins)-1]
	switch ins.Type() {
	case types.Int:
		return strconv.FormatInt(int64(res.Int(true)), 10), nil
	case types.Float:
		return strconv.FormatFloat(res.Float(llvm.FloatType()), 'g', -1, 64), nil
	case types.Unit:
		return "unit /* last instruction of top level maybe a function. run it if you want to execute the function */", nil
	default:
		return "unsupported type: " + ins.Type().String(), nil
	}
}

func bbs(raw string) (string, error) {
	mod, err := emit(raw)
	if err != nil {
		return "", err
	}

	var actualBb string
	actualBb += ir.CFGString(mod.Root)
	for _, fn := range mod.Funcs {
		actualBb += ir.CFGFuncString(fn)
		actualBb += "\n"
	}
	return actualBb, nil
}

func llvmir(raw string) (string, error) {
	mod, err := emit(raw)
	if err != nil {
		return "", err
	}

	codegen.BuildModule(mod, false)
	return codegen.GetRootModule().String(), nil
}
