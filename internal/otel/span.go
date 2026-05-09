package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/yuzu441/tools-go/internal/otel"

// StartSpan はトレーサー名を統一してスパンを開始する。
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name)
}

// RecordError はスパンにエラーを記録しステータスを設定する。
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// InSpan はスパンで囲んだ関数を実行し、エラー時に自動で記録する。
func InSpan[T any](ctx context.Context, name string, fn func(context.Context, trace.Span) (T, error)) (T, error) {
	ctx, span := StartSpan(ctx, name)
	defer span.End()

	result, err := fn(ctx, span)
	RecordError(span, err)
	return result, err
}
