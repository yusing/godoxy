package proxmox

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
)

func TestRefreshSessionReauthenticatesRejectedUnexpiredSession(t *testing.T) {
	var (
		ticketBodiesMu sync.Mutex
		ticketBodies   []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/ticket":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read ticket request: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			ticketBodiesMu.Lock()
			ticketBodies = append(ticketBodies, string(body))
			call := len(ticketBodies)
			ticketBodiesMu.Unlock()

			switch call {
			case 1:
				_, _ = w.Write([]byte(`{"data":{"username":"root@pam","ticket":"OLD-TICKET","CSRFPreventionToken":"OLD-CSRF"}}`))
			case 2:
				w.WriteHeader(http.StatusUnauthorized)
			case 3:
				_, _ = w.Write([]byte(`{"data":{"username":"root@pam","ticket":"NEW-TICKET","CSRFPreventionToken":"NEW-CSRF"}}`))
			default:
				t.Errorf("unexpected ticket request %d", call)
				w.WriteHeader(http.StatusInternalServerError)
			}
		case "/version":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	credentials := &goproxmox.Credentials{
		Username: "root",
		Password: "secret",
		Realm:    "pam",
	}
	client := NewClient(
		srv.URL,
		goproxmox.WithHTTPClient(srv.Client()),
		goproxmox.WithCredentials(credentials),
	)
	cfg := &Config{
		Username: "root",
		Password: "secret",
		Realm:    "pam",
		client:   client,
	}

	require.NoError(t, client.CreateSession(t.Context()))
	require.Equal(t, "OLD-TICKET", client.Session().Ticket)
	require.NoError(t, cfg.refreshSession(t.Context()))
	require.Equal(t, "NEW-TICKET", client.Session().Ticket)

	ticketBodiesMu.Lock()
	defer ticketBodiesMu.Unlock()
	require.Len(t, ticketBodies, 3)
	require.Contains(t, ticketBodies[0], `"password":"secret"`)
	require.Contains(t, ticketBodies[1], `"password":"OLD-TICKET"`)
	require.Contains(t, ticketBodies[2], `"username":"root"`)
	require.Contains(t, ticketBodies[2], `"password":"secret"`)
	require.Contains(t, ticketBodies[2], `"realm":"pam"`)
}
