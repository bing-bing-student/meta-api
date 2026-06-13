package edgeone

import (
	"os"
	"strings"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	teo "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/teo/v20220901"
	"go.uber.org/zap"
)

// 默认配置常量。
const (
	// defaultTimeout 调 EdgeOne CreatePurgeTask 接口的整体超时。
	// EdgeOne 接口通常 1s 内返回；留 5s 应对偶发链路抖动。
	defaultTimeout = 5 * time.Second

	// sdkEndpoint EdgeOne 公网接入域名。
	sdkEndpoint = "teo.tencentcloudapi.com"

	// envSecretID  腾讯云 API SecretId 所在环境变量名（不敏感）。
	envSecretID = "TENCENT_SECRET_ID"

	// envSecretKey 腾讯云 API SecretKey 所在环境变量名（敏感，建议走 docker secret 注入）。
	envSecretKey = "TENCENT_SECRET_KEY"

	// envZoneID EdgeOne 站点 ID 所在环境变量名（如 zone-xxxx）。
	envZoneID = "EDGEONE_ZONE_ID"

	// envPurgeDomain 用于拼接清理 URL 的站点域名前缀，
	// 例如 https://liubing.xyz；末尾斜杠由代码统一裁剪。
	envPurgeDomain = "EDGEONE_PURGE_DOMAIN"
)

// Client 用来调用 EdgeOne CreatePurgeTask 接口。
//
// 实例由 DI 容器构造，单例复用 SDK 内部 http 连接池。
// 当任一必备 env 缺失或 SDK 初始化失败，所有调用立即返回，不发起任何 API 请求。
type Client struct {
	purger  purgeAPI
	zoneID  string
	domain  string // 形如 https://liubing.xyz，末尾不带斜杠
	timeout time.Duration
	logger  *zap.Logger
}

// New 构造一个 EdgeOne 清缓存客户端。
func New(logger *zap.Logger) *Client {
	secretID := os.Getenv(envSecretID)
	secretKey := os.Getenv(envSecretKey)
	zoneID := os.Getenv(envZoneID)
	domain := strings.TrimRight(os.Getenv(envPurgeDomain), "/")

	if secretID == "" || secretKey == "" || zoneID == "" || domain == "" {
		logger.Warn("edgeOne disabled: required env missing",
			zap.Bool("secret_id_loaded", secretID != ""),
			zap.Bool("secret_key_loaded", secretKey != ""),
			zap.Bool("zone_id_loaded", zoneID != ""),
			zap.Bool("domain_loaded", domain != ""))
		return &Client{logger: logger, timeout: defaultTimeout}
	}

	cred := common.NewCredential(secretID, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = sdkEndpoint
	cpf.HttpProfile.ReqTimeout = int(defaultTimeout / time.Second)

	sdkClient, err := teo.NewClient(cred, "", cpf)
	if err != nil {
		logger.Warn("edgeOne sdk init failed", zap.Error(err))
		return &Client{logger: logger, timeout: defaultTimeout}
	}

	return &Client{
		purger:  sdkClient,
		zoneID:  zoneID,
		domain:  domain,
		timeout: defaultTimeout,
		logger:  logger,
	}
}

// enabled 判定 client 是否处于"可用"状态。purger 为 nil 时所有调用立即返回。
func (c *Client) enabled() bool {
	return c != nil && c.purger != nil && c.zoneID != "" && c.domain != ""
}
