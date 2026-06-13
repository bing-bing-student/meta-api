// Package viewlog 实现文章浏览量打点的业务编排（新链路）。
//
// 职责：
//  1. EnsureArticleExists：MySQL 校验 articleId 是否真实存在
//  2. Increment：Redis HINCRBY + ZINCRBY 完成 +1
//
// MySQL 由后台 cron PersistViewCount 周期性把 ZSet 里的增量回写。
//
// 文件分布：
//
//	service.go —— Service 接口 + impl + DI 构造 + 文章存在性校验
//	counter.go —— 计数（Redis HINCRBY + ZINCRBY）
//	types.go   —— Outcome 结构与业务码
package viewlog

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	articleModel "meta-api/app/model/article"
)

// Service 浏览量打点服务接口。专供新链路（guard.Engine）调用 EnsureArticleExists / Increment。
type Service interface {
	// EnsureArticleExists 文章存在性校验。
	//
	// 返回 nil 表示存在，可继续 +1；返回 *Outcome 表示需要按对应 HTTP 状态返回。
	EnsureArticleExists(ctx context.Context, articleID string) *Outcome

	// Increment 执行计数 +1（HINCRBY + ZINCRBY，与原内部 increment 行为一致）。
	//
	// 对失败仅打日志，调用方无需感知错误（避免响应差异成为攻击信号）。
	Increment(ctx context.Context, articleID string)
}

// viewLogService 浏览量打点服务实现。
type viewLogService struct {
	logger       *zap.Logger
	redis        *redis.Client
	articleModel articleModel.Model
}

// NewService 构造打点服务实例。
func NewService(logger *zap.Logger, rdb *redis.Client, am articleModel.Model) Service {
	return &viewLogService{
		logger:       logger,
		redis:        rdb,
		articleModel: am,
	}
}

// EnsureArticleExists 校验 articleId 在 MySQL 中真实存在。
// 不存在 → 404；其它 DB 错误 → 500。
func (s *viewLogService) EnsureArticleExists(ctx context.Context, articleIDStr string) *Outcome {
	id, err := parseArticleID(articleIDStr)
	if err != nil {
		return &Outcome{HTTPStatus: 404, Code: codeNotFound, Message: "article not found"}
	}
	if _, err = s.articleModel.GetArticleDetailByID(ctx, id); err != nil {
		// 与现有代码一致：MySQL 未命中既可能是 sql.ErrNoRows，也可能 GORM 返回字符串 "record not found"
		if isNotFoundErr(err) {
			return &Outcome{HTTPStatus: 404, Code: codeNotFound, Message: "article not found"}
		}
		s.logger.Error("view-log article exists check failed",
			zap.String("article_id", articleIDStr), zap.Error(err))
		return &Outcome{HTTPStatus: 500, Code: codeInternalError, Message: "internal error"}
	}
	return nil
}

// parseArticleID 字符串雪花 ID → uint64，与现有 idutil.ParseID 保持同样的 0 拒绝语义。
// 这里独立实现避免对 idutil 包产生反向依赖（idutil 不依赖 viewlog）。
func parseArticleID(s string) (uint64, error) {
	var id uint64
	if s == "" {
		return 0, fmt.Errorf("empty id")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid id")
		}
		id = id*10 + uint64(ch-'0')
	}
	if id == 0 {
		return 0, fmt.Errorf("zero id")
	}
	return id, nil
}

// isNotFoundErr GORM v2 record not found 判定。
func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	// 与 service/article/user.go 中现有判定保持一致
	return strings.Contains(err.Error(), "record not found")
}
