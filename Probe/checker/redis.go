// checker/redis.go
package checker

import (
	"ProbeWorker/storage"
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
)

type RedisChecker struct {
	host     string
	port     int
	password string
	storage  storage.Storage
}

func NewRedisCheckerWithStorage(host string, port int, password string, storage storage.Storage) *RedisChecker {
	return &RedisChecker{
		host:     host,
		port:     port,
		password: password,
		storage:  storage,
	}
}

func (r *RedisChecker) Name() string {
	return fmt.Sprintf("%s_redis_db", r.host)
}

func (r *RedisChecker) Check(ctx context.Context) error {
	// 构建 Redis 连接选项
	options := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", r.host, r.port),
		Password: r.password, // 密码可以为空
	}

	client := redis.NewClient(options)
	defer client.Close()

	// 测试连接
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		r.storage.RecordStatus(r.Name(), "abnormal")
		return fmt.Errorf("Redis 连接失败: %v", err)
	}

	// 执行简单命令测试
	_, err = client.Do(ctx, "GET", "health_check").Result()
	if err != nil && err != redis.Nil {
		r.storage.RecordStatus(r.Name(), "abnormal")
		return fmt.Errorf("Redis 命令执行失败: %v", err)
	}

	r.storage.RecordStatus(r.Name(), "normal")
	fmt.Printf("[%s] 连接成功，PING 结果: %s\n", r.Name(), pong)
	return nil
}
