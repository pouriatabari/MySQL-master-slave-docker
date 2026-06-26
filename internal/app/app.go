package app

import (
	"fmt"
	"time"

	"github.com/pouriatabari/my-replica/internal/docker_mgr"
	"github.com/pouriatabari/my-replica/internal/sql_mgr"
	"github.com/pouriatabari/my-replica/internal/tester"
	"github.com/pouriatabari/my-replica/internal/ui"
	"github.com/pouriatabari/my-replica/internal/utils"
)

type App struct {
	logger     *utils.UILogger
	ui         *ui.UI
	docker     *docker_mgr.Manager
	replicator *sql_mgr.Replicator
	tester     *tester.LoadTester
}

func New() *App {
	logger := utils.NewUILogger()
	userInterface := ui.NewUI(logger)
	docker := docker_mgr.NewManager(logger)
	replicator := sql_mgr.NewReplicator(logger, docker)
	loadTester := tester.NewLoadTester(logger)

	return &App{
		logger:     logger,
		ui:         userInterface,
		docker:     docker,
		replicator: replicator,
		tester:     loadTester,
	}
}

func (a *App) Run() error {
	a.ui.BindActions(
		a.handleSetup,
		a.handleLoadTest,
		a.handleStatus,
		a.handleDown,
		a.handleReset,
		a.handleCleanup,
	)

	a.logger.Info("Welcome to My Replica")
	a.logger.Info("Press 's' to start interactive setup")

	return a.ui.Run()
}

func (a *App) handleDown() {
	go func() {
		cfg := a.ui.CurrentConfig()
		if cfg == nil {
			a.logger.Warn("No active configuration found. Run setup first.")
			return
		}

		a.logger.Info("Down command is not implemented yet")
	}()
}

func (a *App) handleReset() {
	go func() {
		cfg := a.ui.CurrentConfig()
		if cfg == nil {
			a.logger.Warn("No active configuration found. Run setup first.")
			return
		}

		a.logger.Info("Reset command is not implemented yet")
	}()
}

func (a *App) handleCleanup() {
	go func() {
		cfg := a.ui.CurrentConfig()
		if cfg == nil {
			a.logger.Warn("No active configuration found. Run setup first.")
			return
		}

		a.logger.Info("Cleanup command is not implemented yet")
	}()
}

func (a *App) handleSetup() {
	go func() {
		cfg, err := a.ui.ShowSetupForm()
		if err != nil {
			a.logger.Warn("Setup cancelled")
			return
		}

		a.logger.Info("Preparing docker environment...")
		if err := a.docker.PrepareEnvironment(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Prepare environment failed: %v", err))
			return
		}

		if err := a.replicator.Setup(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Replication setup failed: %v", err))
			return
		}

		a.logger.Success("Master/Replica environment is ready")
	}()
}

func (a *App) handleStatus() {
	go func() {
		cfg := a.ui.CurrentConfig()
		if cfg == nil {
			a.logger.Warn("No active configuration found. Run setup first.")
			return
		}

		status, err := a.replicator.Status(cfg)
		if err != nil {
			a.logger.Error(fmt.Sprintf("Status failed: %v", err))
			return
		}

		a.logger.Info(status)
	}()
}

func (a *App) handleLoadTest() {
	go func() {
		cfg := a.ui.CurrentConfig()
		if cfg == nil {
			a.logger.Warn("No active configuration found. Run setup first.")
			return
		}

		testCfg := tester.LoadTestConfig{
			Workers:       4,
			RequestsPerW:  250,
			PayloadSize:   128,
			Cleanup:       false,
			ReportEvery:   2 * time.Second,
			InsertTimeout: 5 * time.Second,
		}

		a.logger.Info("Starting safe load test on master...")
		a.logger.Info("Benchmark table: replication_bench")

		_, err := a.tester.Run(cfg, testCfg)
		if err != nil {
			a.logger.Error(fmt.Sprintf("Load test failed: %v", err))
			return
		}

		a.logger.Success("Load test completed")
	}()
}
