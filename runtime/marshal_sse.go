package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"google.golang.org/protobuf/encoding/protojson"
)

// SSEMarshaler is a Marshaler which encodes data in Server-Sent Events (SSE) format.
// It wraps another Marshaler to handle the actual data serialization, but formats
// the output according to the SSE specification (https://html.spec.whatwg.org/multipage/server-sent-events.html).
//
// SSE format:
//   data: {"field": "value"}
//   data: {"more": "data"}
//
//   (blank line after each event)
//
// For streaming responses, each message is formatted as an SSE event with the marshaled
// JSON data. Multi-line JSON is properly formatted with "data: " prefix on each line.
//
// Per-Stream Event IDs:
// The SSEMarshaler maintains per-goroutine event counters to ensure each stream gets
// its own ID sequence starting from 0. This works because ForwardResponseStream processes
// each stream in a dedicated goroutine. The counter is goroutine-local using a sync.Map
// keyed by goroutine ID.
type SSEMarshaler struct {
	Marshaler

	// EventType is an optional event type to include in SSE events.
	// If set, events will include "event: <EventType>\n" before the data.
	EventType string

	// IDPrefix is an optional prefix for event IDs.
	// If set, events will include "id: <IDPrefix><counter>\n" before the data.
	IDPrefix string

	// counters maps goroutine ID to event counter for per-stream ID sequences
	counters sync.Map // map[int64]*int
}

// NewSSEMarshaler creates a new SSE marshaler that wraps a JSONPb marshaler with default settings.
func NewSSEMarshaler() *SSEMarshaler {
	return &SSEMarshaler{
		Marshaler: &JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: true,
			},
			UnmarshalOptions: protojson.UnmarshalOptions{
				DiscardUnknown: true,
			},
		},
	}
}

// ContentType returns the content-type for non-streaming responses.
// SSE is only used for streaming, so this returns the underlying marshaler's content type.
func (s *SSEMarshaler) ContentType(v interface{}) string {
	return s.Marshaler.ContentType(v)
}

// StreamContentType returns "text/event-stream" for streaming responses.
// This is the standard MIME type for Server-Sent Events.
func (s *SSEMarshaler) StreamContentType(v interface{}) string {
	return "text/event-stream"
}

// Delimiter returns the SSE event delimiter (double newline).
// According to the SSE spec, events are separated by blank lines.
func (s *SSEMarshaler) Delimiter() []byte {
	return []byte("\n\n")
}

// Marshal marshals "v" into SSE format.
// This method is called by the streaming handler (ForwardResponseStream) for each message.
// It formats the marshaled data as an SSE event with optional event type and ID.
// Event IDs are maintained per-goroutine, so each stream gets its own sequence starting at 0.
func (s *SSEMarshaler) Marshal(v interface{}) ([]byte, error) {
	// Marshal the data using the underlying marshaler
	buf, err := s.Marshaler.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result bytes.Buffer

	// Write optional event type
	if s.EventType != "" {
		fmt.Fprintf(&result, "event: %s\n", s.EventType)
	}

	// Write optional event ID with per-goroutine counter
	if s.IDPrefix != "" {
		gid := getGoroutineID()

		// Get or create counter for this goroutine
		counterVal, _ := s.counters.LoadOrStore(gid, new(int))
		counter := counterVal.(*int)

		id := *counter
		*counter++

		fmt.Fprintf(&result, "id: %s%d\n", s.IDPrefix, id)
	}

	// Format the data according to SSE specification
	// Each line of data must be prefixed with "data: "
	data := string(buf)
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	for _, line := range lines {
		fmt.Fprintf(&result, "data: %s\n", line)
	}

	// Note: The delimiter (double newline) is written by the streaming handler
	// in handler.go's ForwardResponseStream function after this data
	return result.Bytes(), nil
}

// NewEncoder returns an encoder that formats output as SSE events.
// Each call to Encode() will produce a complete SSE event with proper formatting.
// Note: This is used for non-streaming responses and health checks.
func (s *SSEMarshaler) NewEncoder(w io.Writer) Encoder {
	return EncoderFunc(func(v interface{}) error {
		// Use Marshal to format as SSE, which handles event counter
		buf, err := s.Marshal(v)
		if err != nil {
			return err
		}
		_, err = w.Write(buf)
		return err
	})
}

// SSEBuiltinMarshaler is like SSEMarshaler but uses JSONBuiltin for faster JSON encoding.
// This is useful when you don't need protobuf-specific features and want better performance.
//
// Per-Stream Event IDs:
// Like SSEMarshaler, this maintains per-goroutine event counters to ensure each stream
// gets its own ID sequence starting from 0.
type SSEBuiltinMarshaler struct {
	// EventType is an optional event type to include in SSE events.
	EventType string

	// IDPrefix is an optional prefix for event IDs.
	IDPrefix string

	// counters maps goroutine ID to event counter for per-stream ID sequences
	counters sync.Map // map[int64]*int
}

// NewSSEBuiltinMarshaler creates a new SSE marshaler using the standard library's JSON encoder.
func NewSSEBuiltinMarshaler() *SSEBuiltinMarshaler {
	return &SSEBuiltinMarshaler{}
}

// ContentType returns "application/json" for non-streaming responses.
func (s *SSEBuiltinMarshaler) ContentType(v interface{}) string {
	return "application/json"
}

// StreamContentType returns "text/event-stream" for streaming responses.
func (s *SSEBuiltinMarshaler) StreamContentType(v interface{}) string {
	return "text/event-stream"
}

// Delimiter returns the SSE event delimiter (double newline).
func (s *SSEBuiltinMarshaler) Delimiter() []byte {
	return []byte("\n\n")
}

// Marshal marshals "v" into SSE format using JSON encoding.
// This method is called by the streaming handler (ForwardResponseStream) for each message.
// Event IDs are maintained per-goroutine, so each stream gets its own sequence starting at 0.
func (s *SSEBuiltinMarshaler) Marshal(v interface{}) ([]byte, error) {
	// Marshal the data to JSON
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result bytes.Buffer

	// Write optional event type
	if s.EventType != "" {
		fmt.Fprintf(&result, "event: %s\n", s.EventType)
	}

	// Write optional event ID with per-goroutine counter
	if s.IDPrefix != "" {
		gid := getGoroutineID()

		// Get or create counter for this goroutine
		counterVal, _ := s.counters.LoadOrStore(gid, new(int))
		counter := counterVal.(*int)

		id := *counter
		*counter++

		fmt.Fprintf(&result, "id: %s%d\n", s.IDPrefix, id)
	}

	// Format the data according to SSE specification
	data := string(buf)
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	for _, line := range lines {
		fmt.Fprintf(&result, "data: %s\n", line)
	}

	return result.Bytes(), nil
}

// Unmarshal unmarshals JSON "data" into "v".
func (s *SSEBuiltinMarshaler) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// NewDecoder returns a JSON decoder.
func (s *SSEBuiltinMarshaler) NewDecoder(r io.Reader) Decoder {
	return json.NewDecoder(r)
}

// NewEncoder returns an encoder that formats output as SSE events.
// Note: This is used for non-streaming responses and health checks.
func (s *SSEBuiltinMarshaler) NewEncoder(w io.Writer) Encoder {
	return EncoderFunc(func(v interface{}) error {
		// Use Marshal to format as SSE, which handles event counter
		buf, err := s.Marshal(v)
		if err != nil {
			return err
		}
		_, err = w.Write(buf)
		return err
	})
}
