package proxmox

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"
)

type (
	LXCAction string
	LXCStatus string

	statusOnly struct {
		Status LXCStatus `json:"status"`
	}
	nameOnly struct {
		Name string `json:"name"`
	}
)

const (
	LXCStart    LXCAction = "start"
	LXCShutdown LXCAction = "shutdown"
	LXCSuspend  LXCAction = "suspend"
	LXCResume   LXCAction = "resume"
	LXCReboot   LXCAction = "reboot"
)

const (
	LXCStatusRunning   LXCStatus = "running"
	LXCStatusStopped   LXCStatus = "stopped"
	LXCStatusSuspended LXCStatus = "suspended" // placeholder, suspending lxc is experimental and the enum is undocumented
)

const (
	proxmoxTaskCheckInterval = 300 * time.Millisecond
	defaultLXCActionTimeout  = 2 * time.Minute
)

var ErrUnsupportedLXCAction = errors.New("unsupported LXC action")

func expectedLXCStatus(action LXCAction) (LXCStatus, error) {
	switch action {
	case LXCStart, LXCResume, LXCReboot:
		return LXCStatusRunning, nil
	case LXCShutdown:
		return LXCStatusStopped, nil
	case LXCSuspend:
		return LXCStatusSuspended, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedLXCAction, action)
	}
}

func (n *Node) LXCAction(ctx context.Context, vmid uint64, action LXCAction) error {
	expectedStatus, err := expectedLXCStatus(action)
	if err != nil {
		return err
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultLXCActionTimeout)
		defer cancel()
	}

	var upid proxmox.UPID
	reqCtx, reqCancel := context.WithTimeout(ctx, RequestTimeout)
	err = n.client.Post(reqCtx, fmt.Sprintf("/nodes/%s/lxc/%d/status/%s", n.name, vmid, action), nil, &upid)
	reqCancel()
	if err != nil {
		return err
	}
	if strings.Count(string(upid), ":") < 7 {
		return fmt.Errorf("invalid task id returned for %s: %q", action, upid)
	}

	task := proxmox.NewTask(upid, n.client.Client)
	checkTicker := time.NewTicker(proxmoxTaskCheckInterval)
	defer checkTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-checkTicker.C:
			reqCtx, reqCancel := context.WithTimeout(ctx, RequestTimeout)
			err := task.Ping(reqCtx)
			reqCancel()
			if err != nil {
				return err
			}
			if task.Status == proxmox.TaskRunning {
				continue
			}
			if task.ExitStatus != "OK" {
				return fmt.Errorf("proxmox task for %s failed: %s", action, task.ExitStatus)
			}
			reqCtx, reqCancel = context.WithTimeout(ctx, RequestTimeout)
			status, err := n.LXCStatus(reqCtx, vmid)
			reqCancel()
			if err != nil {
				return err
			}
			if status == expectedStatus {
				return nil
			}
		}
	}
}

func (n *Node) LXCName(ctx context.Context, vmid uint64) (string, error) {
	var name nameOnly
	if err := n.client.Get(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/current", n.name, vmid), &name); err != nil {
		return "", err
	}
	return name.Name, nil
}

func (n *Node) LXCStatus(ctx context.Context, vmid uint64) (LXCStatus, error) {
	var status statusOnly
	if err := n.client.Get(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/current", n.name, vmid), &status); err != nil {
		return "", err
	}
	return status.Status, nil
}

func (n *Node) LXCIsRunning(ctx context.Context, vmid uint64) (bool, error) {
	status, err := n.LXCStatus(ctx, vmid)
	return status == LXCStatusRunning, err
}

func (n *Node) LXCIsStopped(ctx context.Context, vmid uint64) (bool, error) {
	status, err := n.LXCStatus(ctx, vmid)
	return status == LXCStatusStopped, err
}

func (n *Node) LXCSetShutdownTimeout(ctx context.Context, vmid uint64, timeout time.Duration) error {
	return n.client.Put(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", n.name, vmid), map[string]any{
		"startup": fmt.Sprintf("down=%.0f", timeout.Seconds()),
	}, nil)
}

func parseCIDR(s string) net.IP {
	if s == "" {
		return nil
	}
	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		return nil
	}
	return privateIPOrNil(ip)
}

func privateIPOrNil(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	if ip.IsPrivate() {
		return ip
	}
	return nil
}

func getIPFromNet(s string) (res []net.IP) { // name:...,bridge:...,gw=..,ip=...,ip6=...
	if s == "" {
		return nil
	}
	var i4, i6 net.IP
	cidrIndex := strings.Index(s, "ip=")
	if cidrIndex != -1 {
		cidrIndex += 3
		slash := strings.Index(s[cidrIndex:], "/")
		if slash != -1 {
			i4 = privateIPOrNil(net.ParseIP(s[cidrIndex : cidrIndex+slash]))
		} else {
			i4 = privateIPOrNil(net.ParseIP(s[cidrIndex:]))
		}
	}
	cidr6Index := strings.Index(s, "ip6=")
	if cidr6Index != -1 {
		cidr6Index += 4
		slash := strings.Index(s[cidr6Index:], "/")
		if slash != -1 {
			i6 = privateIPOrNil(net.ParseIP(s[cidr6Index : cidr6Index+slash]))
		} else {
			i6 = privateIPOrNil(net.ParseIP(s[cidr6Index:]))
		}
	}
	if i4 != nil {
		res = append(res, i4)
	}
	if i6 != nil {
		res = append(res, i6)
	}
	return res
}

// LXCGetIPs returns the ip addresses of the container
// it first tries to get the ip addresses from the interfaces
// if that fails, it gets the ip addresses from the config (offline containers)
func (n *Node) LXCGetIPs(ctx context.Context, vmid int) (res []net.IP, err error) {
	return n.LXCGetIPsWithStatus(ctx, vmid, "")
}

// LXCGetIPsWithStatus returns the ip addresses of the container.
// Stopped and suspended LXCs skip the interfaces endpoint and go directly to config data.
func (n *Node) LXCGetIPsWithStatus(ctx context.Context, vmid int, status string) (res []net.IP, err error) {
	switch LXCStatus(status) {
	case LXCStatusStopped, LXCStatusSuspended:
		return n.LXCGetIPsFromConfig(ctx, vmid)
	}

	ips, err := n.LXCGetIPsFromInterfaces(ctx, vmid)
	if err != nil {
		return nil, err
	}
	if len(ips) > 0 {
		return ips, nil
	}
	ips, err = n.LXCGetIPsFromConfig(ctx, vmid)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

// LXCGetIPsFromConfig returns the ip addresses of the container from the config
func (n *Node) LXCGetIPsFromConfig(ctx context.Context, vmid int) (res []net.IP, err error) {
	var cfg proxmox.ContainerConfig
	if err := n.client.Get(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", n.name, vmid), &cfg); err != nil {
		return nil, err
	}

	networks := slices.SortedFunc(maps.Keys(cfg.Nets), func(a, b string) int {
		aIndex, _ := strconv.Atoi(strings.TrimPrefix(a, "net"))
		bIndex, _ := strconv.Atoi(strings.TrimPrefix(b, "net"))
		return cmp.Compare(aIndex, bIndex)
	})
	for _, network := range networks {
		res = append(res, getIPFromNet(cfg.Nets[network])...)
	}
	return res, nil
}

// LXCGetIPsFromInterfaces returns the ip addresses of the container from the interfaces
// it will return nothing if the container is stopped
func (n *Node) LXCGetIPsFromInterfaces(ctx context.Context, vmid int) ([]net.IP, error) {
	type Interface struct {
		IPv4 string `json:"inet"`
		IPv6 string `json:"inet6"`
		Name string `json:"name"`
	}
	var res []Interface
	if err := n.client.Get(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/interfaces", n.name, vmid), &res); err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0)
	for _, ip := range res {
		if ip.Name == "lo" ||
			strings.HasPrefix(ip.Name, "br-") ||
			strings.HasPrefix(ip.Name, "veth") ||
			strings.HasPrefix(ip.Name, "docker") {
			continue
		}
		if ip := parseCIDR(ip.IPv4); ip != nil {
			ips = append(ips, ip)
		}
		if ip := parseCIDR(ip.IPv6); ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}
