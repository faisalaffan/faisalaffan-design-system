package kit

import (
	"errors"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	e := NewAppError(500, "something broke", errors.New("db down"))
	if e.Code != 500 {
		t.Errorf("expected code 500, got %d", e.Code)
	}
	if e.Unwrap().Error() != "db down" {
		t.Errorf("expected wrapped error 'db down', got %v", e.Unwrap())
	}
}

func TestAppError_ErrorNoWrap(t *testing.T) {
	e := NewAppError(400, "bad input", nil)
	if e.Unwrap() != nil {
		t.Error("expected nil wrapped error")
	}
}
