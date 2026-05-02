# AGENTS.md

## Repo Map

### Proxy / route debugging

Anchors before broad repo search:

- Route lifecycle/finalization: `internal/route/route.go`
- Reverse proxy route construction/start: `internal/route/reverse_proxy.go`
- Entrypoint route resolution/context: `internal/entrypoint/http_server.go`, `internal/route/routes/context.go`
- Rule commands/matchers: `internal/route/rules/do.go`, `internal/route/rules/on.go`, `internal/route/rules/validate.go`
- Docker labels -> routes: `internal/docker/label.go`, `internal/docker/container.go`, `internal/route/provider/docker.go`
- Shared reverse proxy implementation: `goutils/http/reverseproxy/reverse_proxy.go`
- Response buffering/trailers used by rules: `goutils/http/response_modifier.go`
- Agent proxy headers/handler: `agent/pkg/agentproxy/config.go`, `agent/pkg/handler/proxy_http.go`

Root, `agent/`, `goutils/` = separate Go modules; run scoped tests from owning module dir.

### Wiki

- MDX: `webui/wiki/content/docs/` — **godoxy/** user/guide, **impl/** package map (`impl/index.mdx`).
- Fumadocs `defineDocs` + collections: `webui/source.config.ts`; main Vite app content path.
- In-app docs: `/docs`; `webui/src/lib/wiki/`, `webui/src/components/wiki/`.
- Standalone wiki: `webui/wiki/` — docs-only layout, OG/search.
- Impl wiki: `docs/impl` - synced from package READMEs

### Web frontend (`webui/`)

- Vite, TanStack Router/Start — `webui/src/router.tsx`, `webui/src/routes/`, generated `routeTree.gen.ts`.
- API: generated `webui/src/lib/api.ts`; CSRF/session in `webui/src/lib/`.
- `webui/src/types/godoxy/` mirrors backend config/providers — bump on RPC/schema change. `*/schema.json` generated, no edit.
- Config editor: `webui/src/components/config_editor/`, `sections.ts`; CodeMirror/yaml/rules `webui/src/lib/codemirror/`.

## Documentation

Significant change → refresh package `README.md`, wiki, webui types. Skip for ephemeral tasks (debug, repro).

## Go Patterns

1. `internal/task/task.go` lifetime:
   - `task.RootTask()` background ops
   - `parent.Subtask()` nesting
   - `OnFinished()`, `OnCancel()` cleanup
2. `gperr "goutils/errs"` nested errors:
   - `gperr.Multiline()` multi attempts
   - `gperr.NewBuilder()` collect
   - `gperr.NewGroup() + group.Go()` concurrent collect
   - `gperr.PrependSubject()` prepend subject
3. `github.com/puzpuzpuz/xsync/v4` lock-free concurrent maps
4. `goutils/synk` byte buffer get/put

## Testing

- Scoped tests preferred
- `testify`
- `-ldflags="-checklinkname=0"`
