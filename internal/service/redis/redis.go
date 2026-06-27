// Package redis 封装 go-redis 实现 runtime.RedisService 接口.
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config Redis 连接配置.
type Config struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

// Service 实现 runtime.RedisService.
type Service struct {
	client *redis.Client
	logger *slog.Logger
}

// New 创建 Redis 服务并验证连接.
func New(cfg Config) (*Service, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis addr is required")
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 10
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &Service{
		client: client,
		logger: slog.Default().With("module", "redis"),
	}, nil
}

// Close 关闭连接池.
func (s *Service) Close() error {
	return s.client.Close()
}

// Client 返回底层客户端 (高级用法).
func (s *Service) Client() *redis.Client { return s.client }

// Get 获取字符串值.
func (s *Service) Get(ctx context.Context, key string) (string, error) {
	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Set 设置字符串值 (可选 TTL).
func (s *Service) Set(ctx context.Context, key, value string, ttlSec int) error {
	return s.client.Set(ctx, key, value, time.Duration(ttlSec)*time.Second).Err()
}

// Del 删除 key(s).
func (s *Service) Del(ctx context.Context, keys ...string) error {
	return s.client.Del(ctx, keys...).Err()
}

// Exists 检查 key 是否存在.
func (s *Service) Exists(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Incr 自增并返回新值.
func (s *Service) Incr(ctx context.Context, key string) (int64, error) {
	return s.client.Incr(ctx, key).Result()
}

// HGet 获取 Hash 字段.
func (s *Service) HGet(ctx context.Context, key, field string) (string, error) {
	val, err := s.client.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// HSet 设置 Hash 字段.
func (s *Service) HSet(ctx context.Context, key string, values map[string]any) error {
	return s.client.HSet(ctx, key, values).Err()
}

// HGetAll 获取整个 Hash.
func (s *Service) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return s.client.HGetAll(ctx, key).Result()
}

// LPush 左侧推入列表.
func (s *Service) LPush(ctx context.Context, key string, values ...any) error {
	return s.client.LPush(ctx, key, values...).Err()
}

// RPush 右侧推入列表.
func (s *Service) RPush(ctx context.Context, key string, values ...any) error {
	return s.client.RPush(ctx, key, values...).Err()
}

// LPop 左侧弹出列表.
func (s *Service) LPop(ctx context.Context, key string) (string, error) {
	val, err := s.client.LPop(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// LRange 获取列表范围.
func (s *Service) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return s.client.LRange(ctx, key, start, stop).Result()
}

// LLen 获取列表长度.
func (s *Service) LLen(ctx context.Context, key string) (int64, error) {
	return s.client.LLen(ctx, key).Result()
}

var _ = fmt.Sprintf
