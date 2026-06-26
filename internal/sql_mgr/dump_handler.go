package sql_mgr

import (
	"fmt"

	"github.com/pouriatabari/my-replica/internal/ui"
)

func (r *Replicator) RunDumpWithFallback(cfg *ui.SetupConfig) error {
	r.logger.Info("Trying primary dump strategy with mysqldump...")

	primaryCmd := []string{
		"sh", "-c",
		fmt.Sprintf("mysqldump -uroot -p'%s' --databases %s > /var/lib/mysql/data.sql",
			cfg.RootPassword,
			cfg.DatabaseName,
		),
	}

	_, err := r.docker.Exec("mysql-master", primaryCmd)
	if err == nil {
		r.logger.Success("Primary mysqldump strategy succeeded")
		return nil
	}

	r.logger.Warn("Primary mysqldump failed, trying fallback dump strategy...")

	fallbackCmd := []string{
		"sh", "-c",
		fmt.Sprintf("mysqldump -uroot -p'%s' --single-transaction --set-gtid-purged=OFF --databases %s > /var/lib/mysql/data.sql",
			cfg.RootPassword,
			cfg.DatabaseName,
		),
	}

	_, fbErr := r.docker.Exec("mysql-master", fallbackCmd)
	if fbErr != nil {
		return fmt.Errorf("both primary and fallback dump strategies failed: primary=%v fallback=%v", err, fbErr)
	}

	r.logger.Success("Fallback dump strategy succeeded")
	return nil
}
