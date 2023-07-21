package errors

import (
	"runtime/debug"

	"github.com/kingfolk/capybara/token"
)

type ErrorCode int

const (
	INTERNAL_ERROR ErrorCode = iota

	// TYPE ERROR
	TYPE_INCOMPATIBLE_PRIMITIVE
	TYPE_INCOMPATIBLE_RECORD
	TYPE_INCOMPATIBLE_TPVAR
	TYPE_SUBSTITUTE_NUM_MISMATCH
	TYPE_RECORD_KEY_NOTFOUND
	TYPE_RECORD_NOT_FULFILLED
)

var ErrorCodeMap = map[string]ErrorCode{
	"INTERNAL_ERROR":               INTERNAL_ERROR,
	"TYPE_INCOMPATIBLE_PRIMITIVE":  TYPE_INCOMPATIBLE_PRIMITIVE,
	"TYPE_INCOMPATIBLE_RECORD":     TYPE_INCOMPATIBLE_RECORD,
	"TYPE_INCOMPATIBLE_TPVAR":      TYPE_INCOMPATIBLE_TPVAR,
	"TYPE_SUBSTITUTE_NUM_MISMATCH": TYPE_SUBSTITUTE_NUM_MISMATCH,
	"TYPE_RECORD_KEY_NOTFOUND":     TYPE_RECORD_KEY_NOTFOUND,
	"TYPE_RECORD_NOT_FULFILLED":    TYPE_RECORD_NOT_FULFILLED,
}

type LangError struct {
	Code       ErrorCode
	Msg        string
	DebugTrace []byte
}

func NewError(code ErrorCode, msg string) LangError {
	return NewErrorWithTk(code, msg, nil)
}

func NewErrorWithTk(code ErrorCode, msg string, tk *token.Token) LangError {
	if tk != nil {
		msg += ". at " + tk.String()
	}
	return LangError{
		Code:       code,
		Msg:        msg,
		DebugTrace: debug.Stack(),
	}
}

func (e LangError) Error() string {
	return e.Msg
}
