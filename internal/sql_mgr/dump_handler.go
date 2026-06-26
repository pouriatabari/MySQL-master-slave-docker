package sql_mgr

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pouriatabari/my-replica/internal/ui"
)

func (r *Replicator) RunDumpWithFallback(cfg *ui.SetupConfig) error {
	dumpPath := filepath.Join(cfg.BaseDir, "master", "db_master", "data.sql")
	containerDump := "/var/lib/mysql/data.sql"

	r.logger.Info("Trying primary dump strategy with mysqldump...")

	primaryCmd := []string{
		"sh", "-c",
		fmt.Sprintf("mysqldump -uroot -p'%s' --databases %s > %s",
			cfg.RootPassword,
			cfg.DatabaseName,
			containerDump,
		),
	}

	_, err := r.docker.Exec(cfg.MasterContainer, primaryCmd)
	if err == nil {
		r.logger.Success("Primary mysqldump strategy succeeded")
		return nil
	}

	r.logger.Warn("Primary mysqldump failed, trying fallback dump strategy...")

	fallbackCmd := []string{
		"sh", "-c",
		fmt.Sprintf("mysqldump -uroot -p'%s' --single-transaction --set-gtid-purged=OFF --databases %s > %s",
			cfg.RootPassword,
			cfg.DatabaseName,
			containerDump,
		),
	}

	_, fbErr := r.docker.Exec(cfg.MasterContainer, fallbackCmd)
	if fbErr == nil {
		r.logger.Success("Fallback dump strategy succeeded")
		return nil
	}

	r.logger.Warn("Fallback mysqldump failed, trying host-stream dump strategy...")

	if err := os.MkdirAll(filepath.Dir(dumpPath), 0o755); err != nil {
		return fmt.Errorf("create dump dir failed: %w", err)
	}

	outFile, err := os.Create(dumpPath)
	if err != nil {
		return fmt.Errorf("create host dump file failed: %w", err)
	}
	defer outFile.Close()

	streamCmd := []string{
		"mysqldump",
		"-uroot", "-p" + cfg.RootPassword,
		"--single-transaction",
		"--set-gtid-purged=OFF",
		"--databases", cfg.DatabaseName,
	}

	if err := r.docker.ExecStream(cfg.MasterContainer, streamCmd, outFile); err != nil {
		return fmt.Errorf("all dump strategies failed: primary=%v fallback=%v stream=%w", err, fbErr, err)
	}

	r.logger.Success("Host-stream dump strategy succeeded")
	return nil
}
