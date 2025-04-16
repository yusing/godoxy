package gperr

import (
	"fmt"
	"sync"
)

type noLock struct{}

func (noLock) Lock()    {}
func (noLock) Unlock()  {}
func (noLock) RLock()   {}
func (noLock) RUnlock() {}

type rwLock interface {
	sync.Locker
	RLock()
	RUnlock()
}

type Builder struct {
	about string
	errs  []error
	rwLock
}

type multiline struct {
	*Builder
}

// NewBuilder creates a new Builder.
//
// If about is not provided, the Builder will not have a subject
// and will expand when adding to another builder.
func NewBuilder(about ...string) *Builder {
	if len(about) == 0 {
		return &Builder{rwLock: noLock{}}
	}
	return &Builder{about: about[0], rwLock: noLock{}}
}

func NewBuilderWithConcurrency(about ...string) *Builder {
	if len(about) == 0 {
		return &Builder{rwLock: new(sync.RWMutex)}
	}
	return &Builder{about: about[0], rwLock: new(sync.RWMutex)}
}

func (b *Builder) EnableConcurrency() {
	b.rwLock = new(sync.RWMutex)
}

func (b *Builder) About() string {
	return b.about
}

func (b *Builder) HasError() bool {
	// no need to lock, when this is called, the Builder is not used anymore
	return len(b.errs) > 0
}

func (b *Builder) Error() Error {
	if len(b.errs) == 0 {
		return nil
	}
	return &nestedError{Err: New(b.about), Extras: b.errs}
}

func (b *Builder) String() string {
	err := b.Error()
	if err == nil {
		return ""
	}
	return err.Error()
}

// Add adds an error to the Builder.
//
// adding nil is no-op.
func (b *Builder) Add(err error) *Builder {
	if err == nil {
		return b
	}

	b.Lock()
	defer b.Unlock()

	b.add(err)
	return b
}

func (b *Builder) add(err error) {
	switch err := err.(type) {
	case *baseError:
		b.errs = append(b.errs, err.Err)
	case *nestedError:
		if err.Err == nil {
			b.errs = append(b.errs, err.Extras...)
		} else {
			b.errs = append(b.errs, err)
		}
	case *MultilineError:
		b.add(&err.nestedError)
	default:
		b.errs = append(b.errs, err)
	}
}

func (b *Builder) Adds(err string) *Builder {
	b.Lock()
	defer b.Unlock()
	b.errs = append(b.errs, newError(err))
	return b
}

func (b *Builder) Addf(format string, args ...any) *Builder {
	if len(args) > 0 {
		b.Lock()
		defer b.Unlock()
		b.errs = append(b.errs, fmt.Errorf(format, args...))
	} else {
		b.Adds(format)
	}

	return b
}

func (b *Builder) AddFrom(other *Builder, flatten bool) *Builder {
	if other == nil || !other.HasError() {
		return b
	}

	b.Lock()
	defer b.Unlock()
	if flatten {
		b.errs = append(b.errs, other.errs...)
	} else {
		b.errs = append(b.errs, other.Error())
	}
	return b
}

func (b *Builder) AddRange(errs ...error) *Builder {
	nonNilErrs := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			nonNilErrs = append(nonNilErrs, err)
		}
	}

	b.Lock()
	defer b.Unlock()

	for _, err := range nonNilErrs {
		b.add(err)
	}
	return b
}

func (b *Builder) ForEach(fn func(error)) {
	b.RLock()
	errs := b.errs
	b.RUnlock()

	for _, err := range errs {
		fn(err)
	}
}
