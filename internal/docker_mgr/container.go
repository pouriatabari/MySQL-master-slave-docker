package docker_mgr

import (
	"bytes"
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

func (m *Manager) ensureImage(imageName string) error {
	m.logger.Info("Pulling image: " + imageName)

	reader, err := m.cli.ImagePull(m.ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("image pull failed: %w", err)
	}
	defer reader.Close()

	_, _ = io.Copy(io.Discard, reader)

	m.logger.Success("Image ready: " + imageName)
	return nil
}

func (m *Manager) removeIfExists(name string) {
	_, err := m.cli.ContainerInspect(m.ctx, name)
	if err == nil {
		m.logger.Warn("Container already exists, removing: " + name)
		_ = m.cli.ContainerRemove(m.ctx, name, container.RemoveOptions{
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

	timeout := time.After(90 * time.Second)
	tick := time.Tick(3 * time.Second)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for mysql readiness in container %s", containerName)
		case <-tick:
			out, err := m.Exec(containerName, []string{
				"sh", "-c",
				fmt.Sprintf("mysqladmin ping -uroot -p'%s' --silent", cfg.RootPassword),
			})
			if err == nil && strings.Contains(strings.ToLower(out), "mysqld is alive") {
				m.logger.Success("MySQL is ready in " + containerName)
				return nil
			}
		}
	}
}

func (m *Manager) RunMaster(cfg *ui.SetupConfig) error {
	imageName := "mysql:" + cfg.MySQLVersion
	if err := m.ensureImage(imageName); err != nil {
		return err
	}

	m.removeIfExists("mysql-master")

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

	networkingConfig := &container.NetworkingConfig{}

	resp, err := m.cli.ContainerCreate(
		m.ctx,
		containerConfig,
		hostConfig,
		networkingConfig,
		nil,
		"mysql-master",
	)
	if err != nil {
		return fmt.Errorf("create master container failed: %w", err)
	}

	if err := m.cli.NetworkConnect(m.ctx, cfg.NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect master to network failed: %w", err)
	}

	if err := m.cli.ContainerStart(m.ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start master container failed: %w", err)
	}

	m.logger.Success("mysql-master container started")
	return m.waitForMySQL("mysql-master", cfg, true)
}

func (m *Manager) RunSlave(cfg *ui.SetupConfig) error {
	imageName := "mysql:" + cfg.MySQLVersion
	if err := m.ensureImage(imageName); err != nil {
		return err
	}

	m.removeIfExists("mysql-slave")

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

	resp, err := m.cli.ContainerCreate(
		m.ctx,
		containerConfig,
		hostConfig,
		nil,
		nil,
		"mysql-slave",
	)
	if err != nil {
		return fmt.Errorf("create slave container failed: %w", err)
	}

	if err := m.cli.NetworkConnect(m.ctx, cfg.NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect slave to network failed: %w", err)
	}

	if err := m.cli.ContainerStart(m.ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start slave container failed: %w", err)
	}

	m.logger.Success("mysql-slave container started")
	return m.waitForMySQL("mysql-slave", cfg, false)
}

func (m *Manager) Exec(containerName string, cmd []string) (string, error) {
	m.logger.Info(fmt.Sprintf("Exec in %s: %s", containerName, strings.Join(cmd, " ")))

	execResp, err := m.cli.ContainerExecCreate(m.ctx, containerName, types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("exec create failed: %w", err)
	}

	attachResp, err := m.cli.ContainerExecAttach(m.ctx, execResp.ID, types.ExecStartCheck{})
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

	inspect, err := m.cli.ContainerExecInspect(m.ctx, execResp.ID)
	if err != nil {
		return "", fmt.Errorf("exec inspect failed: %w", err)
	}

	out := stdoutBuf.String() + stderrBuf.String()

	if inspect.ExitCode != 0 {
		return out, fmt.Errorf("exec failed with exit code %d: %s", inspect.ExitCode, out)
	}

	return out, nil
}
