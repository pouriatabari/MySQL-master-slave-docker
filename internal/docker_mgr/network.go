package docker_mgr

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/pouriatabari/my-replica/internal/ui"
	"github.com/pouriatabari/my-replica/internal/utils"
)

func (m *Manager) CreateNetwork(name string) error {
	m.logger.Info("Checking docker network: " + name)

	args := filters.NewArgs()
	args.Add("name", name)

	nws, err := m.cli.NetworkList(m.ctx, network.ListOptions{
		Filters: args,
	})
	if err != nil {
		return fmt.Errorf("network list failed: %w", err)
	}

	if len(nws) > 0 {
		m.logger.Success("Docker network already exists: " + name)
		return nil
	}

	m.logger.Info("Creating docker network: " + name)
	_, err = m.cli.NetworkCreate(m.ctx, name, network.CreateOptions{})
	if err != nil {
		return fmt.Errorf("network create failed: %w", err)
	}

	m.logger.Success("Docker network created: " + name)
	return nil
}

func (m *Manager) renderTemplate(templatePath string, outPath string, data any) error {
	tmplBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}

	tmpl, err := template.New(filepath.Base(templatePath)).Parse(string(tmplBytes))
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}

func (m *Manager) PrepareEnvironment(cfg *ui.SetupConfig) error {
	if err := m.ensureDockerClient(); err != nil {
		return err
	}

	m.logger.Info("Preparing working directories...")
	dirs, err := utils.BuildWorkDirs(cfg.BaseDir)
	if err != nil {
		return fmt.Errorf("build work dirs failed: %w", err)
	}

	m.logger.Info("Rendering MySQL master config...")
	if err := m.renderTemplate(
		"templates/master.cnf.tmpl",
		filepath.Join(dirs["master_config"], "60-enable-replication.cnf"),
		cfg,
	); err != nil {
		return fmt.Errorf("render master template failed: %w", err)
	}

	m.logger.Info("Rendering MySQL slave config...")
	if err := m.renderTemplate(
		"templates/slave.cnf.tmpl",
		filepath.Join(dirs["slave_config"], "60-enable-replication.cnf"),
		cfg,
	); err != nil {
		return fmt.Errorf("render slave template failed: %w", err)
	}

	if err := m.CreateNetwork(cfg.NetworkName); err != nil {
		return err
	}

	m.logger.Success("Environment prepared")
	return nil
}
