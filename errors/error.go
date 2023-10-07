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
	TYPE_INCOMPATIBLE_TPVAR
	TYPE_INCOMPATIBLE_RECORD
	TYPE_INCOMPATIBLE_ENUM
	TYPE_INCOMPATIBLE_TRAIT
	TYPE_SUBSTITUTE_NUM_MISMATCH
	TYPE_RECORD_KEY_NOTFOUND
	TYPE_RECORD_NOT_FULFILLED
	TYPE_RECORD_ACS_ILLEGAL
	TYPE_ENUM_ELE_ILLEGAL
	TYPE_ENUM_DESTRUCT_ILLEGAL
	TYPE_ENUM_UNDEFINED
	TYPE_ENUM_ELE_UNDEFINED
	TYPE_ENUM_OTHER_ILLEGAL
	TYPE_TRAIT_TYPE_VAR_UNDEFINED
	TYPE_TRAIT_ACS_ILLEGAL
	TYPE_PARAM_COUNT_WRONG
	TYPE_METHOD_ILLEGAL
)

var ErrorCodeMap = map[string]ErrorCode{
	"INTERNAL_ERROR":                INTERNAL_ERROR,
	"TYPE_INCOMPATIBLE_PRIMITIVE":   TYPE_INCOMPATIBLE_PRIMITIVE,
	"TYPE_INCOMPATIBLE_TPVAR":       TYPE_INCOMPATIBLE_TPVAR,
	"TYPE_INCOMPATIBLE_RECORD":      TYPE_INCOMPATIBLE_RECORD,
	"TYPE_INCOMPATIBLE_ENUM":        TYPE_INCOMPATIBLE_ENUM,
	"TYPE_INCOMPATIBLE_TRAIT":       TYPE_INCOMPATIBLE_TRAIT,
	"TYPE_SUBSTITUTE_NUM_MISMATCH":  TYPE_SUBSTITUTE_NUM_MISMATCH,
	"TYPE_RECORD_KEY_NOTFOUND":      TYPE_RECORD_KEY_NOTFOUND,
	"TYPE_RECORD_NOT_FULFILLED":     TYPE_RECORD_NOT_FULFILLED,
	"TYPE_RECORD_ACS_ILLEGAL":       TYPE_RECORD_ACS_ILLEGAL,
	"TYPE_ENUM_ELE_ILLEGAL":         TYPE_ENUM_ELE_ILLEGAL,
	"TYPE_ENUM_DESTRUCT_ILLEGAL":    TYPE_ENUM_DESTRUCT_ILLEGAL,
	"TYPE_ENUM_UNDEFINED":           TYPE_ENUM_UNDEFINED,
	"TYPE_ENUM_ELE_UNDEFINED":       TYPE_ENUM_ELE_UNDEFINED,
	"TYPE_ENUM_OTHER_ILLEGAL":       TYPE_ENUM_OTHER_ILLEGAL,
	"TYPE_TRAIT_TYPE_VAR_UNDEFINED": TYPE_TRAIT_TYPE_VAR_UNDEFINED,
	"TYPE_TRAIT_ACS_ILLEGAL":        TYPE_TRAIT_ACS_ILLEGAL,
	"TYPE_PARAM_COUNT_WRONG":        TYPE_PARAM_COUNT_WRONG,
	"TYPE_METHOD_ILLEGAL":           TYPE_METHOD_ILLEGAL,
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
		msg += ". at " + tk.Start.String()
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
