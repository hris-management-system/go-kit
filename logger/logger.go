package logger

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"runtime"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ── global logger instances ───────────────────────────────────────────────────

var (
	Log   *zap.Logger
	Sugar *zap.SugaredLogger
)

func Init(env string) error {
	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	l, err := cfg.Build(zap.AddCallerSkip(0))
	if err != nil {
		return err
	}
	Log = l
	Sugar = l.Sugar()
	return nil
}

func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}

// ── context keys ──────────────────────────────────────────────────────────────

type ctxKey string

const (
	RequestID  ctxKey = "requestID"
	UserID     ctxKey = "userID"
	partnerID  ctxKey = "partnerID"
	FuncName   ctxKey = "funcName"
	Device     ctxKey = "device"
	AppVersion ctxKey = "appVersion"
)

// ── context helpers ───────────────────────────────────────────────────────────

func SetCtx(ctx context.Context, key ctxKey, value string) context.Context {
	return context.WithValue(ctx, key, value)
}

func GetCtx(ctx context.Context, key ctxKey) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(key).(string)
	return v
}

func SetRequestIDCtx(ctx context.Context) context.Context {
	b := make([]byte, 4)
	rand.Read(b)
	return SetCtx(ctx, RequestID, hex.EncodeToString(b))
}

func GetRequestIDCtx(ctx context.Context) string { return GetCtx(ctx, RequestID) }

func SetUserIDCtx(ctx context.Context, uid int64) context.Context {
	return SetCtx(ctx, UserID, fmt.Sprintf("%d", uid))
}

func GetUserIDCtx(ctx context.Context) string { return GetCtx(ctx, UserID) }

func SetPartnerIDCtx(ctx context.Context, pid string) context.Context {
	return SetCtx(ctx, partnerID, pid)
}

func GetPartnerIDCtx(ctx context.Context) string { return GetCtx(ctx, partnerID) }

func SetDeviceCtx(ctx context.Context, device string) context.Context {
	return SetCtx(ctx, Device, device)
}

func GetDeviceCtx(ctx context.Context) string { return GetCtx(ctx, Device) }

func SetAppVersionCtx(ctx context.Context, appVersion string) context.Context {
	return SetCtx(ctx, AppVersion, appVersion)
}

func GetAppVersionCtx(ctx context.Context) string { return GetCtx(ctx, AppVersion) }

func SetFuncNameCtx(ctx context.Context) context.Context {
	return SetCtx(ctx, FuncName, GetCallerFuncName(2))
}

func SetFuncNameHandlerCtx(ctx context.Context) context.Context {
	return SetCtx(ctx, FuncName, GetCallerFuncName(3))
}

func GetFuncNameCtx(ctx context.Context) string { return GetCtx(ctx, FuncName) }

func GetCallerFuncName(skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}
	details := runtime.FuncForPC(pc)
	if details == nil {
		return ""
	}
	parts := strings.Split(details.Name(), ".")
	return parts[len(parts)-1]
}

// ── context-aware logging ─────────────────────────────────────────────────────

// WithContext returns a *zap.Logger pre-populated with fields from ctx.
// Usage:  logger.WithContext(ctx).Info("something happened")
func WithContext(ctx context.Context) *zap.Logger {
	if Log == nil || ctx == nil {
		return Log
	}
	fields := make([]zap.Field, 0, 6)

	if v := GetRequestIDCtx(ctx); v != "" {
		fields = append(fields, zap.String("requestID", v))
	}
	if v := GetUserIDCtx(ctx); v != "" {
		fields = append(fields, zap.String("userID", v))
	}
	if v := GetPartnerIDCtx(ctx); v != "" {
		fields = append(fields, zap.String("partnerID", v))
	}
	if v := GetFuncNameCtx(ctx); v != "" {
		fields = append(fields, zap.String("funcName", v))
	}
	if v := GetDeviceCtx(ctx); v != "" {
		fields = append(fields, zap.String("device", v))
	}
	if v := GetAppVersionCtx(ctx); v != "" {
		fields = append(fields, zap.String("appVersion", v))
	}

	return Log.With(fields...)
}

// SugarWithContext returns a *zap.SugaredLogger pre-populated with fields from ctx.
func SugarWithContext(ctx context.Context) *zap.SugaredLogger {
	return WithContext(ctx).Sugar()
}
