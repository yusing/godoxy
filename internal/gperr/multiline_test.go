package gperr

import (
	"net"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestWrapMultiline(t *testing.T) {
	multiline := Multiline()
	var wrapper error = wrap(multiline)
	_, ok := wrapper.(*MultilineError)
	if !ok {
		t.Errorf("wrapper is not a MultilineError")
	}
}

func TestPrependSubjectMultiline(t *testing.T) {
	multiline := Multiline()
	multiline.Addf("line 1 %s", "test")
	multiline.Adds("line 2")
	multiline.AddLines([]any{1, "2", 3.0, net.IPv4(127, 0, 0, 1)})
	multiline.Subject("subject")

	builder := NewBuilder()
	builder.Add(multiline)
	expect.Equal(t, len(multiline.Extras), len(builder.errs))
}
