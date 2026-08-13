package cos

import (
	"net/http"
	"strings"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
	"go.uber.org/zap"

	"meta-api/common/utils"
	"meta-api/config"
)

func New(cfg config.ArticleImageCOSConfig, logger *zap.Logger) *Client {
	bucket := strings.TrimSpace(cfg.Bucket)
	region := strings.TrimSpace(cfg.Region)
	secretID, err := utils.EnvOrFile(envCOSSecretID)
	if err != nil {
		logger.Warn("article image COS disabled: secret id file read failed", zap.Error(err))
		return &Client{logger: logger}
	}
	secretKey, err := utils.EnvOrFile(envCOSSecretKey)
	if err != nil {
		logger.Warn("article image COS disabled: secret key file read failed", zap.Error(err))
		return &Client{logger: logger}
	}

	if bucket == "" || region == "" || secretID == "" || secretKey == "" {
		logger.Warn("article image COS disabled: required config missing",
			zap.Bool("bucket_loaded", bucket != ""),
			zap.Bool("region_loaded", region != ""),
			zap.Bool("secret_id_loaded", secretID != ""),
			zap.Bool("secret_key_loaded", secretKey != ""))
		return &Client{logger: logger}
	}

	bucketURL, err := tencentcos.NewBucketURL(bucket, region, true)
	if err != nil {
		logger.Warn("article image COS disabled: invalid bucket url config", zap.Error(err))
		return &Client{logger: logger}
	}

	httpClient := &http.Client{
		Timeout: defaultTimeout,
		Transport: &tencentcos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	}

	customPublicBaseURL := strings.TrimSpace(cfg.PublicBaseURL)
	return &Client{
		client:              tencentcos.NewClient(&tencentcos.BaseURL{BucketURL: bucketURL}, httpClient),
		directory:           strings.Trim(strings.TrimSpace(cfg.Directory), "/"),
		publicBaseURL:       strings.TrimRight(firstNonEmpty(customPublicBaseURL, bucketURL.String()), "/"),
		hasCustomPublicBase: customPublicBaseURL != "",
		logger:              logger,
	}
}
