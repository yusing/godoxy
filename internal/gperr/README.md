# gperr

gperr is an error interface that supports nested structure and subject highlighting.

## Usage

### gperr.Error

The error interface.

### gperr.New

Like `errors.New`, but returns a `gperr.Error`.

### gperr.Wrap

Like `fmt.Errorf("%s: %w", message, err)`, but returns a `gperr.Error`.

### gperr.Error.Subject

Returns a new error with the subject prepended to the error message. The main subject is highlighted.

```go
err := gperr.New("error message")
err = err.Subject("bar")
err = err.Subject("foo")
```

Output:

<code>foo > <span style="color: red;">bar</span>: error message</code>

### gperr.Error.Subjectf

Like `gperr.Error.Subject`, but formats the subject with `fmt.Sprintf`.

### gperr.PrependSubject

Prepends the subject to the error message like `gperr.Error.Subject`.

```go
err := gperr.New("error message")
err = gperr.PrependSubject(err, "foo")
err = gperr.PrependSubject(err, "bar")
```

Output:

<code>bar > <span style="color: red;">foo</span>: error message</code>

### gperr.Error.With

Adds a new error to the error chain.

```go
err := gperr.New("error message")
err = err.With(gperr.New("inner error"))
err = err.With(gperr.New("inner error 2").With(gperr.New("inner inner error")))
```

Output:

```
error message:
  • inner error
  • inner error 2
    • inner inner error
```

### gperr.Error.Withf

Like `gperr.Error.With`, but formats the error with `fmt.Errorf`.

### gperr.Error.Is

Returns true if the error is equal to the given error.

### gperr.Builder

A builder for `gperr.Error`.

```go
builder := gperr.NewBuilder("foo")
builder.Add(gperr.New("error message"))
builder.Addf("error message: %s", "foo")
builder.AddRange(gperr.New("error message 1"), gperr.New("error message 2"))
```

Output:

```
foo:
  • error message
  • error message: foo
  • error message 1
  • error message 2
```

### gperr.Builder.Build

Builds a `gperr.Error` from the builder.

## When to return gperr.Error

- When you want to return multiple errors
- When the error has a subject
