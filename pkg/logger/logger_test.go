package logger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"
)

type testHandler struct{}

func TestAttrs(t *testing.T) {
	tests := map[string][2]string{
		"AttrError":   {AttrError(errors.New("boom")).String(), "err=boom"},
		"AttrModule":  {AttrModule(&testHandler{}).String(), "mod=github.com/mikogs/gocrud/pkg/logger.testHandler"},
		"AttrService": {AttrService(&testHandler{}, "List").String(), "svc=github.com/mikogs/gocrud/pkg/logger.testHandler.List"},
		"AttrHandler": {AttrHandler(&testHandler{}).String(), "hand=github.com/mikogs/gocrud/pkg/logger.testHandler"},
		"AttrPath":    {AttrPath("/users/1").String(), "path=/users/1"},
	}
	for name, gotWant := range tests {
		if gotWant[0] != gotWant[1] {
			t.Fatalf("%s returned %q, want %q", name, gotWant[0], gotWant[1])
		}
	}
}

func TestSetLogger(t *testing.T) {
	defaultLogger := slog.Default()
	defer slog.SetDefault(defaultLogger)

	tests := map[string]slog.Level{"err": slog.LevelError, "warn": slog.LevelWarn, "debug": slog.LevelDebug, "": slog.LevelInfo, "other": slog.LevelInfo}
	for env, want := range tests {
		SetLogger(env)
		if !slog.Default().Enabled(context.Background(), want) || slog.Default().Enabled(context.Background(), want-1) {
			t.Fatalf("SetLogger(%q) did not set the level to %v", env, want)
		}
	}
}

func TestLogError(t *testing.T) {
	defaultLogger := slog.Default()
	defer slog.SetDefault(defaultLogger)

	var out bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))

	code := LogError("saving failed", AttrPath("/users"))
	if !regexp.MustCompile(`^ERR-[A-Z0-9]{8}-[0-9]+$`).MatchString(code) {
		t.Fatalf("LogError returned code %q, want ERR-<8 characters>-<time>", code)
	}
	if line := out.String(); !strings.Contains(line, `msg="saving failed"`) || !strings.Contains(line, "path=/users") || !strings.Contains(line, "err_code="+code) {
		t.Fatalf("LogError logged %q, want the message, the attributes and the code", line)
	}
}
