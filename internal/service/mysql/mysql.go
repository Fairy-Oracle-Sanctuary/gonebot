// Package mysql 封装 sqlx 实现 runtime.MySQLService 接口.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// Config MySQL 连接配置.
type Config struct {
	DSN     string
	MaxOpen int
	MaxIdle int
	MaxLife int // 秒
}

// Service 实现 runtime.MySQLService.
type Service struct {
	db     *sqlx.DB
	logger *slog.Logger
}

// New 创建 MySQL 服务并验证连接.
func New(cfg Config) (*Service, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("mysql DSN is required")
	}
	if cfg.MaxOpen <= 0 {
		cfg.MaxOpen = 20
	}
	if cfg.MaxIdle <= 0 {
		cfg.MaxIdle = 5
	}
	if cfg.MaxLife <= 0 {
		cfg.MaxLife = 1800 // 30 min
	}

	db, err := sqlx.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpen)
	db.SetMaxIdleConns(cfg.MaxIdle)
	db.SetConnMaxLifetime(time.Duration(cfg.MaxLife) * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}

	return &Service{
		db:     db,
		logger: slog.Default().With("module", "mysql"),
	}, nil
}

// Close 关闭连接池.
func (s *Service) Close() error {
	return s.db.Close()
}

// DB 返回底层 sqlx.DB (高级用法).
func (s *Service) DB() *sqlx.DB { return s.db }

// Query 执行查询, 返回 []map[string]any.
func (s *Service) Query(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := s.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// QueryOne 查询单行.
func (s *Service) QueryOne(ctx context.Context, query string, args ...any) (map[string]any, error) {
	rows, err := s.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query one: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil // 无结果
	}
	row := make(map[string]any)
	if err := rows.MapScan(row); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return row, rows.Err()
}

// Exec 执行写操作, 返回影响行数.
func (s *Service) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("exec: %w", err)
	}
	return result.RowsAffected()
}

var (
	_ = fmt.Sprintf
	_ = sql.ErrNoRows
)
