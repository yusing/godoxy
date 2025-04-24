package trie

import (
	"reflect"
	"testing"
)

func TestNamespace(t *testing.T) {
	k := Namespace("foo")
	if k.String() != "foo" {
		t.Errorf("Namespace.String() = %q, want %q", k.String(), "foo")
	}
	if k.NumSegments() != 1 {
		t.Errorf("Namespace.NumSegments() = %d, want 1", k.NumSegments())
	}
	if k.HasWildcard() {
		t.Error("Namespace.HasWildcard() = true, want false")
	}
}

func TestNewKey(t *testing.T) {
	k := NewKey("a.b.c")
	if !reflect.DeepEqual(k.segments, []string{"a", "b", "c"}) {
		t.Errorf("NewKey.segments = %v, want [a b c]", k.segments)
	}
	if k.String() != "a.b.c" {
		t.Errorf("NewKey.String() = %q, want %q", k.String(), "a.b.c")
	}
	if k.NumSegments() != 3 {
		t.Errorf("NewKey.NumSegments() = %d, want 3", k.NumSegments())
	}
	if k.HasWildcard() {
		t.Error("NewKey.HasWildcard() = true, want false")
	}

	kw := NewKey("foo.*.bar")
	if !kw.HasWildcard() {
		t.Error("NewKey.HasWildcard() = false, want true for wildcard")
	}
}

func TestWithAndWithEscaped(t *testing.T) {
	k := Namespace("foo")
	k2 := k.Clone().With("bar")
	if k2.String() != "foo.bar" {
		t.Errorf("With.String() = %q, want %q", k2.String(), "foo.bar")
	}
	if k2.NumSegments() != 2 {
		t.Errorf("With.NumSegments() = %d, want 2", k2.NumSegments())
	}

	k3 := Namespace("foo").WithEscaped("b.r*")
	esc := EscapeSegment("b.r*")
	if k3.segments[1] != esc {
		t.Errorf("WithEscaped.segment = %q, want %q", k3.segments[1], esc)
	}
}

func TestEscapeSegment(t *testing.T) {
	cases := map[string]string{
		"foo":   "foo",
		"f.o":   "f__o",
		"*":     "__",
		"a*b.c": "a__b__c",
	}
	for in, want := range cases {
		if got := EscapeSegment(in); got != want {
			t.Errorf("EscapeSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClone(t *testing.T) {
	k := NewKey("x.y.z")
	cl := k.Clone()
	if !reflect.DeepEqual(k, cl) {
		t.Errorf("Clone() = %v, want %v", cl, k)
	}
	cl = cl.With("new")
	if cl == k {
		t.Error("Clone() returns same pointer")
	}
	if reflect.DeepEqual(k.segments, cl.segments) {
		t.Error("Clone is not deep copy: segments slice is shared")
	}
}
