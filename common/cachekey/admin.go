package cachekey

const nsAdmin = "admin"

// AdminRateLimit 管理员相关限流 Key。
func AdminRateLimit(parts ...string) Key {
	return build(append([]string{nsAdmin, "rate-limit"}, parts...)...)
}

// AdminLoginChallenge 账号密码校验通过后的二阶段登录挑战。
func AdminLoginChallenge(challenge string) Key {
	return build(nsAdmin, "login-challenge", challenge)
}

// AdminPendingTOTPSecret 管理员 TOTP 绑定前的临时密钥。
func AdminPendingTOTPSecret(challenge string) Key {
	return build(nsAdmin, "pending-totp-secret", challenge)
}

// AdminTOTPSecret 管理员 TOTP 临时密钥（历史缓存键，保留兼容旧调用方）
func AdminTOTPSecret(adminID string) Key {
	return build(nsAdmin, adminID, "secret")
}

// AboutMeHash 前台 "关于我" 信息缓存
//
// 历史 Key 是 "aboutMeInfo:Hash"（无 admin 前缀），保持原格式以避免存量缓存失效。
func AboutMeHash() Key { return "aboutMeInfo:Hash" }

// SMSCode 短信验证码缓存（按手机号隔离）
//
// 历史实现是全局共享一把 "code" 键，并发下不同手机号的验证码会互相覆盖，
// 会出现「A 申请的验证码被 B 的请求冲掉、A 拿 B 的验证码登录成功」的安全问题。
// 这里改为按手机号区分：sms:code:{phone}，调用方需保证已校验过 phone 合法。
func SMSCode(phone string) Key {
	return build("sms", "code", phone)
}
