# AGENTS.md

## Repo Map: Proxy / Route Debugging

Start from these anchors before broad repo search:

- Route lifecycle/finalization: `internal/route/route.go`
- Reverse proxy route construction/start: `internal/route/reverse_proxy.go`
- Entrypoint route resolution/context: `internal/entrypoint/http_server.go`, `internal/route/routes/context.go`
- Rule commands/matchers: `internal/route/rules/do.go`, `internal/route/rules/on.go`, `internal/route/rules/validate.go`
- Docker labels -> routes: `internal/docker/label.go`, `internal/docker/container.go`, `internal/route/provider/docker.go`
- Shared reverse proxy implementation: `goutils/http/reverseproxy/reverse_proxy.go`
- Response buffering/trailers used by rules: `goutils/http/response_modifier.go`
- Agent proxy headers/handler: `agent/pkg/agentproxy/config.go`, `agent/pkg/handler/proxy_http.go`

Module boundaries matter: root, `agent/`, and `goutils/` are separate Go modules; run scoped tests from the owning module directory.

## Documentation

Update package level `README.md`, wiki and webui types after making significant changes.

## Go Patterns

1. `internal/task/task.go` for lifetime management:
   - `task.RootTask()` for background operations
   - `parent.Subtask()` for nested tasks
   - `OnFinished()` and `OnCancel()` callbacks for cleanup
2. `gperr "goutils/errs"` to build pretty nested errors:
   - `gperr.Multiline()` for multiple operation attempts
   - `gperr.NewBuilder()` to collect errors
   - `gperr.NewGroup() + group.Go()` to collect errors of multiple concurrent operations
   - `gperr.PrependSubject()` to prepend subject to errors
3. `github.com/puzpuzpuz/xsync/v4` for lock-free thread safe maps
4. `goutils/synk` to retrieve and put byte buffer

## Testing

- Prefer scoped tests
- Prefer `testify`
- Use `-ldflags="-checklinkname=0"`

## HTML/JS

- js/html files are minified with bun, so embed the minified output such as `{filename}.min.html` or `{filename}.min.js` instead of the original file; see Makefile.
