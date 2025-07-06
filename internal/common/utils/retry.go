package utils

import (
	"context"
	"math/rand"
	"time"

	"meta-api/config"
)

// Operation 定义需要重试的操作类型
type Operation func() error

// WithBackoff 执行带指数退避的重试操作
func WithBackoff(ctx context.Context, cfg *config.RetryConfig, op Operation) error {
	retryCount := 0
	currentDelay := cfg.InitialDelay

	for {
		// 执行操作
		err := op()
		if err == nil {
			return nil // 操作成功
		}

		// 达到最大重试次数
		if retryCount >= cfg.MaxRetries {
			return err
		}

		// 计算下一次延迟（指数退避 + 抖动）
		delay := calculateDelay(cfg, currentDelay, retryCount)

		// 等待或中断
		select {
		case <-ctx.Done():
			return ctx.Err() // 上下文取消
		case <-time.After(delay): // 继续重试
		}

		// 更新状态
		currentDelay = min(2*currentDelay, cfg.MaxDelay)
		retryCount++
	}
}

// calculateDelay 计算带抖动的延迟时间
func calculateDelay(cfg *config.RetryConfig, baseDelay time.Duration, retryCount int) time.Duration {
	// 指数增长：2^retryCount * baseDelay
	exponential := time.Duration(1<<retryCount) * baseDelay

	// 应用抖动：± JitterFactor%
	jitter := 1 + cfg.JitterFactor*(2*rand.Float64()-1)
	delayed := float64(exponential) * jitter

	// 确保不超过最大延迟
	return time.Duration(min(delayed, float64(cfg.MaxDelay)))
}
