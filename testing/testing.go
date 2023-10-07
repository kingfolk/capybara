package testing

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/kingfolk/capybara/ast"
	"github.com/kingfolk/capybara/codegen"
	"github.com/kingfolk/capybara/errors"
	"github.com/kingfolk/capybara/ir"
	"github.com/kingfolk/capybara/semantics"
	"github.com/kingfolk/capybara/syntax"
	"github.com/kingfolk/capybara/types"
	"github.com/llvm/llvm-project/bindings/go/llvm"
	"github.com/rhysd/locerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type AssertToken int

const (
	AssertTokenBB = iota
	AssertTokenVal
	AssertTokenAnon
)

type (
	RunResult struct {
		input  []*ir.Const
		output *ir.Const
		outerr errors.ErrorCode
	}

	AssertFrame struct {
		token  AssertToken
		value  string
		result RunResult
		body   string
	}
)

func RunTest(t *testing.T, debug bool, file string) {
	dat, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}

	codegen.Reset()

	raw, setupPairs := parseSetupSection(t, string(dat))
	for _, global := range setupPairs {
		v := llvm.AddGlobal(codegen.GetRootModule(), global.EleTp, global.Name)
		global.Reg = v
	}

	cases := strings.Split(raw, "$$")

	for _, c := range cases {
		c = strings.TrimSpace(c)
		if len(c) == 0 {
			return
		}
		frames := parseAssertSections(c)
		for i, frame := range frames {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				RunCase(t, debug, frame, setupPairs)
			})
		}
	}
}

func emitMod(t *testing.T, debug bool, raw string, setupPairs []*codegen.ExtGlobal) (*ir.Module, *semantics.Emitter, error) {
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	if debug {
		ast.Println(node)
	}
	var globals []semantics.GlobalDef
	for _, p := range setupPairs {
		globals = append(globals, semantics.GlobalDef{
			Name: p.Name,
			Tp:   p.Tp,
		})
	}
	return semantics.EmitIR(node, debug, globals...)
}

func RunCase(t *testing.T, debug bool, frame *AssertFrame, setupPairs []*codegen.ExtGlobal) {
	mod, _, emitErr := emitMod(t, debug, frame.body, setupPairs)
	if emitErr != nil {
		er, ok := emitErr.(errors.LangError)
		if !ok {
			assert.Fail(t, "emit error is not LangError: "+emitErr.Error())
		}
		if frame.result.outerr == 0 {
			assert.Fail(t, "got unexpected error: "+emitErr.Error())
		} else {
			assert.Equal(t, frame.result.outerr, er.Code, "assert error fail. err: "+emitErr.Error())
		}
		return
	}

	var actualBb string
	actualBb += ir.CFGString(mod.Root)
	for _, fn := range mod.Funcs {
		actualBb += ir.CFGFuncString(fn)
		actualBb += "\n"
	}

	var assertResult = func(jitFn llvm.Value) {
		var constToGenericValue = func(r *ir.Const) llvm.GenericValue {
			switch r.Type() {
			case types.Int:
				v, err := strconv.ParseInt(string(r.Raw()), 10, 64)
				if err != nil {
					panic(err)
				}
				return llvm.NewGenericValueFromInt(llvm.Int32Type(), uint64(v), true)
			case types.Float:
				v, err := strconv.ParseFloat(string(r.Raw()), 64)
				if err != nil {
					panic(err)
				}
				return llvm.NewGenericValueFromFloat(llvm.FloatType(), v)
			default:
				panic("unsupported type: " + r.Type().String())
			}
		}
		var args []llvm.GenericValue
		if frame.result.output != nil {
			require.NoError(t, emitErr)
			var expected = constToGenericValue(frame.result.output)
			for _, arg := range frame.result.input {
				args = append(args, constToGenericValue(arg))
			}
			res := codegen.RunJit(jitFn, setupPairs, args...)
			switch frame.result.output.Type() {
			case types.Int:
				assert.Equal(t, int64(expected.Int(true)), int64(res.Int(true)))
			case types.Float:
				assert.Equal(t, int64(expected.Float(llvm.FloatType())), int64(res.Float(llvm.FloatType())))
			default:
				panic("unsupported type: " + frame.result.output.Type().String())
			}
		} else {
			panic("something wrong. error or result assert fail")
		}
	}

	switch frame.token {
	case AssertTokenBB:
		ok := assert.Equal(t, strings.TrimSpace(frame.value), strings.TrimSpace(actualBb))
		if !ok {
			fmt.Println("--- actual bb ---")
			fmt.Println(actualBb)
			fmt.Println("--- actual bb end ---")
		}
	case AssertTokenVal:
		jitFn := codegen.BuildFunc(mod.Funcs[0], debug, setupPairs...)
		assertResult(jitFn)
	case AssertTokenAnon:
		jitAnon := codegen.BuildModule(mod, debug)
		assertResult(jitAnon)
	default:
		panic(fmt.Sprintf("illegal assert token: %+v\n", frame.token))
	}
}

func parseSetupSection(t *testing.T, raw string) (string, []*codegen.ExtGlobal) {
	if raw[:7] != "/*setup" {
		return raw, nil
	}
	end := strings.Index(raw, "*/")
	if end == -1 {
		panic("setup section should format as /*setup ... */")
	}
	restRaw := strings.TrimSpace(raw[end+2:])
	setup := strings.TrimSpace(raw[7:end])
	mod, em, err := emitMod(t, false, setup, nil)
	require.NoError(t, err)
	constMap := map[string]*ir.Const{}
	arrMap := map[string]*ir.ArrLit{}
	for _, ins := range mod.Root.Ins {
		switch v := ins.Val.(type) {
		case *ir.Ref:
			constMap[ins.Ident] = constMap[v.Ident]
		case *ir.Const:
			constMap[ins.Ident] = v
		case *ir.ArrLit:
			arrMap[ins.Ident] = v
		}
	}

	var fromMap = func(ident string) *codegen.ExtGlobal {
		if v, ok := arrMap[ident]; ok {
			var eleTp llvm.Type
			var arr []int32
			for _, arg := range v.Args {
				ele := constMap[arg]
				switch ele.Type() {
				case types.Int:
					val, err := strconv.ParseInt(string(ele.Raw()), 10, 64)
					if err != nil {
						panic(err)
					}
					arr = append(arr, int32(val))
					// TODO 这个逻辑挪到codegen里
					eleTp = llvm.Int32Type()
				default:
					panic("TODO")
				}
			}
			return &codegen.ExtGlobal{
				Tp:    v.Type(),
				EleTp: eleTp,
				Data:  unsafe.Pointer(&arr[0]),
			}
		} else if v, ok := constMap[ident]; ok {
			var data unsafe.Pointer
			var tp types.ValType
			var eleTp llvm.Type
			switch v.Type() {
			case types.Int:
				val, err := strconv.ParseInt(string(v.Raw()), 10, 64)
				if err != nil {
					panic(err)
				}
				vv := int32(val)
				data = unsafe.Pointer(&vv)
				tp = types.Int
				eleTp = llvm.Int32Type()
			default:
				panic("TODO")
			}
			return &codegen.ExtGlobal{
				Tp:    tp,
				EleTp: eleTp,
				Data:  data,
			}
		}
		panic("unreachable")
	}

	var pairs []*codegen.ExtGlobal
	declVars := em.GetDeclVars()
	for name, ident := range declVars {
		pair := fromMap(ident)
		pair.Name = name
		pairs = append(pairs, pair)
	}
	return restRaw, pairs
}

func parseAssertSections(raw string) []*AssertFrame {
	var frames []*AssertFrame
	for {
		frame := parseAssertSection(raw)
		if frame == nil {
			break
		}
		frames = append(frames, frame)
		raw = frame.body
	}
	last := frames[len(frames)-1]
	for _, f := range frames {
		f.body = last.body
	}
	return frames
}

func parseAssertSection(raw string) *AssertFrame {
	var parseInputOutput = func(raw string) RunResult {
		errMsg := "illegal input output assertion line. should has format of `output, [input_param1, input_param2 ...]`, `int64(1), [int64(2)...]`"
		sep := strings.Index(raw, ",")
		var outputRaw string
		var inputRaws []string
		if sep == -1 {
			outputRaw = raw
		} else {
			outputRaw = strings.TrimSpace(raw[:sep])
			inputRaw := strings.TrimSpace(raw[sep+1:])
			if inputRaw[0] != '[' || inputRaw[len(inputRaw)-1] != ']' {
				panic(errMsg + ". but have: " + inputRaw)
			}
			inputRaw = inputRaw[1 : len(inputRaw)-1]
			inputRaws = strings.Split(inputRaw, ",")
		}

		var parseConst = func(c string) *ir.Const {
			c = strings.TrimSpace(c)
			sep1 := strings.Index(c, "(")
			sep2 := strings.Index(c, ")")
			tp := strings.TrimSpace(c[:sep1])
			v := strings.TrimSpace(c[sep1+1 : sep2])
			switch tp {
			case "int":
				return ir.NewConst(types.Int, []byte(v))
			case "float":
				return ir.NewConst(types.Float, []byte(v))
			}
			panic("unsupported const type: " + tp)
		}

		var inputConsts []*ir.Const
		for _, input := range inputRaws {
			inputConsts = append(inputConsts, parseConst(input))
		}

		var outputConst *ir.Const
		var errCode errors.ErrorCode
		if strings.Index(outputRaw, "error") == 0 {
			sep1 := strings.Index(outputRaw, "(")
			sep2 := strings.Index(outputRaw, ")")
			codeName := outputRaw[sep1+1 : sep2]
			var ok bool
			errCode, ok = errors.ErrorCodeMap[strings.ToUpper(codeName)]
			if !ok {
				panic("no defined errorcode found for " + codeName)
			}
		} else {
			outputConst = parseConst(outputRaw)
		}

		return RunResult{
			input:  inputConsts,
			output: outputConst,
			outerr: errCode,
		}
	}

	raw = strings.TrimSpace(raw)
	var assertToken, assertValue, body string
	if raw[:2] == "/*" {
		raw = strings.TrimSpace(raw[2:])
		end := strings.Index(raw, "*/")
		if end == -1 {
			panic("assert section start with `/*` not closed with `*/`")
		}
		firstSep := strings.IndexFunc(raw, func(r rune) bool {
			return r == ' ' || r == '\n'
		})
		assertToken = raw[:firstSep]
		assertValue = strings.TrimSpace(raw[firstSep:end])
		body = strings.TrimSpace(raw[end+2:])
	} else if raw[:2] == "//" {
		raw = strings.TrimSpace(raw[2:])
		end := strings.Index(raw, "\n")
		firstSep := strings.Index(raw, " ")
		assertToken = raw[:firstSep]
		assertValue = strings.TrimSpace(raw[firstSep:end])
		body = strings.TrimSpace(raw[end+1:])
	} else {
		return nil
	}

	var token AssertToken
	var runResult RunResult
	switch assertToken {
	case "@bb":
		token = AssertTokenBB
	case "@val":
		token = AssertTokenVal
		runResult = parseInputOutput(assertValue)
	case "@anon":
		token = AssertTokenAnon
		runResult = parseInputOutput(assertValue)
	default:
		panic("unsupported assert token: " + assertToken)
	}
	return &AssertFrame{
		token:  token,
		value:  assertValue,
		result: runResult,
		body:   body,
	}
}
