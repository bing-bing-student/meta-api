package types

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type RetryAfterResponse struct {
	RetryAfter int `json:"retryAfter"`
}
