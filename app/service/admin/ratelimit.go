package admin

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"meta-api/common/cachekey"
	"meta-api/common/ratelimit"
	"meta-api/common/types"
	appconfig "meta-api/config"
)

const (
	defaultAccountLoginIPLimit       = 8
	defaultAccountLoginIPWindow      = 10 * time.Minute
	defaultAccountLoginUserLimit     = 5
	defaultAccountLoginUserWindow    = 30 * time.Minute
	defaultAccountLoginUserIPLimit   = 3
	defaultAccountLoginUserIPWindow  = 10 * time.Minute
	defaultAccountLoginFailureLimit  = 3
	defaultAccountLoginFailureWindow = 30 * time.Minute
	defaultAccountLoginLevelTTL      = 24 * time.Hour

	defaultBindDynamicCodeIPLimit     = 6
	defaultVerifyDynamicCodeIPLimit   = 10
	defaultDynamicCodeIPWindow        = 10 * time.Minute
	defaultDynamicCodeChallengeLimit  = 4
	defaultDynamicCodeChallengeWindow = loginChallengeTTL
	defaultDynamicCodeFailureLimit    = 4
	defaultDynamicCodeFailureWindow   = loginChallengeTTL
	unknownRateLimitClientValue       = "unknown"
)

var defaultAccountLoginLockDurations = []time.Duration{
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
	6 * time.Hour,
}

type accountLoginLimitKeys struct {
	ip       string
	user     string
	userIP   string
	failUser string
	lockUser string
	level    string
}

// checkAccountLoginLimit 检查账号密码登录的准入限流。
func (a *adminService) checkAccountLoginLimit(ctx context.Context, request *types.AccountLoginRequest) error {
	cfg := a.adminLoginRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	keys := buildAccountLoginLimitKeys(request.Username, request.ClientIP)
	if err := a.limiter.CheckLock(ctx, keys.lockUser); err != nil {
		return normalizeRateLimitError(err)
	}
	return normalizeRateLimitError(a.limiter.Check(ctx,
		rateLimitRule(keys.ip, cfg.AccountLogin.IP),
		rateLimitRule(keys.user, cfg.AccountLogin.Username),
		rateLimitRule(keys.userIP, cfg.AccountLogin.UsernameIP),
	))
}

// recordAccountLoginFailure 记录账号密码登录失败并触发退避锁定。
func (a *adminService) recordAccountLoginFailure(ctx context.Context, username string) error {
	cfg := a.adminLoginRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	keys := buildAccountLoginLimitKeys(username, "")
	lockDurations := secondsListToDurations(cfg.AccountLogin.LockDurationsSeconds)
	if len(lockDurations) == 0 {
		lockDurations = cloneDurations(defaultAccountLoginLockDurations)
	}
	return normalizeRateLimitError(a.limiter.RecordFailure(ctx, keys.failUser, keys.lockUser, keys.level, ratelimit.BackoffConfig{
		Threshold:  cfg.AccountLogin.FailureThreshold,
		CounterTTL: secondsToDuration(cfg.AccountLogin.FailureWindowSeconds),
		LevelTTL:   secondsToDuration(cfg.AccountLogin.LockLevelTTLSeconds),
		Durations:  lockDurations,
	}))
}

// clearAccountLoginState 清理账号登录失败和锁定状态。
func (a *adminService) clearAccountLoginState(ctx context.Context, username string) {
	keys := buildAccountLoginLimitKeys(username, "")
	if err := a.limiter.Clear(ctx, keys.failUser, keys.lockUser, keys.level); err != nil {
		a.logger.Warn("failed to clear account login rate-limit state", zap.Error(err))
	}
}

// buildAccountLoginLimitKeys 构造账号登录相关的 Redis 限流 Key。
func buildAccountLoginLimitKeys(username, clientIP string) accountLoginLimitKeys {
	userHash := ratelimit.HashPart(normalizeRateLimitValue(username))
	ipHash := ratelimit.HashPart(normalizeClientIP(clientIP))
	return accountLoginLimitKeys{
		ip:       cachekey.AdminRateLimit("account-login", "ip", ipHash).String(),
		user:     cachekey.AdminRateLimit("account-login", "user", userHash).String(),
		userIP:   cachekey.AdminRateLimit("account-login", "user-ip", userHash, ipHash).String(),
		failUser: cachekey.AdminRateLimit("account-login", "fail", "user", userHash).String(),
		lockUser: cachekey.AdminRateLimit("account-login", "lock", "user", userHash).String(),
		level:    cachekey.AdminRateLimit("account-login", "lock-level", "user", userHash).String(),
	}
}

// checkBindDynamicCodeLimit 检查 TOTP 绑定入口的准入限流。
func (a *adminService) checkBindDynamicCodeLimit(ctx context.Context, request *types.BindDynamicCodeRequest) error {
	cfg := a.adminLoginRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	return a.checkDynamicCodeLimit(ctx, "bind-dynamic-code", request.ClientIP, request.LoginChallenge, cfg.BindDynamicCode)
}

// checkVerifyDynamicCodeLimit 检查 TOTP 验证入口的准入限流。
func (a *adminService) checkVerifyDynamicCodeLimit(ctx context.Context, request *types.VerifyDynamicCodeRequest) error {
	cfg := a.adminLoginRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	return a.checkDynamicCodeLimit(ctx, "verify-dynamic-code", request.ClientIP, request.LoginChallenge, cfg.VerifyDynamicCode)
}

// checkDynamicCodeLimit 检查动态验证码入口的 IP 和 challenge 限流。
func (a *adminService) checkDynamicCodeLimit(ctx context.Context, action, clientIP, challenge string,
	cfg appconfig.DynamicCodeRateLimitConfig) error {
	challengeHash := ratelimit.HashPart(challenge)
	ipHash := ratelimit.HashPart(normalizeClientIP(clientIP))
	return normalizeRateLimitError(a.limiter.Check(ctx,
		rateLimitRule(cachekey.AdminRateLimit(action, "ip", ipHash).String(), cfg.IP),
		rateLimitRule(cachekey.AdminRateLimit(action, "challenge", challengeHash).String(), cfg.Challenge),
	))
}

// recordBindDynamicCodeFailure 记录 TOTP 绑定失败次数。
func (a *adminService) recordBindDynamicCodeFailure(ctx context.Context, challenge string, extraKeys ...string) error {
	cfg := a.adminLoginRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	return a.recordDynamicCodeFailure(ctx, challenge, cfg.BindDynamicCode, extraKeys...)
}

// recordVerifyDynamicCodeFailure 记录 TOTP 验证失败次数。
func (a *adminService) recordVerifyDynamicCodeFailure(ctx context.Context, challenge string, extraKeys ...string) error {
	cfg := a.adminLoginRateLimitConfig()
	if cfg.Disabled {
		return nil
	}
	return a.recordDynamicCodeFailure(ctx, challenge, cfg.VerifyDynamicCode, extraKeys...)
}

// recordDynamicCodeFailure 记录动态验证码失败并在超限后清理 challenge。
func (a *adminService) recordDynamicCodeFailure(ctx context.Context, challenge string,
	cfg appconfig.DynamicCodeRateLimitConfig, extraKeys ...string) error {
	failKey := dynamicCodeFailureKey(challenge)
	count, err := a.limiter.Increment(ctx, failKey, secondsToDuration(cfg.FailureWindowSeconds))
	if err != nil {
		return errors.New("登录服务暂不可用")
	}
	if count < cfg.FailureThreshold {
		return nil
	}
	if err = a.clearDynamicCodeState(ctx, challenge, extraKeys...); err != nil {
		return errors.New("登录状态清理失败")
	}
	return errors.New("动态验证码错误次数过多，请重新输入账号密码")
}

// normalizeRateLimitError 将存储层错误转换为统一登录错误。
func normalizeRateLimitError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := ratelimit.AsLimited(err); ok {
		return err
	}
	return errors.New("登录服务暂不可用")
}

// adminLoginRateLimitConfig 获取当前后台登录限流配置并填充默认值。
func (a *adminService) adminLoginRateLimitConfig() appconfig.AdminLoginRateLimitConfig {
	cfg := appconfig.AdminLoginRateLimitConfig{}
	if a != nil && a.config != nil {
		cfg = a.config.RateLimitSnapshot().AdminLogin
	}
	fillAdminLoginRateLimitDefaults(&cfg)
	return cfg
}

// fillAdminLoginRateLimitDefaults 填充后台登录限流默认配置。
func fillAdminLoginRateLimitDefaults(cfg *appconfig.AdminLoginRateLimitConfig) {
	fillWindowConfig(&cfg.AccountLogin.IP, defaultAccountLoginIPLimit, defaultAccountLoginIPWindow)
	fillWindowConfig(&cfg.AccountLogin.Username, defaultAccountLoginUserLimit, defaultAccountLoginUserWindow)
	fillWindowConfig(&cfg.AccountLogin.UsernameIP, defaultAccountLoginUserIPLimit, defaultAccountLoginUserIPWindow)
	if cfg.AccountLogin.FailureThreshold <= 0 {
		cfg.AccountLogin.FailureThreshold = defaultAccountLoginFailureLimit
	}
	if cfg.AccountLogin.FailureWindowSeconds <= 0 {
		cfg.AccountLogin.FailureWindowSeconds = int64(defaultAccountLoginFailureWindow / time.Second)
	}
	if cfg.AccountLogin.LockLevelTTLSeconds <= 0 {
		cfg.AccountLogin.LockLevelTTLSeconds = int64(defaultAccountLoginLevelTTL / time.Second)
	}
	if len(cfg.AccountLogin.LockDurationsSeconds) == 0 {
		cfg.AccountLogin.LockDurationsSeconds = durationsToSeconds(defaultAccountLoginLockDurations)
	}

	fillDynamicCodeRateLimitDefaults(&cfg.BindDynamicCode, defaultBindDynamicCodeIPLimit)
	fillDynamicCodeRateLimitDefaults(&cfg.VerifyDynamicCode, defaultVerifyDynamicCodeIPLimit)
}

// fillDynamicCodeRateLimitDefaults 填充动态验证码限流默认配置。
func fillDynamicCodeRateLimitDefaults(cfg *appconfig.DynamicCodeRateLimitConfig, defaultIPLimit int64) {
	fillWindowConfig(&cfg.IP, defaultIPLimit, defaultDynamicCodeIPWindow)
	fillWindowConfig(&cfg.Challenge, defaultDynamicCodeChallengeLimit, defaultDynamicCodeChallengeWindow)
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = defaultDynamicCodeFailureLimit
	}
	if cfg.FailureWindowSeconds <= 0 {
		cfg.FailureWindowSeconds = int64(defaultDynamicCodeFailureWindow / time.Second)
	}
}

// fillWindowConfig 填充单条窗口规则的默认值。
func fillWindowConfig(cfg *appconfig.RateLimitWindowConfig, defaultLimit int64, defaultWindow time.Duration) {
	if cfg.Limit <= 0 {
		cfg.Limit = defaultLimit
	}
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = int64(defaultWindow / time.Second)
	}
}

// rateLimitRule 将配置项转换为限流规则。
func rateLimitRule(key string, cfg appconfig.RateLimitWindowConfig) ratelimit.Rule {
	return ratelimit.Rule{
		Key:    key,
		Limit:  cfg.Limit,
		Window: secondsToDuration(cfg.WindowSeconds),
	}
}

// secondsToDuration 将配置中的秒数转换为 Duration。
func secondsToDuration(seconds int64) time.Duration {
	if seconds <= 0 {
		return time.Second
	}
	return time.Duration(seconds) * time.Second
}

// secondsListToDurations 将秒数列表转换为 Duration 列表。
func secondsListToDurations(secondsList []int64) []time.Duration {
	if len(secondsList) == 0 {
		return nil
	}
	durations := make([]time.Duration, 0, len(secondsList))
	for _, seconds := range secondsList {
		if seconds <= 0 {
			continue
		}
		durations = append(durations, secondsToDuration(seconds))
	}
	if len(durations) == 0 {
		return nil
	}
	return durations
}

// durationsToSeconds 将 Duration 列表转换为配置秒数列表。
func durationsToSeconds(durations []time.Duration) []int64 {
	if len(durations) == 0 {
		return nil
	}
	seconds := make([]int64, 0, len(durations))
	for _, duration := range durations {
		if duration <= 0 {
			continue
		}
		seconds = append(seconds, int64(duration/time.Second))
	}
	return seconds
}

// cloneDurations 复制 Duration 列表，避免共享底层数组。
func cloneDurations(src []time.Duration) []time.Duration {
	if len(src) == 0 {
		return nil
	}
	dst := make([]time.Duration, len(src))
	copy(dst, src)
	return dst
}

// clearDynamicCodeState 清理二阶段登录和动态验证码限流状态。
func (a *adminService) clearDynamicCodeState(ctx context.Context, challenge string, extraKeys ...string) error {
	keys := []string{
		cachekey.AdminLoginChallenge(challenge).String(),
		cachekey.AdminPendingTOTPSecret(challenge).String(),
		dynamicCodeFailureKey(challenge),
		dynamicCodeChallengeKey("bind-dynamic-code", challenge),
		dynamicCodeChallengeKey("verify-dynamic-code", challenge),
	}
	keys = append(keys, extraKeys...)
	if err := a.limiter.Clear(ctx, keys...); err != nil {
		a.logger.Warn("failed to clear dynamic code rate-limit state", zap.Error(err))
		return err
	}
	return nil
}

// dynamicCodeFailureKey 构造动态验证码失败计数 Key。
func dynamicCodeFailureKey(challenge string) string {
	return cachekey.AdminRateLimit("dynamic-code", "fail", "challenge", ratelimit.HashPart(challenge)).String()
}

// dynamicCodeChallengeKey 构造动态验证码 challenge 限流 Key。
func dynamicCodeChallengeKey(action, challenge string) string {
	return cachekey.AdminRateLimit(action, "challenge", ratelimit.HashPart(challenge)).String()
}

// normalizeRateLimitValue 标准化限流身份值。
func normalizeRateLimitValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return unknownRateLimitClientValue
	}
	return value
}

// normalizeClientIP 标准化客户端 IP 维度。
func normalizeClientIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return unknownRateLimitClientValue
	}
	return ip
}
