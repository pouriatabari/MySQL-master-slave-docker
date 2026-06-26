package docker_mgr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/pouriatabari/my-replica/internal/ui"
)

func mysqlImage(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "mysql:8.0"
	}
	if strings.Contains(v, ":") {
		return v
	}
	return "mysql:" + v
}

func (m *Manager) ensureImage(imageName string) error {
	m.logger.Info("Pulling image: " + imageName)

	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	defer cancel()

	reader, err := m.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("image pull failed: %w", err)
	}
	defer reader.Close()

	_, _ = io.Copy(io.Discard, reader)

	m.logger.Success("Image ready: " + imageName)
	return nil
}

func (m *Manager) removeIfExists(name string) {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	_, err := m.cli.ContainerInspect(ctx, name)
	if err == nil {
		m.logger.Warn("Container already exists, removing: " + name)
		_ = m.cli.ContainerRemove(ctx, name, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
	}
}

func (m *Manager) waitForMySQL(containerName string, cfg *ui.SetupConfig, isMaster bool) error {
	hostPort := cfg.SlavePort
	if isMaster {
		hostPort = cfg.MasterPort
	}

	m.logger.Info(fmt.Sprintf("Waiting for MySQL readiness on %s:%d ...", containerName, hostPort))

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		out, err := m.execWithContext(ctx, containerName, []string{
			"sh", "-c",
			fmt.Sprintf("mysqladmin ping -uroot -p'%s' --silent", cfg.RootPassword),
		})
		cancel()

		if err == nil && strings.Contains(strings.ToLower(out), "mysqld is alive") {
			m.logger.Success("MySQL is ready in " + containerName)
			return nil
		}
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("timeout waiting for mysql readiness in container %s", containerName)
}

func (m *Manager) RunMaster(cfg *ui.SetupConfig) error {
	if err := m.ensureDockerClient(); err != nil {
		return err
	}

	imageName := mysqlImage(cfg.MySQLVersion)
	if err := m.ensureImage(imageName); err != nil {
		return err
	}

	m.removeIfExists(cfg.MasterContainer)

	port, err := nat.NewPort("tcp", "3306")
	if err != nil {
		return err
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			port: []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.MasterPort)},
			},
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: cfg.BaseDir + "/master/db_master",
				Target: "/var/lib/mysql",
			},
			{
				Type:   mount.TypeBind,
				Source: cfg.BaseDir + "/master/config",
				Target: "/etc/mysql/conf.d",
			},
		},
	}

	containerConfig := &container.Config{
		Image: imageName,
		Env: []string{
			"MYSQL_ROOT_PASSWORD=" + cfg.RootPassword,
			"MYSQL_DATABASE=" + cfg.DatabaseName,
		},
		ExposedPorts: nat.PortSet{
			port: struct{}{},
		},
	}

	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	defer cancel()

	resp, err := m.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		cfg.MasterContainer,
	)
	if err != nil {
		return fmt.Errorf("create master container failed: %w", err)
	}

	if err := m.cli.NetworkConnect(ctx, cfg.NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect master to network failed: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start master container failed: %w", err)
	}

	m.logger.Success(cfg.MasterContainer + " container started")
	return m.waitForMySQL(cfg.MasterContainer, cfg, true)
}

func (m *Manager) RunSlave(cfg *ui.SetupConfig) error {
	if err := m.ensureDockerClient(); err != nil {
		return err
	}

	imageName := mysqlImage(cfg.MySQLVersion)
	if err := m.ensureImage(imageName); err != nil {
		return err
	}

	m.removeIfExists(cfg.SlaveContainer)

	port, err := nat.NewPort("tcp", "3306")
	if err != nil {
		return err
	}

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			port: []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.SlavePort)},
			},
		},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: cfg.BaseDir + "/slave/db_slave",
				Target: "/var/lib/mysql",
			},
			{
				Type:   mount.TypeBind,
				Source: cfg.BaseDir + "/slave/config",
				Target: "/etc/mysql/conf.d",
			},
		},
	}

	containerConfig := &container.Config{
		Image: imageName,
		Env: []string{
			"MYSQL_ROOT_PASSWORD=" + cfg.RootPassword,
			"MYSQL_DATABASE=" + cfg.DatabaseName,
		},
		ExposedPorts: nat.PortSet{
			port: struct{}{},
		},
	}

	ctx, cancel := context.WithTimeout(m.ctx, 2*time.Minute)
	defer cancel()

	resp, err := m.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		cfg.SlaveContainer,
	)
	if err != nil {
		return fmt.Errorf("create slave container failed: %w", err)
	}

	if err := m.cli.NetworkConnect(ctx, cfg.NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect slave to network failed: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start slave container failed: %w", err)
	}

	m.logger.Success(cfg.SlaveContainer + " container started")
	return m.waitForMySQL(cfg.SlaveContainer, cfg, false)
}

func sanitizeCmd(cmd []string) string {
	joined := strings.Join(cmd, " ")
	replacements := []string{"-p'", "-p", "PASSWORD="}
	for _, marker := range replacements {
		if idx := strings.Index(joined, marker); idx >= 0 {
			start := idx + len(marker)
			end := start
			for end < len(joined) && joined[end] != '\'' && joined[end] != ' ' {
				end++
			}
			if end > start {
				joined = joined[:start] + "****" + joined[end:]
			}
		}
	}
	return joined
}

func (m *Manager) Exec(containerName string, cmd []string) (string, error) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	defer cancel()
	return m.execWithContext(ctx, containerName, cmd)
}

func (m *Manager) execWithContext(ctx context.Context, containerName string, cmd []string) (string, error) {
	m.logger.Info(fmt.Sprintf("Exec in %s: %s", containerName, sanitizeCmd(cmd)))

	execResp, err := m.cli.ContainerExecCreate(ctx, containerName, types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("exec create failed: %w", err)
	}

	attachResp, err := m.cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", fmt.Errorf("exec attach failed: %w", err)
	}
	defer attachResp.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	_, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader)
	if err != nil {
		return "", fmt.Errorf("exec output read failed: %w", err)
	}

	inspect, err := m.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return "", fmt.Errorf("exec inspect failed: %w", err)
	}

	out := stdoutBuf.String() + stderrBuf.String()

	if inspect.ExitCode != 0 {
		return out, fmt.Errorf("exec failed with exit code %d: %s", inspect.ExitCode, out)
	}

	return out, nil
}
