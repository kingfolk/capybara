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
	funcdef *ast.FuncDef
	decls []*ast.Symbol
	decl *ast.Symbol
	params []ast.Param
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
%token<token> WITH
%token<token> BAR
%token<token> SOME
%token<token> NONE
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
%nonassoc WITH
%right prec_if
%right prec_match
%right prec_fun
%right LESS_MINUS
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
%left DOT
%nonassoc prec_below_ident
%nonassoc IDENT

%type<node> exp
%type<node> simple_exp
%type<node> int_exp
%type<nodes> seq_exp
%type<node> vardef
%type<nodes> args
%type<params> params
// %type<nodes> semi_elems
%type<node> simple_type_annotation
%type<node> type
%type<node> simple_type
%type<node> array_type
%type<nodes> type_comma_list
%type<program> toplevels
// %type<> opt_semi
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

exp:
	simple_exp
		{ $$ = $1 }
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
	| FOR IDENT EQUAL exp DOT DOT exp LCURLY seq_exp RCURLY
		%prec prec_if
		{ 
			ident := sym($2)
			$$ = &ast.Loop{$1, ident, $4, $7, $9}
		}
	| ARRAY LESS simple_type GREATER LPAREN args RPAREN
		{ $$ = &ast.ArrayLit{$1, $7, $6} }
	| simple_exp LBRACKET exp RBRACKET LESS_MINUS exp
		{ $$ = &ast.ArrayPut{$1, $3, $6} }
	| vardef
		{ $$ = $1 }
	| simple_exp LPAREN args RPAREN
		%prec prec_app
		{ $$ = &ast.Apply{$1, $3} }
	| FUN IDENT params simple_type_annotation EQUAL LCURLY seq_exp RCURLY
		%prec prec_fun
		{
			// t := $1
			// ident := ast.NewSymbol(fmt.Sprintf("lambda.line%d.col%d", t.Start.Line, t.Start.Column))
			ident := sym($2)
			def := &ast.FuncDef{
				Symbol: ident,
				Params: $3,
				Body: $7,
				RetType: $4,
			}
			ref := &ast.VarRef{$1, ident}
			$$ = &ast.LetRec{
				LetToken: $1,
				Func: def,
				Body: ref,
			}
		}
	| ILLEGAL error
		{
			yylex.Error("Parsing illegal token: " + $1.String())
			$$ = nil
		}

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

params:
	LPAREN RPAREN
		{ $$ = []ast.Param{} }
	| LPAREN IDENT COLON type RPAREN
		{ $$ = []ast.Param{{sym($2), $4}} }
	| params LPAREN IDENT COLON type RPAREN
		{ $$ = append($1, ast.Param{sym($3), $5}) }

args:
		{ $$ = []ast.Expr{} }
	| args COMMA simple_exp
		{ $$ = append($1, $3) }
	| simple_exp
		{ $$ = []ast.Expr{$1} }

simple_exp:
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
	// | LBRACKET_BAR BAR_RBRACKET
	// 	{ $$ = &ast.ArrayLit{$1, $2, nil} }
	// | LBRACKET_BAR semi_elems opt_semi BAR_RBRACKET
	// 	{ $$ = &ast.ArrayLit{$1, $4, $2} }
	// | LBRACKET RBRACKET error
	// 	{ yylex.Error("List literal is not implemented yet. Please use array literal [| e1; e2; ... |] instead") }
	// | LBRACKET semi_elems opt_semi RBRACKET error
	// 	{ yylex.Error("List literal is not implemented yet. Please use array literal [| e1; e2; ... |] instead") }
	| NONE
		{ $$ = &ast.None{$1} }
	| IDENT
		{ $$ = &ast.VarRef{$1, ast.NewSymbol($1.Value())} }
	| simple_exp LBRACKET exp RBRACKET
		{
			arr := $1
			index := $3
			$$ = &ast.ArrayGet{arr, index}
		}

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

// semi_elems:
// 	exp %prec prec_seq
// 		{ $$ = []ast.Expr{$1} }
// 	| semi_elems SEMICOLON exp
// 		{ $$ = append($1, $3) }

// opt_semi:
// 	/* empty */ {} | SEMICOLON {}

simple_type_annotation:
		{ $$ = nil }
	| COLON type
		{ $$ = $2 }

type:
	simple_type
		{ $$ = $1 }
	| array_type
		{ $$ = $1 }

simple_type:
	IDENT
		{
			t := $1
			$$ = &ast.CtorType{nil, t, nil, ast.NewSymbol(t.Value())}
		}
	| simple_type IDENT
		{
			t := $2
			$$ = &ast.CtorType{nil, t, []ast.Expr{$1}, ast.NewSymbol(t.Value())}
		}
	| LPAREN type_comma_list RPAREN IDENT
		{
			t := $4
			$$ = &ast.CtorType{$1, t, $2, ast.NewSymbol(t.Value())}
		}
	| LPAREN type_comma_list RPAREN
		%prec prec_below_ident
		{
			ts := $2
			if len(ts) > 1 {
				yylex.Error("(t1, t2, ...) is not a type. For tuple, use t1 * t2 * ... * tn")
			} else {
				$$ = $2[0]
			}
		}

array_type:
	ARRAY LESS simple_type COMMA int_exp GREATER
		{
			ele := $3
			size := $5
			$$ = &ast.CtorType{$1, $6, []ast.Expr{ele, size}, sym($1)}
		}

type_comma_list:
	type
		{ $$ = []ast.Expr{$1} }
	| type_comma_list COMMA type
		{ $$ = append($1, $3) }

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
