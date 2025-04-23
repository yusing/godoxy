//go:build debug

package trie

import "fmt"

func panicInvalidAssignment() {
	// assigned anything after manually assigning nil
	// will panic because of type mismatch (zeroValue and v.(type))
	if r := recover(); r != nil {
		panic(fmt.Errorf("attempt to assign non-nil value on edge node or assigning mismatched type: %v", r))
	}
}
