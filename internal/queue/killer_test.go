package queue_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vaibhav0806/era/internal/queue"
)

func TestDockerKiller_InvokesDockerKill(t *testing.T) {
	if os.Getenv("DOCKER_E2E") != "1" {
		t.Skip("set DOCKER_E2E=1 to run")
	}
	name := fmt.Sprintf("era-killer-test-%d", time.Now().UnixNano())
	cmd := exec.Command("docker", "run", "-d", "--name", name, "alpine", "sleep", "60")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	defer exec.Command("docker", "rm", "-f", name).Run()

	k := queue.NewDockerKiller()
	require.NoError(t, k.Kill(context.Background(), name))

	chk := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", name)
	out, _ = chk.Output()
	require.Equal(t, "false", strings.TrimSpace(string(out)))
}
