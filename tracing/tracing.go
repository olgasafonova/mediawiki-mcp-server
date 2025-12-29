// Package tracing provides OpenTelemetry tracing for the MediaWiki MCP server.
// It configures trace exporters and provides utilities for creating spans.
package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	TracerName = "mediawiki-mcp-server"
)

// Config holds tracing configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Enabled        bool
	OTLPEndpoint   string // If set, uses OTLP exporter; otherwise stdout
	SampleRate     float64
}

// DefaultConfig returns sensible defaults for tracing
func DefaultConfig() Config {
	return Config{
		ServiceName:    "mediawiki-mcp-server",
		ServiceVersion: "1.0.0",
		Environment:    getEnvOrDefault("OTEL_ENVIRONMENT", "development"),
		Enabled:        os.Getenv("OTEL_ENABLED") == "true" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "",
		OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		SampleRate:     1.0,
	}
}

// Setup initializes OpenTelemetry tracing and returns a shutdown function
func Setup(ctx context.Context, config Config) (func(context.Context) error, error) {
	if !config.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	// Build resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			attribute.String("environment", config.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create appropriate exporter
	var exporter sdktrace.SpanExporter
	if config.OTLPEndpoint != "" {
		exporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(config.OTLPEndpoint),
			otlptracehttp.WithInsecure(),
		)
	} else {
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	if err != nil {
		return nil, err
	}

	// Create sampler based on config
	var sampler sdktrace.Sampler
	if config.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if config.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(config.SampleRate)
	}

	// Create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global providers
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns the named tracer for the server
func Tracer() trace.Tracer {
	return otel.Tracer(TracerName)
}

// StartSpan starts a new span with the given name and returns the context and span
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name, opts...)
}

// AddToolAttributes adds standard tool attributes to a span
func AddToolAttributes(span trace.Span, toolName, category string) {
	span.SetAttributes(
		attribute.String("mcp.tool.name", toolName),
		attribute.String("mcp.tool.category", category),
	)
}

// AddWikiAttributes adds wiki-related attributes to a span
func AddWikiAttributes(span trace.Span, action, page string) {
	span.SetAttributes(
		attribute.String("wiki.api.action", action),
	)
	if page != "" {
		span.SetAttributes(attribute.String("wiki.page.title", page))
	}
}

// RecordError records an error on the span
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
