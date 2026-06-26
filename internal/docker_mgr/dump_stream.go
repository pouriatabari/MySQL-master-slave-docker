package docker_mgr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
)

func (m *Manager) ExecStream(containerName string, cmd []string, writer io.Writer) error {
	if err := m.ensureDockerClient(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	defer cancel()

	m.logger.Info(fmt.Sprintf("Exec stream in %s: %s", containerName, sanitizeCmd(cmd)))

	execResp, err := m.cli.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("exec create failed: %w", err)
	}

	attachResp, err := m.cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return fmt.Errorf("exec attach failed: %w", err)
	}
	defer attachResp.Close()

	var stderrBuf bytes.Buffer
	_, err = stdcopy.StdCopy(writer, &stderrBuf, attachResp.Reader)
	if err != nil {
		return fmt.Errorf("exec stream read failed: %w", err)
	}

	inspect, err := m.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return fmt.Errorf("exec inspect failed: %w", err)
	}

	if inspect.ExitCode != 0 {
		return fmt.Errorf("exec stream failed with exit code %d: %s", inspect.ExitCode, stderrBuf.String())
	}

	return nil
}
