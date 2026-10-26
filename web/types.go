package web

import "time"

type ResponseDefault struct {
	Status  bool   `json:"status"`
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

type ScopeAccess struct {
	Name        string `json:"feature_name"`
	Permission  string `json:"permission"`
	IsAllowed   bool   `json:"is_allowed"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	ServiceCode string `json:"service_code"`
}

type PayloadAuthorization struct {
	UserID     string        `json:"user_id"`
	Username   string        `json:"username"`
	Email      string        `json:"email"`
	ExpireDate time.Time     `json:"expire_date"`
	Scope      []ScopeAccess `json:"scope"`
}
