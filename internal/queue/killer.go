package queue

import (
	"context"
	"fmt"
	"os/exec"
)

// ContainerKiller kills a running docker container by name. Implemented by
// dockerKiller in prod; fakes in tests.
type ContainerKiller interface {
	Kill(ctx context.Context, containerName string) error
}

type dockerKiller struct{}

func NewDockerKiller() ContainerKiller { return &dockerKiller{} }

func (d *dockerKiller) Kill(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "docker", "kill", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker kill %s: %w: %s", name, err, string(out))
	}
	return nil
}
