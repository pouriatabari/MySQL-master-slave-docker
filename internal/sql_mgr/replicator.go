package sql_mgr

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pouriatabari/my-replica/internal/docker_mgr"
	"github.com/pouriatabari/my-replica/internal/ui"
	"github.com/pouriatabari/my-replica/internal/utils"
)

type Replicator struct {
	logger *utils.UILogger
	docker *docker_mgr.Manager
}

func NewReplicator(logger *utils.UILogger, docker *docker_mgr.Manager) *Replicator {
	return &Replicator{
		logger: logger,
		docker: docker,
	}
}

func (r *Replicator) Setup(cfg *ui.SetupConfig) error {
	r.logger.Info("Starting replication setup...")

	if err := r.docker.RunMaster(cfg); err != nil {
		return fmt.Errorf("run master failed: %w", err)
	}

	r.logger.Info("Creating replication user on master...")
	createUserSQL := fmt.Sprintf(`
CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED WITH mysql_native_password BY '%s';
GRANT REPLICATION SLAVE ON *.* TO '%s'@'%%';
FLUSH PRIVILEGES;
`, cfg.ReplUser, cfg.ReplPassword, cfg.ReplUser)

	_, err := r.docker.Exec(cfg.MasterContainer, []string{
		"sh", "-c",
		fmt.Sprintf(`mysql -uroot -p'%s' -e "%s"`, cfg.RootPassword, oneLineSQL(createUserSQL)),
	})
	if err != nil {
		return fmt.Errorf("create replication user failed: %w", err)
	}

	r.logger.Success("Replication user created on master")

	r.logger.Info("Reading master binlog position before dump...")
	out, err := r.docker.Exec(cfg.MasterContainer, []string{
		"sh", "-c",
		fmt.Sprintf(`mysql -uroot -p'%s' -e "SHOW MASTER STATUS\G"`, cfg.RootPassword),
	})
	if err != nil {
		return fmt.Errorf("show master status failed: %w", err)
	}

	logFile, logPos, err := parseMasterStatus(out)
	if err != nil {
		return fmt.Errorf("parse master status failed: %w", err)
	}

	r.logger.Success(fmt.Sprintf("Master status captured: file=%s pos=%d", logFile, logPos))

	if err := r.RunDumpWithFallback(cfg); err != nil {
		return err
	}

	masterDumpPath := filepath.Join(cfg.BaseDir, "master", "db_master", "data.sql")
	slaveDumpPath := filepath.Join(cfg.BaseDir, "slave", "db_slave", "data.sql")

	r.logger.Info("Copying dump file from master volume to slave volume...")
	if err := utils.CopyFile(masterDumpPath, slaveDumpPath); err != nil {
		return fmt.Errorf("copy dump to slave failed: %w", err)
	}
	r.logger.Success("Dump copied to slave volume")

	if err := r.docker.RunSlave(cfg); err != nil {
		return fmt.Errorf("run slave failed: %w", err)
	}

	r.logger.Info("Importing dump into slave with mysql client...")
	_, err = r.docker.Exec(cfg.SlaveContainer, []string{
		"sh", "-c",
		fmt.Sprintf(`mysql -uroot -p'%s' %s < /var/lib/mysql/data.sql`, cfg.RootPassword, cfg.DatabaseName),
	})
	if err != nil {
		return fmt.Errorf("import dump into slave failed: %w", err)
	}

	r.logger.Success("Dump imported into slave")

	r.logger.Info("Configuring replication on slave...")
	if err := r.configureReplication(cfg, logFile, logPos); err != nil {
		return err
	}

	r.logger.Info("Waiting for replica threads to stabilize...")
	time.Sleep(5 * time.Second)

	status, err := r.Status(cfg)
	if err != nil {
		return fmt.Errorf("replica status validation failed: %w", err)
	}

	r.logger.Info(status)
	r.logger.Success("Replication setup finished successfully")
	return nil
}

func (r *Replicator) configureReplication(cfg *ui.SetupConfig, logFile string, logPos int) error {
	modernSQL := fmt.Sprintf(`
STOP REPLICA;
RESET REPLICA ALL;
CHANGE REPLICATION SOURCE TO
  SOURCE_HOST='%s',
  SOURCE_USER='%s',
  SOURCE_PASSWORD='%s',
  SOURCE_LOG_FILE='%s',
  SOURCE_LOG_POS=%d,
  GET_SOURCE_PUBLIC_KEY=1;
START REPLICA;
`, cfg.MasterContainer, cfg.ReplUser, cfg.ReplPassword, logFile, logPos)

	_, err := r.docker.Exec(cfg.SlaveContainer, []string{
		"sh", "-c",
		fmt.Sprintf(`mysql -uroot -p'%s' -e "%s"`, cfg.RootPassword, oneLineSQL(modernSQL)),
	})
	if err == nil {
		r.logger.Success("Replication configured with modern syntax (CHANGE REPLICATION SOURCE TO)")
		return nil
	}

	r.logger.Warn("Modern replication syntax failed, trying legacy CHANGE MASTER TO...")

	legacySQL := fmt.Sprintf(`
STOP SLAVE;
RESET SLAVE ALL;
CHANGE MASTER TO
  MASTER_HOST='%s',
  MASTER_USER='%s',
  MASTER_PASSWORD='%s',
  MASTER_LOG_FILE='%s',
  MASTER_LOG_POS=%d;
START SLAVE;
`, cfg.MasterContainer, cfg.ReplUser, cfg.ReplPassword, logFile, logPos)

	_, legacyErr := r.docker.Exec(cfg.SlaveContainer, []string{
		"sh", "-c",
		fmt.Sprintf(`mysql -uroot -p'%s' -e "%s"`, cfg.RootPassword, oneLineSQL(legacySQL)),
	})
	if legacyErr != nil {
		return fmt.Errorf("configure replication failed: modern=%v legacy=%v", err, legacyErr)
	}

	r.logger.Success("Replication configured with legacy syntax (CHANGE MASTER TO)")
	return nil
}

func (r *Replicator) Status(cfg *ui.SetupConfig) (string, error) {
	r.logger.Info("Checking replica status...")

	out, err := r.docker.Exec(cfg.SlaveContainer, []string{
		"sh", "-c",
		fmt.Sprintf(`mysql -uroot -p'%s' -e "SHOW REPLICA STATUS\G"`, cfg.RootPassword),
	})
	if err != nil {
		r.logger.Warn("SHOW REPLICA STATUS failed, trying SHOW SLAVE STATUS")
		out, err = r.docker.Exec(cfg.SlaveContainer, []string{
			"sh", "-c",
			fmt.Sprintf(`mysql -uroot -p'%s' -e "SHOW SLAVE STATUS\G"`, cfg.RootPassword),
		})
		if err != nil {
			return "", fmt.Errorf("replica/slave status command failed: %w", err)
		}
	}

	ioRunning := extractField(out, []string{"Replica_IO_Running", "Slave_IO_Running"})
	sqlRunning := extractField(out, []string{"Replica_SQL_Running", "Slave_SQL_Running"})
	lag := extractField(out, []string{"Seconds_Behind_Source", "Seconds_Behind_Master"})
	sourceHost := extractField(out, []string{"Source_Host", "Master_Host"})
	sourceLogFile := extractField(out, []string{"Source_Log_File", "Master_Log_File"})
	sourceLogPos := extractField(out, []string{"Read_Source_Log_Pos", "Exec_Master_Log_Pos", "Read_Master_Log_Pos"})
	lastIOErr := extractField(out, []string{"Last_IO_Error"})
	lastSQLErr := extractField(out, []string{"Last_SQL_Error"})

	var b strings.Builder
	b.WriteString("Replica status\n")
	b.WriteString("----------------------\n")
	b.WriteString(fmt.Sprintf("Source Host : %s\n", emptyAsUnknown(sourceHost)))
	b.WriteString(fmt.Sprintf("IO Running  : %s\n", emptyAsUnknown(ioRunning)))
	b.WriteString(fmt.Sprintf("SQL Running : %s\n", emptyAsUnknown(sqlRunning)))
	b.WriteString(fmt.Sprintf("Lag (sec)   : %s\n", emptyAsUnknown(lag)))
	b.WriteString(fmt.Sprintf("Source Log  : %s\n", emptyAsUnknown(sourceLogFile)))
	b.WriteString(fmt.Sprintf("Position    : %s\n", emptyAsUnknown(sourceLogPos)))
	if strings.TrimSpace(lastIOErr) != "" {
		b.WriteString(fmt.Sprintf("Last IO Err : %s\n", lastIOErr))
	}
	if strings.TrimSpace(lastSQLErr) != "" {
		b.WriteString(fmt.Sprintf("Last SQL Err: %s\n", lastSQLErr))
	}

	return b.String(), nil
}

func parseMasterStatus(out string) (string, int, error) {
	file := extractField(out, []string{"File"})
	posStr := extractField(out, []string{"Position"})

	if file == "" || posStr == "" {
		return "", 0, fmt.Errorf("master status missing File or Position: %s", out)
	}

	pos, err := strconv.Atoi(strings.TrimSpace(posStr))
	if err != nil {
		return "", 0, fmt.Errorf("invalid position %q: %w", posStr, err)
	}

	return strings.TrimSpace(file), pos, nil
}

func extractField(out string, keys []string) string {
	for _, key := range keys {
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `:\s*(.*)$`)
		matches := re.FindStringSubmatch(out)
		if len(matches) == 2 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func oneLineSQL(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func emptyAsUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "Unknown"
	}
	return s
}
