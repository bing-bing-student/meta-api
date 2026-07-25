package cachekey

const nsUserAuth = "user_auth"

func UserOAuthState(state string) Key {
	return build(nsUserAuth, "oauth", "state", state)
}
