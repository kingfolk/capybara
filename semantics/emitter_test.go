package semantics

import (
	"fmt"
	"testing"

	"github.com/kingfolk/capybara/ast"
	"github.com/kingfolk/capybara/syntax"
	"github.com/rhysd/locerr"
	"github.com/stretchr/testify/require"
)

func TestEmitLet(t *testing.T) {
	raw := "let x = 42 + 2"
	// raw := "let x = 42 in x"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitIf(t *testing.T) {
	// TODO: better test both case
	// raw := "let x: int = IF 2 > 1 THEN 10 ELSE 20"
	raw := "let x = if 2 then 10 else 20"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitLoop(t *testing.T) {
	raw := "fun f(a:int): int = { for a = 1 .. 10 { a } }"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitFunc(t *testing.T) {
	raw := "fun f(a:int): int = {a + 2}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitFunc2(t *testing.T) {
	raw := "fun f(a:array<int,5>): int = {1 + 2}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitFunc3(t *testing.T) {
	raw := "fun f(a:int): array<int,3> = {array<int>(1,2,3)}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitFunc4(t *testing.T) {
	raw := "fun f(a:array<int,3>): int = {a[1]}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitFunc5(t *testing.T) {
	raw := "fun f(): int = {let a:array<int,3> = array<int>(1,2,3);a[1]}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}

func TestEmitFunc6(t *testing.T) {
	raw := "fun f(): int = {let a:array<int,3> = array<int>(1,2,3);a[1] <- 1}"
	s := locerr.NewDummySource(raw)
	node, err := syntax.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	ast.Println(node)
	ir, _, err := EmitIR(node)
	require.NoError(t, err)
	fmt.Println("ir", ir)
}
