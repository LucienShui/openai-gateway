import os

from fastapi import FastAPI
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.resources import Resource, SERVICE_NAME, DEPLOYMENT_ENVIRONMENT
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor


def patch_open_telemetry(target: FastAPI):
    if (base_endpoint := os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT", None)) is None:
        raise Exception("OTEL_EXPORTER_OTLP_ENDPOINT environment variable is not set")

    resource = Resource.create(attributes={
        SERVICE_NAME: "openai-gateway",
        DEPLOYMENT_ENVIRONMENT: os.environ.get("ENV", "unknown")
    })
    trace_provider = TracerProvider(resource=resource)

    endpoint = base_endpoint + "/v1/traces"
    otlp_exporter = OTLPSpanExporter(endpoint=endpoint)
    span_processor = BatchSpanProcessor(otlp_exporter)

    trace_provider.add_span_processor(span_processor)
    
    # Only set the tracer provider if the default one is still active
    # This prevents errors when running with multiple workers
    current_provider = trace.get_tracer_provider()
    if type(current_provider).__name__ == 'ProxyTracerProvider':
        trace.set_tracer_provider(trace_provider)

    FastAPIInstrumentor.instrument_app(target, excluded_urls="health", exclude_spans=["send"])
