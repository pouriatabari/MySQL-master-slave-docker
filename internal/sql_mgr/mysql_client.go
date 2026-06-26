package sql_mgr

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pouriatabari/my-replica/internal/ui"
)

type MySQLClient struct{}

func NewMySQLClient() *MySQLClient {
	return &MySQLClient{}
}

func (c *MySQLClient) OpenMaster(cfg *ui.SetupConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("root:%s@tcp(127.0.0.1:%d)/%s?parseTime=true&multiStatements=true",
		cfg.RootPassword,
		cfg.MasterPort,
		cfg.DatabaseName,
	)
	return sql.Open("mysql", dsn)
}

func (c *MySQLClient) OpenSlave(cfg *ui.SetupConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("root:%s@tcp(127.0.0.1:%d)/%s?parseTime=true&multiStatements=true",
		cfg.RootPassword,
		cfg.SlavePort,
		cfg.DatabaseName,
	)
	return sql.Open("mysql", dsn)
}

func (c *MySQLClient) Ping(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func (c *MySQLClient) EnsureBenchTable(db *sql.DB) error {
	query := `
CREATE TABLE IF NOT EXISTS replication_bench (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    worker_id INT NOT NULL,
    payload VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, query)
	return err
}

func (c *MySQLClient) CleanupBenchTable(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS replication_bench`)
	return err
}

func (c *MySQLClient) CountBenchRows(db *sql.DB) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM replication_bench`).Scan(&count)
	return count, err
}

func (c *MySQLClient) SecondsBehindReplica(db *sql.DB) (sql.NullInt64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `SHOW REPLICA STATUS`)
	if err != nil {
		rows, err = db.QueryContext(ctx, `SHOW SLAVE STATUS`)
		if err != nil {
			return sql.NullInt64{}, err
		}
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return sql.NullInt64{}, err
	}

	if !rows.Next() {
		return sql.NullInt64{}, fmt.Errorf("no replica status row returned")
	}

	raw := make([]any, len(cols))
	rawPtrs := make([]any, len(cols))
	for i := range raw {
		rawPtrs[i] = &raw[i]
	}

	if err := rows.Scan(rawPtrs...); err != nil {
		return sql.NullInt64{}, err
	}

	for i, col := range cols {
		if col == "Seconds_Behind_Source" || col == "Seconds_Behind_Master" {
			val := raw[i]
			switch v := val.(type) {
			case nil:
				return sql.NullInt64{Valid: false}, nil
			case []byte:
				var n int64
				_, err := fmt.Sscanf(string(v), "%d", &n)
				if err != nil {
					return sql.NullInt64{}, err
				}
				return sql.NullInt64{Int64: n, Valid: true}, nil
			case int64:
				return sql.NullInt64{Int64: v, Valid: true}, nil
			}
		}
	}

	return sql.NullInt64{Valid: false}, nil
}
