package proxmox

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/gorilla/websocket"
)

var ErrNoSession = fmt.Errorf("no session found, make sure username and password are set")

// LXCCommand connects to the Proxmox VNC websocket and streams command output.
// It returns an io.ReadCloser that streams the command output.
func (n *Node) LXCCommand(ctx context.Context, vmid int, command string) (io.ReadCloser, error) {
	if !n.client.HasSession() {
		return nil, ErrNoSession
	}

	node, err := n.client.Node(ctx, n.name)
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	term, err := node.TermProxy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get term proxy: %w", err)
	}

	send, recv, errs, close, err := node.TermWebSocket(term)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to term websocket: %w", err)
	}

	handleSend := func(data []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case send <- data:
			return nil
		case err := <-errs:
			return fmt.Errorf("failed to send: %w", err)
		}
	}

	// Send command: `pct exec <vmid> -- <command>`
	cmd := fmt.Appendf(nil, "pct exec %d -- %s\n", vmid, command)
	if err := handleSend(cmd); err != nil {
		return nil, err
	}

	// Create a pipe to stream the websocket messages
	pr, pw := io.Pipe()

	shouldSkip := true

	// Start a goroutine to read from websocket and write to pipe
	go func() {
		defer close()
		defer pw.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-recv:
				// skip the header message like

				// Linux pve 6.17.4-1-pve #1 SMP PREEMPT_DYNAMIC PMX 6.17.4-1 (2025-12-03T15:42Z) x86_64
				//
				// The programs included with the Debian GNU/Linux system are free software;
				// the exact distribution terms for each program are described in the
				// individual files in /usr/share/doc/*/copyright.
				//
				// Debian GNU/Linux comes with ABSOLUTELY NO WARRANTY, to the extent
				// permitted by applicable law.
				//
				// root@pve:~# pct exec 101 -- journalctl -u "sftpgo" -f
				//
				// send begins after the line above
				if shouldSkip {
					if bytes.Contains(msg, cmd[:len(cmd)-2]) { // without the \n
						shouldSkip = false
					}
					continue
				}
				if _, err := pw.Write(msg); err != nil {
					return
				}
			case err := <-errs:
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						_ = pw.Close()
						return
					}
					_ = pw.CloseWithError(err)
					return
				}
			}
		}
	}()

	return pr, nil
}

// LXCJournalctl streams journalctl output for the given service.
func (n *Node) LXCJournalctl(ctx context.Context, vmid int, service string) (io.ReadCloser, error) {
	return n.LXCCommand(ctx, vmid, fmt.Sprintf("journalctl -u %q -f", service))
}
