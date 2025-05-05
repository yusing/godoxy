package gperr

import (
	"testing"
)

type testErr struct{}

func (e testErr) Error() string {
	return "test error"
}

func (e testErr) Plain() []byte {
	return []byte("test error")
}

func (e testErr) Markdown() []byte {
	return []byte("**test error**")
}

type testMultiErr struct {
	errors []error
}

func (e testMultiErr) Error() string {
	return Join(e.errors...).Error()
}

func (e testMultiErr) Unwrap() []error {
	return e.errors
}

func TestFormatting(t *testing.T) {
	err := testErr{}
	plain := Plain(err)
	if string(plain) != "test error" {
		t.Errorf("expected test error, got %s", string(plain))
	}
	md := Markdown(err)
	if string(md) != "**test error**" {
		t.Errorf("expected test error, got %s", string(md))
	}
}

func TestMultiError(t *testing.T) {
	err := testMultiErr{[]error{testErr{}, testErr{}}}
	plain := Plain(err)
	if string(plain) != "test error\ntest error\n" {
		t.Errorf("expected test error, got %s", string(plain))
	}
	md := Markdown(err)
	if string(md) != "**test error**\n**test error**\n" {
		t.Errorf("expected test error, got %s", string(md))
	}
}
