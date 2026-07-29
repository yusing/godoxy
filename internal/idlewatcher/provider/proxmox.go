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
)

type ProxmoxProvider struct {
	*proxmox.Node

	vmid               uint64
	lxcName            string
	running            bool
	stateCheckInterval time.Duration
}

const proxmoxStateCheckInterval = 1 * time.Second

func NewProxmoxProvider(ctx context.Context, nodeName string, vmid uint64) (idlewatcher.Provider, error) {
	if nodeName == "" || vmid == 0 {
		return nil, errors.New("node name and vmid are required")
	}

	node, err := proxmox.NodeFromCtx(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("%w (available nodes: %s)", err, proxmox.AvailableNodeNames(ctx))
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

		interval := p.stateCheckInterval
		if interval == 0 {
			interval = proxmoxStateCheckInterval
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		event := watcher.Event{
			Type:      watcherEvents.EventTypeDocker,
			ActorID:   strconv.FormatUint(p.vmid, 10),
			ActorName: p.lxcName,
		}
		initialized := false
		for {
			status, err := p.ContainerStatus(ctx)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case errCh <- err:
				}
			} else {
				running := status == idlewatcher.ContainerStatusRunning
				if !initialized {
					p.running = running
					initialized = true
				} else if p.running != running {
					p.running = running
					if running {
						event.Action = watcherEvents.ActionContainerStart
					} else {
						event.Action = watcherEvents.ActionContainerStop
					}
					select {
					case <-ctx.Done():
						return
					case eventCh <- event:
					}
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return eventCh, errCh
}

func (p *ProxmoxProvider) Close() {
	// noop
}
