# Socket Proxy Reverse Proxy

This package provides an HTTP reverse proxy implementation for proxying requests to Unix sockets (typically Docker sockets). It is based on Go's `net/http/httputil.ReverseProxy` with simplifications for socket proxying use cases.

## Differences from `net/http/httputil.ReverseProxy`

| Feature               | stdlib `httputil.ReverseProxy` | This package                        |
| --------------------- | ------------------------------ | ----------------------------------- |
| Request modification  | `Director` or `Rewrite`        | `Director` only                     |
| Response modification | `ModifyResponse` hook          | Not supported                       |
| Buffering             | Uses `io.Copy`                 | Uses `ioutils.CopyCloseWithContext` |
| Flush support         | `Flush()` method               | Not exposed                         |

### Key Simplifications

1. **Director only**: Only the `Director` function is supported. The stdlib's `Rewrite` type and `ModifyResponse` hook are removed.

2. **Context-aware body copying**: Uses `ioutils.CopyCloseWithContext` which:

   - Respects request context for cancellation
   - Uses `Content-Length` for optimal copying when available
   - Properly handles trailer headers

3. **No buffering**: Unlike the stdlib which can buffer responses, this implementation streams directly to the client.

## Usage

```go
rp := &reverseproxy.ReverseProxy{
    Director: func(req *http.Request) {
        req.URL.Scheme = "http"
        req.URL.Host = "api.moby.localhost"
        req.RequestURI = req.URL.String()
    },
    Transport: &http.Transport{
        DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
            return net.DialTimeout("unix", "/var/run/docker.sock", 5*time.Second)
        },
        DisableCompression: true,
    },
}

http.HandleFunc("/", rp.ServeHTTP)
http.ListenAndServe(":2375", nil)
```

## Socket Proxy Integration

The socket proxy uses this package in `socket-proxy/pkg/handler.go`:

```go
func dockerSocketHandler(socket string) http.HandlerFunc {
    rp := &reverseproxy.ReverseProxy{
        Director: func(req *http.Request) {
            req.URL.Scheme = "http"
            req.URL.Host = "api.moby.localhost"
            req.RequestURI = req.URL.String()
        },
        Transport: &http.Transport{
            DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
                dialer := &net.Dialer{KeepAlive: 1 * time.Second}
                return dialer.DialContext(ctx, "unix", socket)
            },
            DisableCompression: true,
        },
    }
    return rp.ServeHTTP
}
```

## License

This code is based on Go's `net/http/httputil` and is licensed under the BSD-style license found at the top of `reverse_proxy.go`.
