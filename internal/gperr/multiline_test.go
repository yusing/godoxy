package gperr

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultiline(t *testing.T) {
	multiline := Multiline()
	multiline.Addf("line 1 %s", "test")
	multiline.Adds("line 2")
	multiline.AddLines([]any{1, "2", 3.0, net.IPv4(127, 0, 0, 1)})
	t.Error(New("result").With(multiline))
	t.Error(multiline.Subject("subject").Withf("inner"))
}

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
	require.Equal(t, len(builder.errs), len(multiline.Extras), builder.errs)
}
