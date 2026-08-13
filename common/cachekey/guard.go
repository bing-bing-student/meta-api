package cachekey

const nsGuard = "guard"

// GuardNonce 风控信封 nonce 防重放 Key。
func GuardNonce(scene string, nonceHex string) Key {
	return build(nsGuard, scene, "nonce", nonceHex)
}

// GuardDedup 风控主去重 Key，按场景、fingerprint 和目标资源隔离。
func GuardDedup(scene string, fpHex string, targetID string) Key {
	return build(nsGuard, scene, "dedup", fpHex, targetID)
}

// GuardToken 风控一次性 token Key。
func GuardToken(scene string, tokenHex string) Key {
	return build(nsGuard, scene, "token", tokenHex)
}

// GuardRate 风控频控 Key。
func GuardRate(scene string, dimension string, subject string, window string) Key {
	return build(nsGuard, scene, "rate", dimension, subject, window)
}
