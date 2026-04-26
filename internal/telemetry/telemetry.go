/*
 * Copyright (c) 2023, Deductive AI, Inc. All rights reserved.
 *
 * This software is the confidential and proprietary information of
 * Deductive AI, Inc. You shall not disclose such confidential
 * information and shall use it only in accordance with the terms of
 * the license agreement you entered into with Deductive AI, Inc.
 */

package telemetry

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/deductive-ai/dx/internal/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer
var enabled bool

// Enabled returns true if OTel telemetry is active (exporter configured).
func Enabled() bool {
	return enabled
}

// Init initializes the OTel tracer. Returns a shutdown function.
// Telemetry is opt-in: only enabled when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Set OTEL_SDK_DISABLED=true to explicitly disable even when endpoint is set.
func Init(version string) func() {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" || os.Getenv("OTEL_SDK_DISABLED") == "true" {
		tracer = otel.Tracer("dx-cli")
		return func() {}
	}

	ctx := context.Background()
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(endpoint),
	}
	if strings.HasPrefix(endpoint, "http://") {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		tracer = otel.Tracer("dx-cli")
		return func() {}
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("dx-cli"),
			semconv.ServiceVersion(version),
			attribute.String("deployment.environment", getEnv("DEPLOYMENT", "local")),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logging.Debug("otel export error", "error", err)
	}))

	tracer = tp.Tracer("dx-cli")
	enabled = true

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tp.Shutdown(ctx)
	}
}

// Tracer returns the global tracer
func Tracer() trace.Tracer {
	if tracer == nil {
		tracer = otel.Tracer("dx-cli")
	}
	return tracer
}

// StartSpan starts a new span with the given name and attributes
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
