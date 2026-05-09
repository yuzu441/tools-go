package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	otelsetup "github.com/yuzu441/tools-go/internal/otel"
	"github.com/yuzu441/tools-go/translate-meta/internal/cli"
)

func main() {
	ctx := context.Background()

	shutdown, err := otelsetup.Setup(ctx, "translate-meta")
	if err != nil {
		slog.Warn("failed to setup OTEL", "error", err)
	}

	code := cli.Run(ctx, os.Args[1:])

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil {
		slog.Warn("OTEL shutdown error", "error", err)
	}

	os.Exit(code)
}
