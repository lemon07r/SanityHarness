// Package runner provides Docker container management and task execution.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// ExecResult holds the result of executing a command in a container.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Combined string
	Duration time.Duration
}

// DockerClient wraps the Docker SDK client with harness-specific operations.
type DockerClient struct {
	client *client.Client
}

// NewDockerClient creates a new Docker client.
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}

	return &DockerClient{client: cli}, nil
}

// Close closes the Docker client.
func (d *DockerClient) Close() error {
	return d.client.Close()
}

// Ping checks if the Docker daemon is accessible.
func (d *DockerClient) Ping(ctx context.Context) error {
	_, err := d.client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker daemon not accessible: %w", err)
	}
	return nil
}

// ImageExists checks if an image exists locally.
func (d *DockerClient) ImageExists(ctx context.Context, imageName string) (bool, error) {
	images, err := d.client.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageName {
				return true, nil
			}
		}
	}

	return false, nil
}

// PullImage pulls an image from a registry.
func (d *DockerClient) PullImage(ctx context.Context, imageName string) error {
	reader, err := d.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %s: %w", imageName, err)
	}
	defer func() { _ = reader.Close() }()

	// Consume the output to wait for completion
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("reading pull response: %w", err)
	}

	return nil
}

// EnsureImage ensures an image is available locally, pulling if necessary.
func (d *DockerClient) EnsureImage(ctx context.Context, imageName string, autoPull bool) error {
	exists, err := d.ImageExists(ctx, imageName)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	if !autoPull {
		return fmt.Errorf("image %s not found locally and auto-pull is disabled", imageName)
	}

	return d.PullImage(ctx, imageName)
}

// ContainerConfig holds configuration for creating a container.
type ContainerConfig struct {
	Image        string
	WorkspaceDir string
	Name         string
	User         string
	Env          []string
}

// CreateContainer creates a new container with the specified configuration.
func (d *DockerClient) CreateContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	containerCfg := &container.Config{
		Image: cfg.Image,
		Cmd:   []string{"sleep", "infinity"},
		Tty:   false,
		User:  cfg.User,
		Env:   cfg.Env,
	}

	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: cfg.WorkspaceDir,
				Target: "/workspace",
			},
		},
	}

	resp, err := d.client.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	return resp.ID, nil
}

// StartContainer starts a container.
func (d *DockerClient) StartContainer(ctx context.Context, containerID string) error {
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}
	return nil
}

// StopContainer stops a container with a timeout.
func (d *DockerClient) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	timeoutSecs := int(timeout.Seconds())
	if err := d.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeoutSecs}); err != nil {
		return fmt.Errorf("stopping container: %w", err)
	}
	return nil
}

// RemoveContainer removes a container.
func (d *DockerClient) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	if err := d.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("removing container: %w", err)
	}
	return nil
}

// Exec executes a command in a running container and returns the result.
func (d *DockerClient) Exec(ctx context.Context, containerID string, cmd []string, workdir string, timeout time.Duration) (*ExecResult, error) {
	start := time.Now()

	// Create exec context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create exec configuration
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   workdir,
	}

	// Create exec instance
	execResp, err := d.client.ContainerExecCreate(execCtx, containerID, execConfig)
	if err != nil {
		return nil, fmt.Errorf("creating exec: %w", err)
	}

	// Attach to exec instance
	attachResp, err := d.client.ContainerExecAttach(execCtx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("attaching to exec: %w", err)
	}
	defer attachResp.Close()

	// Read output
	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, attachResp.Reader)
	if err != nil {
		return nil, fmt.Errorf("reading exec output: %w", err)
	}

	// Get exit code
	var exitCode int
	for {
		inspectResp, err := d.client.ContainerExecInspect(execCtx, execResp.ID)
		if err != nil {
			return nil, fmt.Errorf("inspecting exec: %w", err)
		}

		if !inspectResp.Running {
			exitCode = inspectResp.ExitCode
			break
		}

		select {
		case <-execCtx.Done():
			return &ExecResult{
				ExitCode: -1,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				Combined: stdout.String() + stderr.String(),
				Duration: time.Since(start),
			}, fmt.Errorf("exec timed out")
		case <-time.After(50 * time.Millisecond):
			continue
		}
	}

	duration := time.Since(start)

	return &ExecResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Combined: stdout.String() + stderr.String(),
		Duration: duration,
	}, nil
}

// GetContainerLogs retrieves the logs from a container.
func (d *DockerClient) GetContainerLogs(ctx context.Context, containerID string) (string, error) {
	reader, err := d.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("getting container logs: %w", err)
	}
	defer func() { _ = reader.Close() }()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, reader)
	if err != nil {
		return "", fmt.Errorf("reading container logs: %w", err)
	}

	return buf.String(), nil
}
