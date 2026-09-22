package errors

import (
	"fmt"
	"net/http"
)

type Code uint8

type CodeDetail struct {
	Message        string
	HttpStatus     int
	InternalStatus int
}

// AppError wraps a Code and implements the error interface.
type AppError struct {
	Code  Code
	cause error
}

func New(code Code) *AppError {
	return &AppError{Code: code}
}

func Wrap(code Code, cause error) *AppError {
	return &AppError{Code: code, cause: cause}
}

func (e *AppError) Error() string {
	d := Detail(e.Code)
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", d.Message, e.cause)
	}
	return d.Message
}

func (e *AppError) Unwrap() error { return e.cause }

func (e *AppError) HttpStatus() int { return Detail(e.Code).HttpStatus }

// Detail returns the CodeDetail for a given Code.
// Falls back to Undefined if the code is not registered.
func Detail(c Code) CodeDetail {
	if d, ok := codes[c]; ok {
		return d
	}
	return codes[Undefined]
}

const (
	Panic Code = iota
	Undefined

	InvalidRequestParams
	InvalidPackageID
	InvalidPaymentMethodID

	FailToSelect
	FailToInsert
	FailToUpdate

	SQLDataIsNotFound
	SQLDataExist

	RedisDataIsNotFound

	InvalidAuthorizationCode
	InvalidJWTTokenAuth
	InvalidJWTToken
	InvalidJWTTokenUUID
	InvalidCallbackParams
	UnverifiedUser

	InvalidAPIKeyEnv
	InvalidSecretKeyEnv

	InvalidHeaderField
	InvalidHeaderValue

	InvalidUserAuth

	InvalidClaimCode
	DeviceAlreadyClaimed
	InvalidClaimTarget
	InvalidDeviceToken
	RateLimited
)

var codes = map[Code]CodeDetail{
	Panic: {
		Message:        "panic recovered",
		HttpStatus:     http.StatusInternalServerError,
		InternalStatus: 5000,
	},
	Undefined: {
		Message:        "undefined error code",
		HttpStatus:     http.StatusInternalServerError,
		InternalStatus: 5001,
	},
	InvalidRequestParams: {
		Message:        "invalid request params",
		HttpStatus:     http.StatusBadRequest,
		InternalStatus: 4001,
	},
	InvalidPackageID: {
		Message:        "invalid package id",
		HttpStatus:     http.StatusBadRequest,
		InternalStatus: 4002,
	},
	InvalidPaymentMethodID: {
		Message:        "invalid payment method id",
		HttpStatus:     http.StatusBadRequest,
		InternalStatus: 4003,
	},
	InvalidHeaderField: {
		Message:        "invalid or missing header field",
		HttpStatus:     http.StatusBadRequest,
		InternalStatus: 4004,
	},
	InvalidHeaderValue: {
		Message:        "invalid header value",
		HttpStatus:     http.StatusBadRequest,
		InternalStatus: 4005,
	},
	FailToSelect: {
		Message:        "fail to select",
		HttpStatus:     http.StatusInternalServerError,
		InternalStatus: 5010,
	},
	FailToInsert: {
		Message:        "fail to insert",
		HttpStatus:     http.StatusInternalServerError,
		InternalStatus: 5011,
	},
	FailToUpdate: {
		Message:        "fail to update",
		HttpStatus:     http.StatusInternalServerError,
		InternalStatus: 5012,
	},
	SQLDataIsNotFound: {
		Message:        "data not found",
		HttpStatus:     http.StatusNotFound,
		InternalStatus: 4040,
	},
	SQLDataExist: {
		Message:        "data already exists",
		HttpStatus:     http.StatusConflict,
		InternalStatus: 4090,
	},
	RedisDataIsNotFound: {
		Message:        "cache data not found",
		HttpStatus:     http.StatusNotFound,
		InternalStatus: 4041,
	},
	InvalidAuthorizationCode: {
		Message:        "invalid authorization code",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4010,
	},
	InvalidJWTTokenAuth: {
		Message:        "invalid jwt token auth",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4011,
	},
	InvalidJWTToken: {
		Message:        "invalid jwt token",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4012,
	},
	InvalidJWTTokenUUID: {
		Message:        "invalid jwt token uuid",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4013,
	},
	InvalidCallbackParams: {
		Message:        "invalid callback params",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4014,
	},
	UnverifiedUser: {
		Message:        "unverified user",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4015,
	},
	InvalidUserAuth: {
		Message:        "invalid username / password",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4016,
	},
	InvalidAPIKeyEnv: {
		Message:        "API key not valid",
		HttpStatus:     http.StatusForbidden,
		InternalStatus: 4030,
	},
	InvalidSecretKeyEnv: {
		Message:        "secret key not valid",
		HttpStatus:     http.StatusForbidden,
		InternalStatus: 4031,
	},
	// Deliberately one code for "unknown", "expired", "already consumed" and
	// "tenant suspended". Distinguishing them tells an attacker probing short
	// codes whether a guess was ever a real code (SRS §4.2 rule 3).
	InvalidClaimCode: {
		Message:        "claim code is invalid or has expired",
		HttpStatus:     http.StatusNotFound,
		InternalStatus: 4042,
	},
	DeviceAlreadyClaimed: {
		Message:        "device is already claimed",
		HttpStatus:     http.StatusConflict,
		InternalStatus: 4091,
	},
	// Same code for absent, malformed, unknown, revoked and suspended-tenant
	// tokens: probing must not distinguish them.
	InvalidDeviceToken: {
		Message:        "invalid or revoked device token",
		HttpStatus:     http.StatusUnauthorized,
		InternalStatus: 4017,
	},
	RateLimited: {
		Message:        "too many requests",
		HttpStatus:     http.StatusTooManyRequests,
		InternalStatus: 4290,
	},
	InvalidClaimTarget: {
		Message:        "claim target is not provisioned correctly",
		HttpStatus:     http.StatusUnprocessableEntity,
		InternalStatus: 4220,
	},
}
