// storage/storage.go
package storage

import (
	"database/sql"
	"log"
	"sync"

	_ "github.com/lib/pq"
)

type Storage interface {
	RecordStatus(databaseName, status string)
}

type PostgresStorage struct {
	db         *sql.DB
	lastStatus map[string]string
	mu         sync.Mutex
}

func NewPostgresStorage(connStr string) (*PostgresStorage, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	return &PostgresStorage{
		db:         db,
		lastStatus: make(map[string]string),
	}, nil
}

func (s *PostgresStorage) RecordStatus(databaseName, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastStatus[databaseName] == status {
		return // 状态未变化，跳过记录
	}

	_, err := s.db.Exec("INSERT INTO database_status_log (database_name, status) VALUES ($1, $2)",
		databaseName, status)
	if err != nil {
		log.Printf("记录状态失败: %v", err)
		return
	}

	s.lastStatus[databaseName] = status
}
