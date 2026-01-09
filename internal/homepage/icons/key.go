package icons

import (
	"fmt"
	"strings"
)

type Key string

func NewKey(source Source, reference string) Key {
	return Key(fmt.Sprintf("%s/%s", source, reference))
}

func (k Key) SourceRef() (Source, string) {
	source, ref, _ := strings.Cut(string(k), "/")
	return Source(source), ref
}
