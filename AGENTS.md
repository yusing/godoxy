# AGENTS.md

## Development Commands

- Build: You should not run build command.
- Test: `go test -ldflags="-checklinkname=0" ...`

## Documentation

Update package level `README.md` if exists after making significant changes.

## Go Guidelines

1. Use builtin `min` and `max` functions instead of creating custom ones
2. Prefer `for i := range 10` over `for i := 0; i < 10; i++`
3. Beware of variable shadowing when making edits
4. Use `internal/task/task.go` for lifetime management:
   - `task.RootTask()` for background operations
   - `parent.Subtask()` for nested tasks
   - `OnFinished()` and `OnCancel()` callbacks for cleanup
5. Use `gperr "goutils/errs"` to build pretty nested errors:
   - `gperr.Multiline()` for multiple operation attempts
   - `gperr.NewBuilder()` to collect errors
   - `gperr.NewGroup() + group.Go()` to collect errors of multiple concurrent operations
   - `gperr.PrependSubject()` to prepend subject to errors
6. Use `github.com/puzpuzpuz/xsync/v4` for lock-free thread safe maps
7. Use `goutils/synk` to retrieve and put byte buffer

## Testing

- Run scoped tests instead of `./...`
- Use `testify`, no manual assertions.
