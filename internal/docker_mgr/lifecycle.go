package docker_mgr

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/pouriatabari/my-replica/internal/ui"
	"github.com/pouriatabari/my-replica/internal/utils"
)

func (m *Manager) Down(cfg *ui.SetupConfig) error {
	if err := m.ensureDockerClient(); err != nil {
		return err
	}

	names := []string{cfg.MasterContainer, cfg.SlaveContainer}
	for _, name := range names {
		if err := m.stopAndRemoveContainer(name); err != nil {
			return err
		}
	}

	m.logger.Success("Containers stopped and removed")
	return nil
}

func (m *Manager) stopAndRemoveContainer(name string) error {
	_, err := m.cli.ContainerInspect(m.ctx, name)
	if err != nil {
		m.logger.Info("Container not found, skipping: " + name)
		return nil
	}

	m.logger.Info("Stopping container: " + name)
	timeout := 10
	if err := m.cli.ContainerStop(m.ctx, name, container.StopOptions{Timeout: &timeout}); err != nil {
		m.logger.Warn("Stop container failed (continuing): " + name + ": " + err.Error())
	}

	m.logger.Info("Removing container: " + name)
	if err := m.cli.ContainerRemove(m.ctx, name, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	}); err != nil {
		return fmt.Errorf("remove container %s failed: %w", name, err)
	}

	return nil
}

func (m *Manager) Cleanup(cfg *ui.SetupConfig) error {
	if err := m.ensureDockerClient(); err != nil {
		return err
	}

	if err := m.Down(cfg); err != nil {
		return err
	}

	m.logger.Info("Removing docker network: " + cfg.NetworkName)
	args := filters.NewArgs()
	args.Add("name", cfg.NetworkName)

	nws, err := m.cli.NetworkList(m.ctx, network.ListOptions{Filters: args})
	if err != nil {
		return fmt.Errorf("network list failed: %w", err)
	}

	for _, nw := range nws {
		if err := m.cli.NetworkRemove(m.ctx, nw.ID); err != nil {
			return fmt.Errorf("network remove failed: %w", err)
		}
	}

	dumpFiles := []string{
		filepath.Join(cfg.BaseDir, "master", "db_master", "data.sql"),
		filepath.Join(cfg.BaseDir, "slave", "db_slave", "data.sql"),
	}
	for _, f := range dumpFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove dump file %s failed: %w", f, err)
		}
	}

	m.logger.Success("Cleanup completed")
	return nil
}

func (m *Manager) ResetDataDirs(cfg *ui.SetupConfig) error {
	dirs := []string{
		filepath.Join(cfg.BaseDir, "master", "db_master"),
		filepath.Join(cfg.BaseDir, "slave", "db_slave"),
	}

	for _, dir := range dirs {
		m.logger.Info("Removing data directory: " + dir)
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove data dir %s failed: %w", dir, err)
		}
	}

	if _, err := utils.BuildWorkDirs(cfg.BaseDir); err != nil {
		return fmt.Errorf("recreate work dirs failed: %w", err)
	}

	m.logger.Success("Data directories reset")
	return nil
}
