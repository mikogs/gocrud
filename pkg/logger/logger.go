package logger

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"time"
)

const (
	logAttrError   = "err"
	logAttrModule  = "mod"
	logAttrService = "reg"
	logAttrHandler = "hand"
	logAttrPath    = "path"
)

func SetLogger(envValue string) {
	logLevel := logLevelFromEnv(envValue)
	slog.Info("log level from env", "level", logLevel)

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)
}

func AttrError(err error) slog.Attr {
	return slog.String(logAttrError, err.Error())
}

func AttrModule(obj interface{}) slog.Attr {
	name := reflect.TypeOf(obj).Elem().PkgPath() + "." + reflect.TypeOf(obj).Elem().Name()

	return slog.String(logAttrModule, name)
}

func AttrService(obj interface{}, action string) slog.Attr {
	name := reflect.TypeOf(obj).Elem().PkgPath() + "." + reflect.TypeOf(obj).Elem().Name() + "." + action

	return slog.String(logAttrService, name)
}

func AttrHandler(obj interface{}) slog.Attr {
	name := reflect.TypeOf(obj).Elem().PkgPath() + "." + reflect.TypeOf(obj).Elem().Name()

	return slog.String(logAttrHandler, name)
}

func AttrPath(path string) slog.Attr {
	return slog.String(logAttrPath, path)
}

func logLevelFromEnv(s string) slog.Level {
	switch s {
	case "err":
		return slog.LevelError
	case "warn":
		return slog.LevelWarn
	case "debug":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// LogError logs msg with attrs as an error, with a code that it returns, e.g. to show it to a user, who can then tell
// which error they got: ERR-<8 random letters and digits>-<time in nanoseconds>.
func LogError(msg string, attrs ...interface{}) string {
	// crypto/rand, as the code of an error should not be predictable, e.g. to point to the error of another user
	code := fmt.Sprintf("ERR-%s-%d", rand.Text()[:8], time.Now().UnixNano())
	attrs = append(attrs, slog.String("err_code", code))
	slog.Error(msg, attrs...)
	return code
}
