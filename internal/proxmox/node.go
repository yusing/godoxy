package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/luthermonson/go-proxmox"
	"github.com/yusing/goutils/pool"
)

type NodeConfig struct {
	Node    string `json:"node" validate:"required"`
	VMID    int    `json:"vmid" validate:"required"`
	VMName  string `json:"vmname,omitempty"`
	Service string `json:"service,omitempty"`
} // @name ProxmoxNodeConfig

type Node struct {
	name   string
	id     string // likely node/<name>
	client *Client

	// statsScriptInitErrs *xsync.Map[int, error]
}

var Nodes = pool.New[*Node]("proxmox_nodes")

func NewNode(client *Client, name, id string) *Node {
	return &Node{
		name:   name,
		id:     id,
		client: client,
		// statsScriptInitErrs: xsync.NewMap[int, error](xsync.WithGrowOnly()),
	}
}

func AvailableNodeNames() string {
	if Nodes.Size() == 0 {
		return ""
	}
	var sb strings.Builder
	for _, node := range Nodes.Iter {
		sb.WriteString(node.name)
		sb.WriteString(", ")
	}
	return sb.String()[:sb.Len()-2]
}

func (n *Node) Key() string {
	return n.name
}

func (n *Node) Name() string {
	return n.name
}

func (n *Node) Client() *Client {
	return n.client
}

func (n *Node) String() string {
	return fmt.Sprintf("%s (%s)", n.name, n.id)
}

func (n *Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"name": n.name,
		"id":   n.id,
	})
}

func (n *Node) Get(ctx context.Context, path string, v any) error {
	return n.client.Get(ctx, path, v)
}

// NodeCommand connects to the Proxmox VNC websocket and streams command output.
// It returns an io.ReadCloser that streams the command output.
func (n *Node) NodeCommand(ctx context.Context, command string) (io.ReadCloser, error) {
	if !n.client.HasSession() {
		return nil, ErrNoSession
	}

	node := proxmox.NewNode(n.client.Client, n.name)
	term, err := node.TermProxy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get term proxy: %w", err)
	}

	send, recv, errs, closeWS, err := node.TermWebSocket(term)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to term websocket: %w", err)
	}

	// Wrap the websocket closer to also close HTTP transport connections.
	// This prevents goroutine leaks when streaming connections are interrupted.
	httpClient := n.client.GetHTTPClient()
	closeFn := func() error {
		closeTransportConnections(httpClient)
		return closeWS()
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

	// Send command
	cmd := []byte(command + "\n")
	if err := handleSend(cmd); err != nil {
		return nil, err
	}

	// Create a pipe to stream the websocket messages
	pr, pw := io.Pipe()

	// Command line without trailing newline for matching in output
	cmdLine := cmd[:len(cmd)-1]

	// Start a goroutine to read from websocket and write to pipe
	go func() {
		defer closeFn()
		defer pw.Close()

		seenCommand := false
		shouldSkip := true

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
					// First, check if this message contains our command echo
					if !seenCommand && bytes.Contains(msg, cmdLine) {
						seenCommand = true
					}
					// Only stop skipping after we've seen the command AND output markers
					if seenCommand {
						if bytes.Contains(msg, []byte("\x1b[H")) || // watch cursor home
							bytes.Contains(msg, []byte("\x1b[?2004l")) { // bracket paste OFF (command ended)
							shouldSkip = false
						}
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
