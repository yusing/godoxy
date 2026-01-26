package proxmox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/luthermonson/go-proxmox"
)

var ErrNoSession = fmt.Errorf("no session found, make sure username and password are set")

// closeTransportConnections forces close idle HTTP connections to prevent goroutine leaks.
// This is needed because the go-proxmox library's TermWebSocket closer doesn't close
// the underlying HTTP/2 connections, leaving goroutines stuck in writeLoop/readLoop.
func closeTransportConnections(httpClient *http.Client) {
	if tr, ok := httpClient.Transport.(*http.Transport); ok {
		tr.CloseIdleConnections()
	}
}

// LXCCommand connects to the Proxmox VNC websocket and streams command output.
// It returns an io.ReadCloser that streams the command output.
func (n *Node) LXCCommand(ctx context.Context, vmid int, command string) (io.ReadCloser, error) {
	node := proxmox.NewNode(n.client.Client, n.name)
	lxc, err := node.Container(ctx, vmid)
	if err != nil {
		return nil, fmt.Errorf("failed to get container: %w", err)
	}

	if lxc.Status != "running" {
		return io.NopCloser(bytes.NewReader(fmt.Appendf(nil, "container %d is not running, status: %s\n", vmid, lxc.Status))), nil
	}

	return n.NodeCommand(ctx, fmt.Sprintf("pct exec %d -- %s", vmid, command))
}

// LXCJournalctl streams journalctl output for the given service.
//
// If services are not empty, it will be used to filter the output by service.
// If limit is greater than 0, it will be used to limit the number of lines of output.
func (n *Node) LXCJournalctl(ctx context.Context, vmid int, services []string, limit int) (io.ReadCloser, error) {
	command, err := formatJournalctl(services, limit)
	if err != nil {
		return nil, err
	}
	return n.LXCCommand(ctx, vmid, command)
}

// LXCTail streams tail output for the given file.
//
// If limit is greater than 0, it will be used to limit the number of lines of output.
func (n *Node) LXCTail(ctx context.Context, vmid int, files []string, limit int) (io.ReadCloser, error) {
	command, err := formatTail(files, limit)
	if err != nil {
		return nil, err
	}
	return n.LXCCommand(ctx, vmid, command)
}
