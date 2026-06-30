package runtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	memoryLimit = 128 * 1024 * 1024 // 128 MB
	nanoCPUs    = 500_000_000        // 0.5 CPU
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

func (m *Manager) RunCode(ctx context.Context, language, code, input string, publish func(msg, level string)) error {
	img, cmd, env := runtimeConfig(language, code)

	if input != "" {
		env = append(env, "NANO_INPUT="+input)
	}

	reader, err := m.cli.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %v", err)
	}
	_, _ = io.Copy(io.Discard, reader)
	_ = reader.Close()

	resp, err := m.cli.ContainerCreate(ctx,
		&container.Config{
			Image: img,
			Cmd:   cmd,
			Env:   env,
			Tty:   false,
		},
		&container.HostConfig{
			Resources: container.Resources{
				Memory:   memoryLimit,
				NanoCPUs: nanoCPUs,
			},
			NetworkMode: "none",
		},
		nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}

	defer func() {
		cleanupCtx := context.Background()
		_ = m.cli.ContainerRemove(cleanupCtx, resp.ID, container.RemoveOptions{Force: true})
	}()

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	logOut, err := m.cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return fmt.Errorf("failed to attach logs: %v", err)
	}
	defer func() { _ = logOut.Close() }()

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutR)
		for scanner.Scan() {
			publish(scanner.Text(), "stdout")
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrR)
		for scanner.Scan() {
			publish(scanner.Text(), "stderr")
		}
	}()

	_, _ = stdcopy.StdCopy(stdoutW, stderrW, logOut)
	_ = stdoutW.Close()
	_ = stderrW.Close()
	wg.Wait()

	return ctx.Err()
}

func runtimeConfig(language, code string) (img string, cmd []string, env []string) {
	switch language {
	case "NODEJS":
		return "node:18-alpine", []string{"node", "-e", code}, nil
	case "GO":
		return "golang:1.21-alpine",
			[]string{"sh", "-c", `printf '%s' "$NANO_CODE" > /tmp/main.go && go run /tmp/main.go`},
			[]string{"NANO_CODE=" + wrapGoCode(code)}
	default: // PYTHON
		return "python:3.9-alpine", []string{"python", "-c", code}, nil
	}
}

func wrapGoCode(code string) string {
	if strings.Contains(code, "package main") {
		return code
	}
	return "package main\n\nfunc main() {\n" + code + "\n}\n"
}
