package docker_mgr

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/client"
	"github.com/pouriatabari/my-replica/internal/utils"
)

type Manager struct {
	logger *utils.UILogger
	cli    *client.Client
	ctx    context.Context
}

func NewManager(logger *utils.UILogger) *Manager {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("Failed to initialize Docker client: " + err.Error())
		return &Manager{logger: logger, ctx: context.Background()}
	}

	return &Manager{
		logger: logger,
		cli:    cli,
		ctx:    context.Background(),
	}
}

func (m *Manager) ensureDockerClient() error {
	if m.cli == nil {
		return fmt.Errorf("docker client not initialized; ensure Docker daemon is available")
	}
	return nil
}

func (m *Manager) Close() error {
	if m.cli == nil {
		return nil
	}
	return m.cli.Close()
}

func (m *Manager) ValidateDockerAvailable() error {
	if err := m.ensureDockerClient(); err != nil {
		return err
	}

	_, err := m.cli.Ping(m.ctx)
	if err != nil {
		return fmt.Errorf("docker daemon ping failed: %w", err)
	}

	m.logger.Success("Docker daemon is available")
	return nil
}

func init() {
	if os.Getenv("DOCKER_API_VERSION") == "" {
		_ = os.Setenv("DOCKER_API_VERSION", "1.41")
	}
}
