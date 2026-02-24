package checker

import (
	"context"
)

// DatabaseChecker 定义数据库检查的通用接口
type DatabaseChecker interface {
	Check(ctx context.Context) error
	Name() string
}
