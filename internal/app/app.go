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
	defer a.docker.Close()

	a.ui.BindActions(
		a.handleSetup,
		a.handleLoadTest,
		a.handleStatus,
		a.handleDown,
		a.handleReset,
		a.handleCleanup,
	)

	a.logger.Info("Welcome to my-replica")
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

		a.logger.Info("Stopping and removing master/slave containers...")
		if err := a.docker.Down(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Down failed: %v", err))
			return
		}
	}()
}

func (a *App) handleReset() {
	go func() {
		cfg := a.ui.CurrentConfig()
		if cfg == nil {
			a.logger.Warn("No active configuration found. Run setup first.")
			return
		}

		if !a.ui.ShowConfirmDialog("Reset", "Reset will stop containers, delete data directories, and run setup again.\n\nContinue?") {
			a.logger.Warn("Reset cancelled")
			return
		}

		a.logger.Info("Resetting environment...")
		if err := a.docker.Down(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Reset down failed: %v", err))
			return
		}

		if err := a.docker.ResetDataDirs(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Reset data dirs failed: %v", err))
			return
		}

		if err := a.docker.PrepareEnvironment(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Reset prepare failed: %v", err))
			return
		}

		if err := a.replicator.Setup(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Reset setup failed: %v", err))
			return
		}

		a.logger.Success("Environment reset and replication reconfigured")
	}()
}

func (a *App) handleCleanup() {
	go func() {
		cfg := a.ui.CurrentConfig()
		if cfg == nil {
			a.logger.Warn("No active configuration found. Run setup first.")
			return
		}

		if !a.ui.ShowConfirmDialog("Cleanup", "Cleanup will stop containers, remove the Docker network, and delete dump files.\n\nData directories are kept unless you run reset.\n\nContinue?") {
			a.logger.Warn("Cleanup cancelled")
			return
		}

		a.logger.Info("Running cleanup...")
		if err := a.docker.Cleanup(cfg); err != nil {
			a.logger.Error(fmt.Sprintf("Cleanup failed: %v", err))
			return
		}
	}()
}

func (a *App) handleSetup() {
	go func() {
		cfg, err := a.ui.ShowSetupForm()
		if err != nil {
			a.logger.Warn("Setup cancelled")
			return
		}

		a.logger.Info("Validating Docker daemon access...")
		if err := a.docker.ValidateDockerAvailable(); err != nil {
			a.logger.Error(fmt.Sprintf("Docker unavailable: %v", err))
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

		a.logger.Warn("Load test can increase CPU, disk I/O, binlog volume, and replica lag.")
		a.logger.Warn("Use only on non-production environments.")

		if !a.ui.ShowConfirmDialog("Load Test", "Run a safe replication load test?\n\nOnly table replication_bench is used.\n\nContinue?") {
			a.logger.Warn("Load test cancelled")
			return
		}

		testCfg, ok := a.ui.ShowLoadTestForm()
		if !ok {
			a.logger.Warn("Load test cancelled")
			return
		}

		a.logger.Info("Starting safe load test on master...")
		a.logger.Info("Benchmark table: replication_bench")

		_, err := a.tester.Run(cfg, tester.LoadTestConfig{
			Workers:       testCfg.Workers,
			RequestsPerW:  testCfg.RequestsPerW,
			PayloadSize:   testCfg.PayloadSize,
			Cleanup:       testCfg.Cleanup,
			ReportEvery:   2 * time.Second,
			InsertTimeout: 5 * time.Second,
		})
		if err != nil {
			a.logger.Error(fmt.Sprintf("Load test failed: %v", err))
			return
		}

		a.logger.Success("Load test completed")
	}()
}
