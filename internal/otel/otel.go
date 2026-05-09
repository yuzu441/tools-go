package otel

import (
	"context"
	"os"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Setup は OTEL_TRACES_EXPORTER が設定されていれば
// TracerProvider を構築しグローバルに登録する。
// 未設定ならトレース無効（no-op）。
func Setup(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }

	if _, ok := os.LookupEnv("OTEL_TRACES_EXPORTER"); !ok {
		return noop, nil
	}

	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return noop, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return noop, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
