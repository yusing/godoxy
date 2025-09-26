package provider

import (
	"context"
	"strconv"
	"time"

	"github.com/yusing/godoxy/internal/gperr"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher"
	"github.com/yusing/godoxy/internal/watcher/events"
)

type ProxmoxProvider struct {
	*proxmox.Node

	vmid    int
	lxcName string
	running bool
}

const proxmoxStateCheckInterval = 1 * time.Second

var ErrNodeNotFound = gperr.New("node not found in pool")

func NewProxmoxProvider(nodeName string, vmid int) (idlewatcher.Provider, error) {
	node, ok := proxmox.Nodes.Get(nodeName)
	if !ok {
		return nil, ErrNodeNotFound.Subject(nodeName).
			Withf("available nodes: %s", proxmox.AvailableNodeNames())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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

func (p *ProxmoxProvider) ContainerStop(ctx context.Context, _ types.ContainerSignal, _ int) error {
	return p.LXCAction(ctx, p.vmid, proxmox.LXCShutdown)
}

func (p *ProxmoxProvider) ContainerKill(ctx context.Context, _ types.ContainerSignal) error {
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
	return idlewatcher.ContainerStatusError, idlewatcher.ErrUnexpectedContainerStatus.Subject(string(status))
}

func (p *ProxmoxProvider) Watch(ctx context.Context) (<-chan watcher.Event, <-chan gperr.Error) {
	eventCh := make(chan watcher.Event)
	errCh := make(chan gperr.Error)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		var err error
		p.running, err = p.LXCIsRunning(ctx, p.vmid)
		if err != nil {
			errCh <- gperr.Wrap(err)
			return
		}

		ticker := time.NewTicker(proxmoxStateCheckInterval)
		defer ticker.Stop()

		event := watcher.Event{
			Type:      events.EventTypeDocker,
			ActorID:   strconv.Itoa(p.vmid),
			ActorName: p.lxcName,
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, err := p.ContainerStatus(ctx)
				if err != nil {
					errCh <- gperr.Wrap(err)
					return
				}
				running := status == idlewatcher.ContainerStatusRunning
				if p.running != running {
					p.running = running
					if running {
						event.Action = events.ActionContainerStart
					} else {
						event.Action = events.ActionContainerStop
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
