package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.SugaredLogger

func init() {
	level := GetZapLevelFromEnv() // Use the function from utils
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)

	raw, err := config.Build()
	if err != nil {
		panic("failed to build zap logger: " + err.Error())
	}
	Log = raw.Sugar()
}

func GetZapLevelFromEnv() zapcore.Level {
	levelStr := strings.ToLower(os.Getenv("LOG_LEVEL"))
	switch levelStr {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel // fallback
	}
}
