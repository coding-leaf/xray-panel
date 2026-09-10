package domain

import "errors"

var (
	ErrNotFound          = errors.New("resource not found")
	ErrAlreadyExists     = errors.New("resource already exists")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrInvalidInput      = errors.New("invalid input data")
	ErrXrayUnavailable   = errors.New("xray service or grpc is unreachable")
	ErrInvalidConfig     = errors.New("invalid xray configuration")
	ErrQuotaExceeded     = errors.New("user traffic quota exceeded")
	ErrUserDisabled      = errors.New("user is disabled")
	ErrSubscriptionToken      = errors.New("invalid or expired subscription token")
	ErrTicketInvalidOrExpired = errors.New("invalid, expired or consumed ticket")
	ErrIPRateLimited          = errors.New("ip temporarily banned due to excessive failed attempts")
)
