package telemetry

import (
	"context"
	"net/url"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceName = "openai-gateway"
	tracerName  = "openai-gateway"
)

var tracer trace.Tracer
var enabled bool

// Init initializes OpenTelemetry tracing if OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Returns a shutdown function that should be called on application exit.
func Init(ctx context.Context) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		tracer = otel.Tracer(tracerName)
		return func(context.Context) error { return nil }, nil
	}

	env := os.Getenv("ENV")
	if env == "" {
		env = "unknown"
	}

	version := os.Getenv("VERSION")
	if version == "" {
		version = "unknown"
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			semconv.DeploymentEnvironmentName(env),
		),
	)
	if err != nil {
		return nil, err
	}

	// Set up propagator for distributed tracing
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	// Parse the endpoint URL to extract host:port and determine if insecure
	opts := []otlptracehttp.Option{}
	parsedURL, err := url.Parse(endpoint)
	if err == nil && parsedURL.Host != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(parsedURL.Host))
		if parsedURL.Scheme == "http" {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
	} else {
		// Fallback: treat as host:port directly
		opts = append(opts, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	tracer = tp.Tracer(tracerName)
	enabled = true

	return tp.Shutdown, nil
}

// Tracer returns the global tracer instance.
func Tracer() trace.Tracer {
	if tracer == nil {
		tracer = otel.Tracer(tracerName)
	}
	return tracer
}

// StartSpan starts a new span with the given name.
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name)
}

// SetSpanAttributes sets attributes on the current span.
func SetSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// RecordError records an error on the span and sets the span status to Error.
func RecordError(span trace.Span, err error, description string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, description)
}

// SetSpanError sets the span status to Error with the given description.
func SetSpanError(span trace.Span, description string) {
	span.SetStatus(codes.Error, description)
}

// Enabled returns true if tracing is enabled.
func Enabled() bool {
	return enabled
}
