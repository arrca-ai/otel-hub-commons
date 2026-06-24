// SPDX-License-Identifier: Apache-2.0
package bus

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestSpanRoundTrip(t *testing.T) {
	in := &Span{
		TraceId: "t", SpanId: "s", ParentId: "p", Name: "GET /x", Kind: "SERVER",
		StartNano: 1_000_000_001, EndNano: 2_000_000_002, StatusCode: "ERROR",
		Attrs:         map[string]string{"http.route": "/x"},
		ResourceAttrs: map[string]string{"service.name": "svc"},
	}
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Span
	if err := proto.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Name != "GET /x" {
		t.Fatalf("Name mismatch: got %q", out.Name)
	}
	if out.StatusCode != "ERROR" {
		t.Fatalf("StatusCode mismatch: got %q", out.StatusCode)
	}
	if out.Attrs["http.route"] != "/x" {
		t.Fatalf("Attrs[http.route] mismatch: got %q", out.Attrs["http.route"])
	}
	if out.StartNano != in.StartNano {
		t.Fatalf("StartNano mismatch: want %d got %d", in.StartNano, out.StartNano)
	}
	if out.EndNano != in.EndNano {
		t.Fatalf("EndNano mismatch: want %d got %d", in.EndNano, out.EndNano)
	}
	if out.ResourceAttrs["service.name"] != "svc" {
		t.Fatalf("ResourceAttrs[service.name] mismatch: got %q", out.ResourceAttrs["service.name"])
	}
}

func TestCanonicalizedTraceRoundTrip(t *testing.T) {
	in := &CanonicalizedTrace{
		RootHash: "roothash-abc",
		Trace: &AssembledTrace{
			TraceId:   "trace-1",
			Connected: true,
			Spans: []*Span{
				{SpanId: "r", Name: "root"},
				{SpanId: "c", ParentId: "r", Name: "child"},
			},
		},
		SpanNodeHashes: map[string]string{"r": "h-root", "c": "h-child"},
	}
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out CanonicalizedTrace
	if err := proto.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RootHash != "roothash-abc" {
		t.Fatalf("RootHash mismatch: got %q", out.RootHash)
	}
	if out.GetTrace().GetTraceId() != "trace-1" {
		t.Fatalf("Trace.TraceId mismatch: got %q", out.GetTrace().GetTraceId())
	}
	if len(out.GetTrace().GetSpans()) != 2 {
		t.Fatalf("len(Trace.Spans) mismatch: want 2 got %d", len(out.GetTrace().GetSpans()))
	}
	if out.SpanNodeHashes["r"] != "h-root" || out.SpanNodeHashes["c"] != "h-child" {
		t.Fatalf("SpanNodeHashes mismatch: got %v", out.SpanNodeHashes)
	}
}

func TestAssembledTraceRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		connected bool
	}{
		{"connected", true},
		{"disconnected", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := &AssembledTrace{
				TraceId:   "trace-abc-123",
				Connected: tc.connected,
				Spans: []*Span{
					{
						SpanId: "span-001",
						Name:   "root-op",
					},
					{
						SpanId: "span-002",
						Name:   "child-op",
					},
				},
			}
			b, err := proto.Marshal(in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var out AssembledTrace
			if err := proto.Unmarshal(b, &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if out.TraceId != "trace-abc-123" {
				t.Fatalf("TraceId mismatch: got %q", out.TraceId)
			}
			if out.Connected != tc.connected {
				t.Fatalf("Connected mismatch: want %v got %v", tc.connected, out.Connected)
			}
			if len(out.Spans) != 2 {
				t.Fatalf("len(Spans) mismatch: want 2 got %d", len(out.Spans))
			}
			if out.Spans[0].SpanId != "span-001" {
				t.Fatalf("Spans[0].SpanId mismatch: got %q", out.Spans[0].SpanId)
			}
			if out.Spans[0].Name != "root-op" {
				t.Fatalf("Spans[0].Name mismatch: got %q", out.Spans[0].Name)
			}
		})
	}
}

func TestSpanStatusMessageAndEventsRoundTrip(t *testing.T) {
	in := &Span{
		TraceId: "t", SpanId: "s", Name: "POST /pay", Kind: "CLIENT",
		StatusCode:    "ERROR",
		StatusMessage: "boom",
		Attrs:         map[string]string{"http.request.method": "POST"},
		Events: []*SpanEvent{
			{
				Name:     "exception",
				TimeNano: 1_700_000_000_000,
				Attrs: map[string]string{
					"exception.type":    "NullPointerException",
					"exception.message": "npe",
				},
			},
			{Name: "retry", TimeNano: 1_700_000_000_500},
		},
	}
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Span
	if err := proto.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.StatusMessage != "boom" {
		t.Fatalf("StatusMessage mismatch: got %q", out.StatusMessage)
	}
	if len(out.Events) != 2 {
		t.Fatalf("len(Events) mismatch: want 2 got %d", len(out.Events))
	}
	if out.Events[0].Name != "exception" {
		t.Fatalf("Events[0].Name mismatch: got %q", out.Events[0].Name)
	}
	if out.Events[0].TimeNano != 1_700_000_000_000 {
		t.Fatalf("Events[0].TimeNano mismatch: got %d", out.Events[0].TimeNano)
	}
	if out.Events[0].Attrs["exception.type"] != "NullPointerException" {
		t.Fatalf("Events[0].Attrs[exception.type] mismatch: got %q", out.Events[0].Attrs["exception.type"])
	}
	// pre-existing fields unaffected
	if out.StatusCode != "ERROR" || out.Attrs["http.request.method"] != "POST" {
		t.Fatalf("pre-existing fields altered: status=%q attrs=%v", out.StatusCode, out.Attrs)
	}
}
