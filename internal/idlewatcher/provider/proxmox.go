package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
)

type ProxmoxProvider struct {
	*proxmox.Node

	vmid    uint64
	lxcName string
	running bool
}

const proxmoxStateCheckInterval = 1 * time.Second

var ErrNodeNotFound = gperr.New("node not found in pool")

func NewProxmoxProvider(ctx context.Context, nodeName string, vmid uint64) (idlewatcher.Provider, error) {
	if nodeName == "" || vmid == 0 {
		return nil, errors.New("node name and vmid are required")
	}

	node, ok := proxmox.Nodes.Get(nodeName)
	if !ok {
		return nil, ErrNodeNotFound.Subject(nodeName).
			Withf("available nodes: %s", proxmox.AvailableNodeNames())
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	lxcName, err := node.LXCName(ctx, vmid)
	if err != nil {
		return nil, err
	}
	return &ProxmoxProvider{Node: node, vmid: vmid, lxcName: lxcName}, nil
}

func (p *ProxmoxProvider) ContainerPause(ctx context.Context) error {
	return p.LXCAction(ctx, p.vmid, proxmox.LXCSuspend)
}

func (p *ProxmoxProvider) ContainerUnpause(ctx context.Context) error {
	return p.LXCAction(ctx, p.vmid, proxmox.LXCResume)
}

func (p *ProxmoxProvider) ContainerStart(ctx context.Context) error {
	return p.LXCAction(ctx, p.vmid, proxmox.LXCStart)
}

func (p *ProxmoxProvider) ContainerStop(ctx context.Context, _ idlewatcher.ContainerSignal, _ int) error {
	return p.LXCAction(ctx, p.vmid, proxmox.LXCShutdown)
}

func (p *ProxmoxProvider) ContainerKill(ctx context.Context, _ idlewatcher.ContainerSignal) error {
	return p.LXCAction(ctx, p.vmid, proxmox.LXCShutdown)
}

func (p *ProxmoxProvider) ContainerStatus(ctx context.Context) (idlewatcher.ContainerStatus, error) {
	status, err := p.LXCStatus(ctx, p.vmid)
	if err != nil {
		return idlewatcher.ContainerStatusError, err
	}
	switch status {
	case proxmox.LXCStatusRunning:
		return idlewatcher.ContainerStatusRunning, nil
	case proxmox.LXCStatusStopped:
		return idlewatcher.ContainerStatusStopped, nil
	}
	return idlewatcher.ContainerStatusError, fmt.Errorf("%w: %s", idlewatcher.ErrUnexpectedContainerStatus, string(status))
}

func (p *ProxmoxProvider) Watch(ctx context.Context) (<-chan watcher.Event, <-chan error) {
	eventCh := make(chan watcher.Event)
	errCh := make(chan error)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		var err error
		p.running, err = p.LXCIsRunning(ctx, p.vmid)
		if err != nil {
			errCh <- err
			return
		}

		ticker := time.NewTicker(proxmoxStateCheckInterval)
		defer ticker.Stop()

		event := watcher.Event{
			Type:      watcherEvents.EventTypeDocker,
			ActorID:   strconv.FormatUint(p.vmid, 10),
			ActorName: p.lxcName,
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, err := p.ContainerStatus(ctx)
				if err != nil {
					errCh <- err
					return
				}
				running := status == idlewatcher.ContainerStatusRunning
				if p.running != running {
					p.running = running
					if running {
						event.Action = watcherEvents.ActionContainerStart
					} else {
						event.Action = watcherEvents.ActionContainerStop
					}
					eventCh <- event
				}
			}
		}
	}()

	return eventCh, errCh
}

func (p *ProxmoxProvider) Close() {
	// noop
}
