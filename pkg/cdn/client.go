package cdn

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	teo "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/teo/v20220901"
	"go.uber.org/zap"

	"meta-api/common/utils"
)

// 默认配置常量。
const (
	// defaultTimeout 调 EdgeOne 单次 API 的超时。
	// EdgeOne 接口通常 1s 内返回；留 5s 应对偶发链路抖动。
	defaultTimeout = 5 * time.Second

	// defaultPurgeWaitTimeout 等待 EdgeOne purge 任务到达终态的最长时间。
	defaultPurgeWaitTimeout = 30 * time.Second

	// defaultPurgePollInterval 查询 EdgeOne purge 任务状态的间隔。
	defaultPurgePollInterval = time.Second

	// sdkEndpoint EdgeOne 公网接入域名。
	sdkEndpoint = "teo.tencentcloudapi.com"

	// envSecretID EdgeOne API SecretId 所在环境变量名。
	envSecretID = "EDGEONE_SECRET_ID"

	// envSecretKey EdgeOne API SecretKey 所在环境变量名（敏感，建议走 docker secret 注入）。
	envSecretKey = "EDGEONE_SECRET_KEY"

	// envZoneID EdgeOne 站点 ID 所在环境变量名（如 zone-xxxx）。
	envZoneID = "EDGEONE_ZONE_ID"

	// envPurgeDomain 用于拼接清理 URL 的站点域名前缀，
	// 例如 https://liubing.xyz；末尾斜杠由代码统一裁剪。
	envPurgeDomain = "EDGEONE_PURGE_DOMAIN"
)

// Client 用来调用 CDN 清缓存任务接口。
//
// 实例由 DI 容器构造，单例复用 SDK 内部 http 连接池。
// 当任一必备 env 缺失或 SDK 初始化失败，所有调用立即返回，不发起任何 API 请求。
type Client struct {
	purger       purgeAPI
	zoneID       string
	domain       string
	waitTimeout  time.Duration
	pollInterval time.Duration
	logger       *zap.Logger
	ctx          context.Context
}

// New 构造一个 CDN 清缓存客户端。
func New(logger *zap.Logger, ctx context.Context) *Client {
	secretID, err := utils.EnvOrFile(envSecretID)
	if err != nil {
		logger.Warn("cdn disabled: secret id file read failed", zap.Error(err))
		return newDisabledClient(logger, ctx)
	}
	secretKey, err := utils.EnvOrFile(envSecretKey)
	if err != nil {
		logger.Warn("cdn disabled: secret file read failed", zap.Error(err))
		return newDisabledClient(logger, ctx)
	}
	zoneID := os.Getenv(envZoneID)
	domain := strings.TrimRight(os.Getenv(envPurgeDomain), "/")

	if secretID == "" || secretKey == "" || zoneID == "" || domain == "" {
		logger.Warn("cdn disabled: required env missing",
			zap.Bool("secret_id_loaded", secretID != ""),
			zap.Bool("secret_key_loaded", secretKey != ""),
			zap.Bool("zone_id_loaded", zoneID != ""),
			zap.Bool("domain_loaded", domain != ""))
		return newDisabledClient(logger, ctx)
	}

	cred := common.NewCredential(secretID, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = sdkEndpoint
	cpf.HttpProfile.ReqTimeout = int(defaultTimeout / time.Second)

	sdkClient, err := teo.NewClient(cred, "", cpf)
	if err != nil {
		logger.Warn("cdn sdk init failed", zap.Error(err))
		return newDisabledClient(logger, ctx)
	}

	return &Client{
		purger:       sdkClient,
		zoneID:       zoneID,
		domain:       domain,
		waitTimeout:  defaultPurgeWaitTimeout,
		pollInterval: defaultPurgePollInterval,
		logger:       logger,
		ctx:          ctx,
	}
}

func newDisabledClient(logger *zap.Logger, ctx context.Context) *Client {
	return &Client{
		logger:       logger,
		waitTimeout:  defaultPurgeWaitTimeout,
		pollInterval: defaultPurgePollInterval,
		ctx:          ctx,
	}
}

// enabled 判定 client 是否处于"可用"状态。purger 为 nil 时所有调用立即返回。
func (c *Client) enabled() bool {
	return c != nil && c.purger != nil && c.zoneID != "" && c.domain != ""
}
