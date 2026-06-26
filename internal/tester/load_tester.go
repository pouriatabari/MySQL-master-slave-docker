package tester

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pouriatabari/my-replica/internal/sql_mgr"
	"github.com/pouriatabari/my-replica/internal/ui"
	"github.com/pouriatabari/my-replica/internal/utils"
)

type LoadTestConfig struct {
	Workers       int
	RequestsPerW  int
	PayloadSize   int
	Cleanup       bool
	ReportEvery   time.Duration
	InsertTimeout time.Duration
}

type LoadTestResult struct {
	Workers         int
	RequestsPerW    int
	TotalRequests   int
	InsertedRows    int64
	Duration        time.Duration
	RowsPerSecond   float64
	SlaveLagSeconds sql.NullInt64
	MasterRowCount  int64
	SlaveRowCount   int64
}

type LoadTester struct {
	logger *utils.UILogger
	mysql  *sql_mgr.MySQLClient
}

func NewLoadTester(logger *utils.UILogger) *LoadTester {
	return &LoadTester{
		logger: logger,
		mysql:  sql_mgr.NewMySQLClient(),
	}
}

func (lt *LoadTester) Run(cfg *ui.SetupConfig, tc LoadTestConfig) (*LoadTestResult, error) {
	if tc.Workers <= 0 {
		tc.Workers = 4
	}
	if tc.RequestsPerW <= 0 {
		tc.RequestsPerW = 100
	}
	if tc.PayloadSize <= 0 {
		tc.PayloadSize = 64
	}
	if tc.ReportEvery <= 0 {
		tc.ReportEvery = 2 * time.Second
	}
	if tc.InsertTimeout <= 0 {
		tc.InsertTimeout = 5 * time.Second
	}

	lt.logger.Info("Opening master database connection...")
	masterDB, err := lt.mysql.OpenMaster(cfg)
	if err != nil {
		return nil, fmt.Errorf("open master db failed: %w", err)
	}
	defer masterDB.Close()

	lt.logger.Info("Opening slave database connection...")
	slaveDB, err := lt.mysql.OpenSlave(cfg)
	if err != nil {
		return nil, fmt.Errorf("open slave db failed: %w", err)
	}
	defer slaveDB.Close()

	if err := lt.mysql.Ping(masterDB); err != nil {
		return nil, fmt.Errorf("ping master failed: %w", err)
	}
	if err := lt.mysql.Ping(slaveDB); err != nil {
		return nil, fmt.Errorf("ping slave failed: %w", err)
	}

	lt.logger.Info("Ensuring benchmark table exists on master...")
	if err := lt.mysql.EnsureBenchTable(masterDB); err != nil {
		return nil, fmt.Errorf("ensure bench table failed: %w", err)
	}

	total := tc.Workers * tc.RequestsPerW
	lt.logger.Info(fmt.Sprintf("Starting load test: workers=%d total_requests=%d payload_size=%d",
		tc.Workers, total, tc.PayloadSize))

	var inserted int64
	var wg sync.WaitGroup
	errCh := make(chan error, tc.Workers)

	start := time.Now()
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(tc.ReportEvery)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				cur := atomic.LoadInt64(&inserted)
				lt.logger.Info(fmt.Sprintf("Progress: inserted=%d/%d", cur, total))
			}
		}
	}()

	for w := 0; w < tc.Workers; w++ {
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			stmt, err := masterDB.Prepare(`
INSERT INTO replication_bench (worker_id, payload)
VALUES (?, ?)
`)
			if err != nil {
				errCh <- fmt.Errorf("worker %d prepare failed: %w", workerID, err)
				return
			}
			defer stmt.Close()

			payload := strings.Repeat("x", tc.PayloadSize)

			for i := 0; i < tc.RequestsPerW; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), tc.InsertTimeout)
				_, err := stmt.ExecContext(ctx, workerID, fmt.Sprintf("%s-%d-%d", payload, workerID, i))
				cancel()

				if err != nil {
					errCh <- fmt.Errorf("worker %d insert %d failed: %w", workerID, i, err)
					return
				}

				atomic.AddInt64(&inserted, 1)
			}
		}(w + 1)
	}

	wg.Wait()
	close(done)
	close(errCh)

	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}

	duration := time.Since(start)

	lt.logger.Info("Waiting a bit for replica to catch up...")
	time.Sleep(3 * time.Second)

	masterCount, err := lt.mysql.CountBenchRows(masterDB)
	if err != nil {
		return nil, fmt.Errorf("count master rows failed: %w", err)
	}

	slaveCount, err := lt.mysql.CountBenchRows(slaveDB)
	if err != nil {
		lt.logger.Warn(fmt.Sprintf("count slave rows failed: %v", err))
		slaveCount = -1
	}

	lag, err := lt.mysql.SecondsBehindReplica(slaveDB)
	if err != nil {
		lt.logger.Warn(fmt.Sprintf("read replica lag failed: %v", err))
		lag = sql.NullInt64{Valid: false}
	}

	res := &LoadTestResult{
		Workers:         tc.Workers,
		RequestsPerW:    tc.RequestsPerW,
		TotalRequests:   total,
		InsertedRows:    inserted,
		Duration:        duration,
		RowsPerSecond:   float64(inserted) / duration.Seconds(),
		SlaveLagSeconds: lag,
		MasterRowCount:  masterCount,
		SlaveRowCount:   slaveCount,
	}

	lt.logger.Success("Load test finished")
	lt.logger.Info(res.String())

	if tc.Cleanup {
		lt.logger.Info("Cleanup enabled, dropping benchmark table on master...")
		if err := lt.mysql.CleanupBenchTable(masterDB); err != nil {
			return res, fmt.Errorf("cleanup bench table failed: %w", err)
		}
		lt.logger.Success("Benchmark table dropped on master")
	}

	return res, nil
}

func (r *LoadTestResult) String() string {
	lag := "unknown"
	if r.SlaveLagSeconds.Valid {
		lag = fmt.Sprintf("%d sec", r.SlaveLagSeconds.Int64)
	}

	return fmt.Sprintf(
		"Load test result\n"+
			"----------------------\n"+
			"Workers         : %d\n"+
			"Req/Worker      : %d\n"+
			"Total Requests  : %d\n"+
			"Inserted Rows   : %d\n"+
			"Duration        : %s\n"+
			"Rows/Sec        : %.2f\n"+
			"Master Rows     : %d\n"+
			"Slave Rows      : %d\n"+
			"Replica Lag     : %s\n",
		r.Workers,
		r.RequestsPerW,
		r.TotalRequests,
		r.InsertedRows,
		r.Duration.String(),
		r.RowsPerSecond,
		r.MasterRowCount,
		r.SlaveRowCount,
		lag,
	)
}
