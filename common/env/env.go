// Package env centralizes environment variable names used by the application.
package env

import "strings"

const (
	AppEnv        = "APP_ENV"
	AppProduction = "production"

	HTTPHost = "HTTP_HOST"
	HTTPPort = "HTTP_PORT"

	JWTSigningKey = "JWT_SIGNING_KEY"

	// SonyflakeMachineID pins the sonyflake machine ID.
	// It must be unique per running application instance.
	SonyflakeMachineID = "SONYFLAKE_MACHINE_ID"
	KeyDir             = "KEY_DIR"

	MySQLUsername     = "MYSQL_USERNAME"
	MySQLWorkPassword = "MYSQL_WORK_PASSWORD"
	MySQLHost         = "MYSQL_HOST"
	MySQLPort         = "MYSQL_PORT"
	MySQLDBName       = "MYSQL_DB_NAME"

	RedisAddress    = "REDIS_ADDRESS"
	RedisMasterName = "REDIS_MASTER_NAME"
	RedisPassword   = "REDIS_PASSWORD"

	EdgeOneSecretID    = "EDGEONE_SECRET_ID"
	EdgeOneSecretKey   = "EDGEONE_SECRET_KEY"
	EdgeOneZoneID      = "EDGEONE_ZONE_ID"
	EdgeOnePurgeDomain = "EDGEONE_PURGE_DOMAIN"

	COSSecretID  = "COS_SECRET_ID"
	COSSecretKey = "COS_SECRET_KEY"

	SitemapRevalidateEndpoint = "SITEMAP_REVALIDATE_ENDPOINT"
	SitemapRevalidateSecret   = "SITEMAP_REVALIDATE_SECRET"

	AliyunAccessKeyID     = "ALIYUN_ACCESS_KEY_ID"
	AliyunAccessKeySecret = "ALIYUN_ACCESS_KEY_SECRET"

	BugFeedbackSMTPHost     = "BUG_FEEDBACK_SMTP_HOST"
	BugFeedbackSMTPPort     = "BUG_FEEDBACK_SMTP_PORT"
	BugFeedbackSMTPUsername = "BUG_FEEDBACK_SMTP_USERNAME"
	BugFeedbackSMTPPassword = "BUG_FEEDBACK_SMTP_PASSWORD"
	BugFeedbackSMTPFrom     = "BUG_FEEDBACK_SMTP_FROM"
	BugFeedbackSMTPFromName = "BUG_FEEDBACK_SMTP_FROM_NAME"
)

func OAuthClientID(provider string) string {
	return oauthProviderEnv(provider, "CLIENT_ID")
}

func OAuthClientSecret(provider string) string {
	return oauthProviderEnv(provider, "CLIENT_SECRET")
}

func OAuthRedirectURI(provider string) string {
	return oauthProviderEnv(provider, "REDIRECT_URI")
}

func File(name string) string {
	return name + "_FILE"
}

func oauthProviderEnv(provider string, suffix string) string {
	return "OAUTH_" + strings.ToUpper(strings.TrimSpace(provider)) + "_" + suffix
}
