/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package tracing

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartConfig holds options for creating a new span.
type StartConfig struct {
	spanOpts []trace.SpanStartOption
}

// SpanOpt configures a span at creation time.
type SpanOpt func(config *StartConfig)

// WithAttribute adds an attribute to the span at creation.
func WithAttribute(k string, v interface{}) SpanOpt {
	return func(config *StartConfig) {
		config.spanOpts = append(config.spanOpts,
			trace.WithAttributes(Attribute(k, v)))
	}
}

// UpdateHTTPClient wraps the client's transport with OpenTelemetry instrumentation.
func UpdateHTTPClient(client *http.Client, name string) {
	client.Transport = otelhttp.NewTransport(
		nil,
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return name
		}),
	)
}

// StartSpan creates a child span and returns updated context.
func StartSpan(ctx context.Context, opName string, opts ...SpanOpt) (context.Context, *Span) {
	config := StartConfig{}
	for _, fn := range opts {
		fn(&config)
	}
	tracer := otel.Tracer("")
	if parent := trace.SpanFromContext(ctx); parent != nil && parent.SpanContext().IsValid() {
		tracer = parent.TracerProvider().Tracer("")
	}
	ctx, span := tracer.Start(ctx, opName, config.spanOpts...)
	return ctx, &Span{otelSpan: span}
}

// SpanFromContext returns the Span from the context, or a no-op span if none.
func SpanFromContext(ctx context.Context) *Span {
	return &Span{
		otelSpan: trace.SpanFromContext(ctx),
	}
}

// Span is wrapper around otel trace.Span.
// Span is the individual component of a trace. It represents a
// single named and timed operation of a workflow that is traced.
type Span struct {
	otelSpan trace.Span
}

// End finishes the span. Must be called when the operation completes.
func (s *Span) End() {
	s.otelSpan.End()
}

// AddEvent records an event on the span with optional attributes.
func (s *Span) AddEvent(name string, attributes ...attribute.KeyValue) {
	s.otelSpan.AddEvent(name, trace.WithAttributes(attributes...))
}

// RecordError records the error as an exception event on the span.
func (s *Span) RecordError(err error, options ...trace.EventOption) {
	s.otelSpan.RecordError(err, options...)
}

// SetStatus sets the span status. Pass non-nil err to mark as error.
func (s *Span) SetStatus(err error) {
	if err != nil {
		s.otelSpan.SetStatus(codes.Error, err.Error())
	} else {
		s.otelSpan.SetStatus(codes.Ok, "")
	}
}

// SetAttributes adds key-value attributes to the span.
func (s *Span) SetAttributes(kv ...attribute.KeyValue) {
	s.otelSpan.SetAttributes(kv...)
}

const spanDelimiter = "."

// Name joins the given strings with dots to form a span name.
func Name(names ...string) string {
	return strings.Join(names, spanDelimiter)
}

// Attribute creates an attribute.KeyValue from a key and value.
func Attribute(k string, v any) attribute.KeyValue {
	return keyValue(k, v)
}

// HTTPStatusCodeAttributes returns attributes for HTTP status code per OTel conventions.
func HTTPStatusCodeAttributes(code int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int("http.response.status_code", code),
		attribute.Int("http.status_code", code), // Deprecated: SemConv <= v1.21
	}
}
