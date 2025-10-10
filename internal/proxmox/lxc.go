package proxmox

import (
	"context"
	"fmt"
	"net"
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
	proxmoxReqTimeout        = 3 * time.Second
	proxmoxTaskCheckInterval = 300 * time.Millisecond
)

func (n *Node) LXCAction(ctx context.Context, vmid int, action LXCAction) error {
	var upid proxmox.UPID
	if err := n.client.Post(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/%s", n.name, vmid, action), nil, &upid); err != nil {
		return err
	}

	task := proxmox.NewTask(upid, n.client)
	checkTicker := time.NewTicker(proxmoxTaskCheckInterval)
	defer checkTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-checkTicker.C:
			if err := task.Ping(ctx); err != nil {
				return err
			}
			if task.Status != proxmox.TaskRunning {
				status, err := n.LXCStatus(ctx, vmid)
				if err != nil {
					return err
				}
				switch status {
				case LXCStatusRunning:
					if action == LXCStart {
						return nil
					}
				case LXCStatusStopped:
					if action == LXCShutdown {
						return nil
					}
				case LXCStatusSuspended:
					if action == LXCSuspend {
						return nil
					}
				}
			}
		}
	}
}

func (n *Node) LXCName(ctx context.Context, vmid int) (string, error) {
	var name nameOnly
	if err := n.client.Get(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/current", n.name, vmid), &name); err != nil {
		return "", err
	}
	return name.Name, nil
}

func (n *Node) LXCStatus(ctx context.Context, vmid int) (LXCStatus, error) {
	var status statusOnly
	if err := n.client.Get(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/status/current", n.name, vmid), &status); err != nil {
		return "", err
	}
	return status.Status, nil
}

func (n *Node) LXCIsRunning(ctx context.Context, vmid int) (bool, error) {
	status, err := n.LXCStatus(ctx, vmid)
	return status == LXCStatusRunning, err
}

func (n *Node) LXCIsStopped(ctx context.Context, vmid int) (bool, error) {
	status, err := n.LXCStatus(ctx, vmid)
	return status == LXCStatusStopped, err
}

func (n *Node) LXCSetShutdownTimeout(ctx context.Context, vmid int, timeout time.Duration) error {
	return n.client.Put(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", n.name, vmid), map[string]interface{}{
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
// it first tries to get the ip addresses from the config
// if that fails, it gets the ip addresses from the interfaces
func (n *Node) LXCGetIPs(ctx context.Context, vmid int) (res []net.IP, err error) {
	ips, err := n.LXCGetIPsFromConfig(ctx, vmid)
	if err != nil {
		return nil, err
	}
	if len(ips) > 0 {
		return ips, nil
	}
	ips, err = n.LXCGetIPsFromInterfaces(ctx, vmid)
	if err != nil {
		return nil, err
	}
	return ips, nil
}

// LXCGetIPsFromConfig returns the ip addresses of the container from the config
func (n *Node) LXCGetIPsFromConfig(ctx context.Context, vmid int) (res []net.IP, err error) {
	type Config struct {
		Net0 string `json:"net0"`
		Net1 string `json:"net1"`
		Net2 string `json:"net2"`
	}
	var cfg Config
	if err := n.client.Get(ctx, fmt.Sprintf("/nodes/%s/lxc/%d/config", n.name, vmid), &cfg); err != nil {
		return nil, err
	}

	res = append(res, getIPFromNet(cfg.Net0)...)
	res = append(res, getIPFromNet(cfg.Net1)...)
	res = append(res, getIPFromNet(cfg.Net2)...)
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
