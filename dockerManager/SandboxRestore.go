package dockerManager

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
)

// RestoreSandboxByContainerID tries to rebind a sandbox to an existing container.
// If the container exists but is stopped, it will be started.
func (dm *DockerManager) RestoreSandboxByContainerID(containerID, sourceCodeDir string) (*Sandbox, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, fmt.Errorf("empty sandbox container id")
	}

	ctx := context.Background()
	inspect, err := dm.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("sandbox container not found: %s", containerID)
		}
		return nil, err
	}

	if inspect.State != nil && !inspect.State.Running {
		if err := dm.cli.ContainerStart(ctx, inspect.ID, container.StartOptions{}); err != nil {
			return nil, fmt.Errorf("start sandbox container failed: %w", err)
		}
	}

	ip, _, err := GetContainerIPAndPortMap(dm.cli, inspect.ID)
	if err != nil {
		return nil, err
	}
	id := inspect.ID
	if len(id) > 12 {
		id = id[:12]
	}
	return NewSandboxFromExisting(dm, id, ip, sourceCodeDir), nil
}
