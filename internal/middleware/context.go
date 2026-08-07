package middleware

type ctxKey string

const (
	CtxRequestID ctxKey = "request_id"
	CtxAPIKey    ctxKey = "api_key"
	CtxAPIKeyID  ctxKey = "api_key_id"
	CtxIsAdmin   ctxKey = "is_admin"
	CtxUID       ctxKey = "uid" // X-User-Id
)
