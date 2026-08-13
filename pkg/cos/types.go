package cos

import (
	"errors"
	"time"

	tencentcos "github.com/tencentyun/cos-go-sdk-v5"
	"go.uber.org/zap"
)

const (
	envArticleImageCOSSecretID  = "ARTICLE_IMAGE_COS_SECRET_ID"
	envArticleImageCOSSecretKey = "ARTICLE_IMAGE_COS_SECRET_KEY"

	defaultTimeout = 15 * time.Second
)

var ErrDisabled = errors.New("article image COS storage is not configured")

type Client struct {
	client              *tencentcos.Client
	directory           string
	publicBaseURL       string
	hasCustomPublicBase bool
	logger              *zap.Logger
}
