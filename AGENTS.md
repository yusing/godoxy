# AGENTS.md

## Principles

DO NOT run build command.

## Documentation

Update package level `README.md` after making significant changes.

## Go Guidelines

1. Use `golang-best-practices` skill.
2. Use `internal/task/task.go` for lifetime management:
   - `task.RootTask()` for background operations
   - `parent.Subtask()` for nested tasks
   - `OnFinished()` and `OnCancel()` callbacks for cleanup
3. Use `gperr "goutils/errs"` to build pretty nested errors:
   - `gperr.Multiline()` for multiple operation attempts
   - `gperr.NewBuilder()` to collect errors
   - `gperr.NewGroup() + group.Go()` to collect errors of multiple concurrent operations
   - `gperr.PrependSubject()` to prepend subject to errors
4. Use `github.com/puzpuzpuz/xsync/v4` for lock-free thread safe maps
5. Use `goutils/synk` to retrieve and put byte buffer

## Testing

- Prefer scoped tests
- Prefer `testify`
- Use `-ldflags="-checklinkname=0"`

## HTML/JS

- js/html files are minified with bun, so embed the minified output such as `{filename}.min.html` or `{filename}.min.js` instead of the original file; see Makefile.
