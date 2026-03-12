package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

// SSEExample demonstrates how to use the SSE marshaler for streaming responses.
//
// Usage:
//
//	mux := runtime.NewServeMux(
//	    runtime.WithMarshalerOption("text/event-stream", runtime.NewSSEMarshaler()),
//	)
//
// Then, clients can request SSE format by setting the Accept header:
//
//	curl -H "Accept: text/event-stream" http://localhost:8080/v1/stream
//
// The response will be formatted as Server-Sent Events:
//
//	data: {"message": "Hello"}
//
//	data: {"message": "World"}
//
// For more advanced usage with event types and IDs:
//
//	sseMarshaler := runtime.NewSSEMarshaler()
//	sseMarshaler.EventType = "message"    // Optional: adds "event: message" line
//	sseMarshaler.IDPrefix = "msg-"        // Optional: adds "id: msg-0", "id: msg-1", etc.
//	mux := runtime.NewServeMux(
//	    runtime.WithMarshalerOption("text/event-stream", sseMarshaler),
//	)
//
// This will produce:
//
//	event: message
//	id: msg-0
//	data: {"message": "Hello"}
//
//	event: message
//	id: msg-1
//	data: {"message": "World"}
//
// IMPORTANT: Event Counter Behavior
// The SSEMarshaler maintains per-goroutine event counters, which means each stream
// gets its own ID sequence starting from 0. This works because ForwardResponseStream
// processes each gRPC stream in a dedicated goroutine.
//
// Example: Two concurrent streams will produce:
// Stream 1: id: msg-0, msg-1, msg-2, ...
// Stream 2: id: msg-0, msg-1, msg-2, ... (independent sequence)
//
// This is the expected behavior for SSE where each client connection should have
// its own event ID sequence for proper reconnection handling.
func SSEExample() {
	// Create SSE marshaler with custom options
	sseMarshaler := runtime.NewSSEMarshaler()
	sseMarshaler.EventType = "update"  // Optional: event type
	sseMarshaler.IDPrefix = "evt-"     // Optional: event ID prefix

	// Create ServeMux with SSE marshaler registered
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption("text/event-stream", sseMarshaler),
	)

	// Register your gRPC service handlers
	// opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	// err := RegisterYourServiceHandler(context.Background(), mux, "localhost:9090", opts)
	// if err != nil {
	//     panic(err)
	// }

	// Start HTTP server
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Println("Server listening on :8080")
	fmt.Println("Try: curl -H 'Accept: text/event-stream' http://localhost:8080/v1/your-stream-endpoint")
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}

// SSEWithBuiltinJSON demonstrates using SSEBuiltinMarshaler for better performance.
// This is useful when you don't need protobuf-specific features.
func SSEWithBuiltinJSON() {
	sseMarshaler := runtime.NewSSEBuiltinMarshaler()
	sseMarshaler.EventType = "update"

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption("text/event-stream", sseMarshaler),
	)

	// Register handlers...
	_ = mux
}

// SSEWithMultipleMarshalers shows how to support both SSE and NDJSON.
// Clients can choose the format via the Accept header.
func SSEWithMultipleMarshalers() {
	mux := runtime.NewServeMux(
		// SSE format
		runtime.WithMarshalerOption("text/event-stream", runtime.NewSSEMarshaler()),
		// NDJSON format (default JSONPb already supports this)
		runtime.WithMarshalerOption("application/x-ndjson", &runtime.JSONPb{}),
	)

	// Now clients can choose:
	// Accept: text/event-stream     -> SSE format
	// Accept: application/x-ndjson  -> NDJSON format
	// Accept: application/json       -> Regular JSON (default)

	_ = mux
}

// MockStreamingService demonstrates a server-side streaming gRPC service
// that works well with SSE.
type MockStreamingService struct{}

// StreamMessages is an example of a server-side streaming RPC.
// When called through the gateway with SSE marshaler, each message
// will be formatted as an SSE event.
func (s *MockStreamingService) StreamMessages(
	req *StreamRequest,
	stream StreamService_StreamMessagesServer,
) error {
	ctx := stream.Context()

	for i := 0; i < 10; i++ {
		// Check if client disconnected
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Send a message
		msg := &StreamResponse{
			Message: fmt.Sprintf("Message %d", i),
			Index:   int32(i),
		}

		if err := stream.Send(msg); err != nil {
			return err
		}

		// Simulate some processing time
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// Placeholder types for the example
type StreamRequest struct{}
type StreamResponse struct {
	Message string `json:"message"`
	Index   int32  `json:"index"`
}
type StreamService_StreamMessagesServer interface {
	Send(*StreamResponse) error
	Context() context.Context
	grpc.ServerStream
}

// ClientExample shows how to consume SSE from a client.
//
// Using curl:
//
//	curl -N -H "Accept: text/event-stream" http://localhost:8080/v1/stream
//
// Using JavaScript (browser):
//
//	const eventSource = new EventSource('/v1/stream');
//	eventSource.onmessage = (event) => {
//	    const data = JSON.parse(event.data);
//	    console.log('Received:', data);
//	};
//	eventSource.addEventListener('update', (event) => {
//	    const data = JSON.parse(event.data);
//	    console.log('Update event:', data);
//	});
//	eventSource.onerror = (error) => {
//	    console.error('SSE error:', error);
//	    eventSource.close();
//	};
//
// Using Go:
//
//	req, _ := http.NewRequest("GET", "http://localhost:8080/v1/stream", nil)
//	req.Header.Set("Accept", "text/event-stream")
//	resp, _ := http.DefaultClient.Do(req)
//	defer resp.Body.Close()
//
//	scanner := bufio.NewScanner(resp.Body)
//	for scanner.Scan() {
//	    line := scanner.Text()
//	    if strings.HasPrefix(line, "data: ") {
//	        data := strings.TrimPrefix(line, "data: ")
//	        // Parse JSON data
//	        var msg map[string]interface{}
//	        json.Unmarshal([]byte(data), &msg)
//	        fmt.Println("Received:", msg)
//	    }
//	}
func ClientExample() {
	// See comments above for implementation examples
}
