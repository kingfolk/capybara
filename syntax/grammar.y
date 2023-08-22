/*
  This parser definition is based on min-caml/parser.mly
  Copyright (c) 2005-2008, Eijiro Sumii, Moe Masuko, and Kenichi Asai
*/

%{
package syntax

import (
	"fmt"
	"strconv"
	"github.com/kingfolk/capybara/ast"
	"github.com/kingfolk/capybara/token"
)
%}

%union{
	node ast.Expr
	nodes []ast.Expr
	token *token.Token
	decls []*ast.Symbol
	decl *ast.Symbol
	params []ast.Param
	param *ast.Param
	program *ast.AST
}

%token<token> ILLEGAL
%token<token> COMMENT
%token<token> LPAREN
%token<token> RPAREN
%token<token> LCURLY
%token<token> RCURLY
%token<token> IDENT
%token<token> BOOL
%token<token> NOT
%token<token> INT
%token<token> FLOAT
%token<token> MINUS
%token<token> PLUS
%token<token> MINUS_DOT
%token<token> PLUS_DOT
%token<token> STAR_DOT
%token<token> SLASH_DOT
%token<token> DOUBLE_EQUAL
%token<token> EQUAL
%token<token> LESS_GREATER
%token<token> LESS_EQUAL
%token<token> LESS
%token<token> GREATER
%token<token> GREATER_EQUAL
%token<token> IF
%token<token> THEN
%token<token> ELSE
%token<token> FOR
%token<token> LET
%token<token> IN
%token<token> REC
%token<token> TUP
%token<token> TRAIT
%token<token> ENUM
%token<token> COMMA
%token<token> ARRAY
%token<token> ARRAY_MAKE
%token<token> DOT
%token<token> LESS_MINUS
%token<token> SEMICOLON
%token<token> STAR
%token<token> SLASH
%token<token> BAR_BAR
%token<token> AND_AND
%token<token> ARRAY_LENGTH
%token<token> STRING_LITERAL
%token<token> PERCENT
%token<token> MATCH
%token<token> CASE
%token<token> BAR
%token<token> MINUS_GREATER
%token<token> FUN
%token<token> COLON
%token<token> TYPE
%token<token> LBRACKET_BAR
%token<token> BAR_RBRACKET
%token<token> LBRACKET
%token<token> RBRACKET
%token<token> EXTERNAL

%nonassoc IN
%right prec_let
%right prec_seq
%right SEMICOLON
%left prec_def
%right prec_if
%right prec_match
%right prec_fun
%right LESS_MINUS
%right EQUAL
%nonassoc BAR
%left prec_tuple
%left COMMA
%left BAR_BAR
%left AND_AND
%left DOUBLE_EQUAL LESS_GREATER LESS GREATER LESS_EQUAL GREATER_EQUAL
%left PLUS MINUS PLUS_DOT MINUS_DOT
%left STAR SLASH STAR_DOT SLASH_DOT PERCENT
%right prec_unary_minus
%left prec_app
%nonassoc IDENT
%nonassoc prec_below_ident
%nonassoc LCURLY
%nonassoc LPAREN
%nonassoc LBRACKET
%left DOT

%type<node> exp
%type<node> int_exp
%type<nodes> seq_exp
%type<nodes> list_exp
%type<node> case
%type<nodes> seq_case
%type<node> vardef
%type<nodes> args
%type<param> opt_fun_receiver
%type<params> params
%type<params> func_params
%type<params> opt_params
%type<decls> id_list
%type<decls> opt_type_params
%type<params> named_args
%type<node> simple_type_annotation
%type<node> type
%type<nodes> seq_type
%type<node> trait_fun
%type<nodes> seq_trait_fun
%type<node> simple_type
%type<node> array_type
%type<node> rec_type
%type<node> tup_type
%type<node> enum_type
%type<node> trait_type
%type<program> toplevels
%type<> program

%start program

%%

program:
	toplevels seq_exp
		{
			tree := $1
			tree.Root = $2
			yylex.(*pseudoLexer).result = tree
		}

toplevels:
	/* empty */
		{ $$ = &ast.AST{} }
	| toplevels TYPE IDENT EQUAL type SEMICOLON
		{
			decl := &ast.TypeDecl{$2, ast.NewSymbol($3.Value()), $5}
			tree := $1
			tree.TypeDecls = append(tree.TypeDecls, decl)
			$$ = tree
		}
	| toplevels EXTERNAL IDENT COLON type EQUAL STRING_LITERAL SEMICOLON
		{
			from := $7.Value()
			lit, err := strconv.Unquote(from)
			if err != nil {
				yylex.Error(fmt.Sprintf("Parse error at string literal in 'external' decl: %s: %s", from, err.Error()))
			} else {
				tree := $1
				ext := &ast.External{$2, $7, sym($3), $5, lit}
				tree.Externals = append(tree.Externals, ext)
				$$ = tree
			}
		}

seq_exp:
	exp %prec prec_seq
		{ $$ = []ast.Expr{$1} }
	| seq_exp SEMICOLON exp
		{ $$ = append($1, $3) }

list_exp:
	exp %prec prec_seq
		{ $$ = []ast.Expr{$1} }
	| seq_exp COMMA exp
		{ $$ = append($1, $3) }

exp:
	BOOL
		{ $$ = &ast.Bool{$1, $1.Value() == "true"} }
	| INT
		{
			i, err := strconv.ParseInt($1.Value(), 10, 64)
			if err != nil {
				yylex.Error("Parse error at int literal: " + err.Error())
			} else {
				$$ = &ast.Int{$1, i}
			}
		}
	| FLOAT
		{
			f, err := strconv.ParseFloat($1.Value(), 64)
			if err != nil {
				yylex.Error("Parse error at float literal: " + err.Error())
			} else {
				$$ = &ast.Float{$1, f}
			}
		}
	| STRING_LITERAL
		{
			from := $1.Value()
			s, err := strconv.Unquote(from)
			if err != nil {
				yylex.Error(fmt.Sprintf("Parse error at string literal %s: %s", from, err.Error()))
			} else {
				$$ = &ast.String{$1, s}
			}
		}
	| IDENT
		{ $$ = &ast.VarRef{$1, ast.NewSymbol($1.Value())} }
	| NOT exp
		%prec prec_app
		{ $$ = &ast.Not{$1, $2} }
	| MINUS exp
		%prec prec_unary_minus
		{ $$ = &ast.Neg{$1, $2} }
	| exp PLUS exp
		{ $$ = &ast.Add{$1, $3} }
	| exp MINUS exp
		{ $$ = &ast.Sub{$1, $3} }
	| exp STAR exp
		{ $$ = &ast.Mul{$1, $3} }
	| exp SLASH exp
		{ $$ = &ast.Div{$1, $3} }
	| exp PERCENT exp
		{ $$ = &ast.Mod{$1, $3} }
	| exp DOUBLE_EQUAL exp
		{ $$ = &ast.Eq{$1, $3} }
	| exp LESS_GREATER exp
		{ $$ = &ast.NotEq{$1, $3} }
	| exp LESS exp
		{ $$ = &ast.Less{$1, $3} }
	| exp GREATER exp
		{ $$ = &ast.Greater{$1, $3} }
	| exp LESS_EQUAL exp
		{ $$ = &ast.LessEq{$1, $3} }
	| exp GREATER_EQUAL exp
		{ $$ = &ast.GreaterEq{$1, $3} }
	| exp AND_AND exp
		{ $$ = &ast.And{$1, $3} }
	| exp BAR_BAR exp
		{ $$ = &ast.Or{$1, $3} }
	| IF exp THEN seq_exp ELSE seq_exp
		%prec prec_if
		{ $$ = &ast.If{$1, $2, $4, $6} }
	| FOR LPAREN exp RPAREN LCURLY seq_exp RCURLY
		%prec prec_if
		{
			$$ = &ast.Loop{$1, $3, $6}
		}
	| ARRAY LBRACKET simple_type RBRACKET LPAREN args RPAREN
		{ $$ = &ast.ArrayLit{$1, $7, $6} }
	| vardef
		{ $$ = $1 }
	| exp LBRACKET list_exp RBRACKET
		{ $$ = &ast.ApplyBracket{$1, $3} }
	| exp LPAREN args RPAREN
		%prec prec_app
		{
			switch b := $1.(type) {
			case *ast.ApplyBracket:
				if d, ok := b.Expr.(*ast.DotAcs); ok {
					app := &ast.Apply{d.Dot, b.Args, $3}
					$$ = &ast.DotAcs{d.Expr, app, $4}
				} else {
					$$ = &ast.Apply{b.Expr, b.Args, $3}
				}
			case *ast.DotAcs:
				app := &ast.Apply{b.Dot, nil, $3}
				$$ = &ast.DotAcs{b.Expr, app, $4}
			default:
				$$ = &ast.Apply{$1, nil, $3}
			}
		}
	| exp EQUAL exp
		%prec prec_def
		{
			b, ok := $1.(*ast.ApplyBracket)
			if ok {
				if len(b.Args) != 1 {
					yylex.Error("array subscript more than one element")
				}
				$$ = &ast.ArrayPut{b.Expr, b.Args[0], $3}
			} else {
				$$ = &ast.Mutate{$1.(*ast.VarRef), $3}
			}
		}
	| exp LCURLY named_args RCURLY
	 	%prec prec_app
	 	{
			rec := $1
			var tpArgs []ast.Expr
			if b, ok := $1.(*ast.ApplyBracket); ok {
				rec = b.Expr
				tpArgs = b.Args
			}
			
			if ref, ok := rec.(*ast.VarRef); ok {
				$$ = &ast.RecLit{ref, tpArgs, $3}
			} else {
				yylex.Error("record literal expr fail. illegal record type id before {")
			}
		}
	| exp DOT IDENT
		{
			ref := &ast.VarRef{$3, sym($3)}
			$$ = &ast.DotAcs{$1, ref, $3} 
		}
	| exp DOT INT
		{ 
			ref := &ast.VarRef{$3, sym($3)}
			$$ = &ast.DotAcs{$1, ref, $3}
		}
	| FUN opt_fun_receiver IDENT opt_type_params func_params simple_type_annotation EQUAL LCURLY seq_exp RCURLY
		%prec prec_fun
		{
			ident := sym($3)
			def := &ast.FuncDef{
				FuncType: ast.FuncType{
					Token: $1,
					Params: $5,
					TpParams: $4,
					RetType: $6,
				},
				Symbol: ident,
				Rcv: $2,
				Body: $9,
			}
			ref := &ast.VarRef{$1, ident}
			$$ = &ast.LetRec{
				LetToken: $1,
				Func: def,
				Body: ref,
			}
		}
	| MATCH exp LCURLY seq_case RCURLY
		%prec prec_if
		{
			var cases []*ast.Case
			for _, c := range $4 {
				cases = append(cases, c.(*ast.Case))
			}
			$$ = &ast.Match{
				StartToken: $1,
				EndToken: $5,
				Target: $2,
				Cases: cases,
			}
		}
	| ILLEGAL error
		{
			yylex.Error("Parsing illegal token: " + $1.String())
			$$ = nil
		}

seq_case:
	case
		{ $$ = []ast.Expr{$1} }
	| seq_case case
		{ $$ = append($1, $2) }

case:
	CASE exp COLON seq_exp
		{ $$ = &ast.Case{$1, $2, $4} }

vardef:
	LET IDENT COLON type
		%prec prec_let
		{ $$ = &ast.Let{$1, sym($2), nil, $4} }
	| LET IDENT EQUAL exp
		%prec prec_let
		{ $$ = &ast.Let{$1, sym($2), $4, nil} }
	| LET IDENT COLON type EQUAL exp
		%prec prec_let
		{ $$ = &ast.Let{$1, sym($2), $6, $4} }

func_params:
	LPAREN opt_params RPAREN
		{ $$ = $2 }

opt_params:
		{ $$ = nil }
	| params
		{ $$ = $1 }

params:
	IDENT COLON type
		{ $$ = []ast.Param{ast.Param{$1, sym($1), $3}} }
	| opt_params COMMA IDENT COLON type
		{ $$ = append($1, ast.Param{$3, sym($3), $5}) }

named_args:
	IDENT COLON exp
		{ $$ = []ast.Param{ast.Param{$1, sym($1), $3}} }
	| named_args COMMA IDENT COLON exp
		{ $$ = append($1, ast.Param{$3, sym($3), $5}) }

args:
		{ $$ = []ast.Expr{} }
	| args COMMA exp
		{ $$ = append($1, $3) }
	| exp
		{ $$ = []ast.Expr{$1} }

int_exp:
	INT
		{
			i, err := strconv.ParseInt($1.Value(), 10, 64)
			if err != nil {
				yylex.Error("Parse error at int literal: " + err.Error())
			} else {
				$$ = &ast.Int{$1, i}
			}
		}

simple_type_annotation:
		{ $$ = nil }
	| COLON type
		{ $$ = $2 }

type:
	simple_type
		{ $$ = $1 }
	| array_type
		{ $$ = $1 }
	| rec_type
		{ $$ = $1 }
	| tup_type
		{ $$ = $1 }
	| enum_type
		{ $$ = $1 }
	| trait_type
		{ $$ = $1 }

seq_type:
	type
		{ $$ = []ast.Expr{$1} }
	| seq_type COMMA type
		{ $$ = append($1, $3) }

simple_type:
	IDENT
		{ $$ = &ast.CtorType{nil, $1, nil, nil, ast.NewSymbol($1.Value())} }
	| IDENT LBRACKET list_exp RBRACKET
		{ $$ = &ast.CtorType{nil, $1, $3, nil, ast.NewSymbol($1.Value())} }

array_type:
	ARRAY LBRACKET simple_type COMMA int_exp RBRACKET
		{
			ele := $3
			size := $5
			$$ = &ast.CtorType{$1, $6, []ast.Expr{ele, size}, nil, sym($1)}
		}

rec_type:
	REC opt_type_params LCURLY params RCURLY
		{
			var e []ast.Expr
			for _, p := range $4 {
				e = append(e, p)
			}
			$$ = &ast.CtorType{$1, $5, e, $2, sym($1)}
		}

tup_type:
	TUP opt_type_params LPAREN seq_type RPAREN
		{
			$$ = &ast.CtorType{$1, $5, $4, $2, sym($1)}
		}

enum_type:
	ENUM opt_type_params LCURLY seq_type RCURLY
		{
			$$ = &ast.CtorType{$1, $5, $4, $2, sym($1)}
		}

trait_type:
	TRAIT opt_type_params LCURLY seq_trait_fun RCURLY
		{
			$$ = &ast.CtorType{$1, $5, $4, $2, sym($1)}
		}

trait_fun:
	IDENT func_params simple_type_annotation
		{
			tp := &ast.FuncType{
				Token: $1,
				Params: $2,
				RetType: $3,
			}
			$$ = ast.Param{$1, sym($1), tp}
		}

seq_trait_fun:
	trait_fun
		{ $$ = []ast.Expr{$1} }
	| seq_trait_fun trait_fun
		{ $$ = append($1, $2) }

opt_type_params:
		{ $$ = nil }
	| LBRACKET id_list RBRACKET
		{ $$ = $2 }

opt_fun_receiver:
		{ $$ = nil }
	| LPAREN IDENT exp RPAREN
		{
			var tp *ast.CtorType
			if e, ok := $3.(*ast.VarRef); ok {
				tp = &ast.CtorType{$1, $4, nil, nil, e.Symbol}
			} else if e, ok := $3.(*ast.ApplyBracket); ok {
				rcv, ok := e.Expr.(*ast.VarRef)
				if !ok {
					yylex.Error("illegal trait access. trait func call syntax error")
				}
				tp = &ast.CtorType{$1, $4, e.Args, nil, rcv.Symbol}
			} else {
				yylex.Error("illegal trait access. trait func call syntax error")
			}
			$$ = &ast.Param{$1, sym($2), tp}
		}

id_list:
	IDENT
		{
			s := sym($1)
			$$ = []*ast.Symbol{s}
		}
	| id_list COMMA IDENT
		{
			s := sym($3)
			$$ = append($1, s)
		}

%%

func sym(tok *token.Token) *ast.Symbol {
	s := tok.Value()
	if s == "_" {
		return ast.IgnoredSymbol()
	} else {
		return ast.NewSymbol(s)
	}
}

// vim: noet
