// checker/pg.go
package checker

import (
	"ProbeWorker/storage"
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type PostgresChecker struct {
	host     string
	port     int
	user     string
	password string
	dbname   string
	storage  storage.Storage
}

func NewPostgresCheckerWithStorage(host string, port int, user, password, dbname string, storage storage.Storage) *PostgresChecker {
	return &PostgresChecker{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		dbname:   dbname,
		storage:  storage,
	}
}

func (p *PostgresChecker) Name() string {
	return fmt.Sprintf("%s_postgresql_%s", p.host, p.dbname)
}

func (p *PostgresChecker) Check(ctx context.Context) error {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.host, p.port, p.user, p.password, p.dbname)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		p.storage.RecordStatus(p.Name(), "abnormal")
		return fmt.Errorf("无法打开数据库连接: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		p.storage.RecordStatus(p.Name(), "abnormal")
		return fmt.Errorf("数据库不可达: %v", err)
	}

	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		p.storage.RecordStatus(p.Name(), "abnormal")
		return fmt.Errorf("查询失败: %v", err)
	}

	p.storage.RecordStatus(p.Name(), "normal")
	fmt.Printf("[%s] 连接成功，查询结果: %d\n", p.Name(), result)
	return nil
}
