package web

import (
	"log"

	"github.com/gin-gonic/gin"

	"github.com/radian-solusi/microservice-helpers/validate"
)

const (
	NoCode              = 0
	Success             = 200
	SuccessCreate       = 201
	SuccessNoContent    = 204
	BadRequest          = 400
	Unauthorized        = 401
	Forbidden           = 403
	NotFound            = 404
	MethodNotAllowed    = 405
	Conflict            = 409
	ValidationError     = 422
	TooManyRequest      = 429
	InternalServerError = 500
	ServiceBroken       = 502
	ServiceUnavailable  = 503
	GatewayTimeout      = 504
	Expired             = 419
	WrongCredential     = 1001
)

func ErrorMessage(code int) string {
	switch code {
	case BadRequest:
		return "Bad Request"
	case Unauthorized:
		return "Unauthorized"
	case Forbidden:
		return "Forbidden"
	case NotFound:
		return "Not Found"
	case MethodNotAllowed:
		return "Method Not Allowed"
	case InternalServerError:
		return "Internal Server Error"
	case ServiceUnavailable:
		return "Service Unavailable"
	case GatewayTimeout:
		return "Gateway Timeout"
	case ServiceBroken:
		return "Service Not Completed"
	case WrongCredential:
		return "Username or Password is incorrect"
	case TooManyRequest:
		return "Too Many Request"
	}
	return ""
}

func ErrorResponse(ctx *gin.Context, code int, message string) {
	if code == WrongCredential {
		code = Unauthorized
	}
	logErrorResponse(ctx, code, message)
	SendResponse(ctx, ResponseDefault{Status: false, Code: code, Data: nil, Message: message})
	ctx.Abort()
}

// ValidationErrorResponse sends a 422 with a nested per-field message map
// translated to the request language. Non-validation errors yield an empty
// data map and the localized generic message — raw details never leak.
func ValidationErrorResponse(ctx *gin.Context, err error) {
	ValidationErrorResponseWithRef(ctx, err, nil)
}

// ValidationErrorResponseWithRef is ValidationErrorResponse with the bound
// request struct as ref, so field paths honour its json/form tags and
// embedded-struct flattening.
func ValidationErrorResponseWithRef(ctx *gin.Context, err error, ref any) {
	lang := GetLanguageCtx(ctx)
	data := validate.FormatValidationErrorDataWithRef(err, lang, ref)
	logValidationError(ctx, err)
	SendResponse(ctx, ResponseDefault{
		Status:  false,
		Code:    ValidationError,
		Data:    data,
		Message: validate.FormatValidationSummary(err, lang),
	})
	ctx.Abort()
}

type ErrorCodeMapper func(error) int

func HandleErrorResponse(ctx *gin.Context, err error, mapper ErrorCodeMapper) {
	code := InternalServerError
	if mapper != nil {
		if mapped := mapper(err); mapped != 0 {
			code = mapped
		}
	}
	ErrorResponse(ctx, code, err.Error())
}

// logErrorResponse writes one log line for every error response (4xx/5xx):
// method, path, status, message. No response-body/contract change — this is
// observability only. Never pass request bodies or headers here (secrets).
func logErrorResponse(ctx *gin.Context, code int, message string) {
	method, path := "", ""
	if ctx != nil && ctx.Request != nil {
		method = ctx.Request.Method
		if ctx.Request.URL != nil {
			path = ctx.Request.URL.Path
		}
	}
	log.Printf("error_response method=%s path=%s status=%d message=%q", method, path, code, message)
}

// logValidationError logs the field-level breakdown (field/rule/param) when
// err maps to validator.ValidationErrors/*AppValidationError, otherwise logs
// the raw err.Error() so a non-field-level cause (e.g. JSON unmarshal/bind
// errors) is never silently reduced to the generic "Invalid input data"
// fallback without a trace.
func logValidationError(ctx *gin.Context, err error) {
	if err == nil {
		logErrorResponse(ctx, ValidationError, "")
		return
	}
	fields := validate.CollectFieldErrors(err)
	if len(fields) == 0 {
		logErrorResponse(ctx, ValidationError, "unrecognized bind/validation error: "+err.Error())
		return
	}
	method, path := "", ""
	if ctx != nil && ctx.Request != nil {
		method = ctx.Request.Method
		if ctx.Request.URL != nil {
			path = ctx.Request.URL.Path
		}
	}
	for _, fe := range fields {
		log.Printf("error_response method=%s path=%s status=%d field=%s rule=%s param=%q",
			method, path, ValidationError, fe.Field, fe.Tag, fe.Param)
	}
}
