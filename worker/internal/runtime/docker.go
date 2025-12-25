package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type Manager struct {
	cli *client.Client
}

func NewManager() (*Manager, error) {
	uid := os.Getuid()
	socketPath := fmt.Sprintf("unix:///run/user/%d/podman/podman.sock", uid)

	cli, err := client.NewClientWithOpts(
		client.WithHost(socketPath),
		client.WithAPIVersionNegotiation(),
	)

	if err != nil {
		return nil, err
	}

	return &Manager{cli: cli}, nil
}

func (m *Manager) RunCode(ctx context.Context, id string, language string, code string) (string, error) {
	img := "python:3.9-alpine"
	cmd := []string{"python", "-c", code}

	switch language {
	case "NODEJS":
		img = "node:18-alpine"
		cmd = []string{"node", "-e", code}
	case "GO":
		img = "python:3.9-alpine"
	}

	_, err := m.cli.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %v", err)
	}

	resp, err := m.cli.ContainerCreate(ctx, &container.Config{
		Image: img,
		Cmd:   cmd,
		Tty:   false,
	}, nil, nil, nil, "nano-"+id)

	if err != nil {
		return "", fmt.Errorf("failed to create container: %v", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start: %v", err)
	}

	statusCh, errCh := m.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return "", err
		}
	case <-statusCh:
	}

	out, err := m.cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", err
	}

	buf := new(strings.Builder)
	stdcopy.StdCopy(buf, os.Stderr, out)

	m.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{})

	return buf.String(), nil
}
