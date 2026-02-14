package fscache

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerTimingHeaderString(t *testing.T) {
	description := "fetch upstream"
	start := time.Unix(100, 0)
	h := &ServerTimingHeader{
		Start: start,
		Steps: []ServerTimingSteps{
			{Name: "cache_lookup", Now: start.Add(1500 * time.Millisecond)},
			{Name: "download", Now: start.Add(2500 * time.Millisecond), Description: &description},
		},
	}

	got := h.String()
	want := `cache_lookup;dur=1500, download;dur=1000;desc="fetch upstream"`
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestServerTimingHeaderAddStepWithDescription(t *testing.T) {
	h := NewServerTimingHeader()
	h.AddStepWithDescription("refresh", "metadata check")

	if len(h.Steps) != 1 {
		t.Fatalf("steps length = %d, want 1", len(h.Steps))
	}
	if h.Steps[0].Description == nil || *h.Steps[0].Description != "metadata check" {
		t.Fatalf("expected description to be set, got %#v", h.Steps[0].Description)
	}
}

func TestServerTimingHeaderWriteToResponse(t *testing.T) {
	h := NewServerTimingHeader()
	h.AddStep("start")

	rr := httptest.NewRecorder()
	h.WriteToResponse(rr)

	if len(h.Steps) != 2 {
		t.Fatalf("steps length after WriteToResponse = %d, want 2", len(h.Steps))
	}
	if h.Steps[1].Name != "end" {
		t.Fatalf("last step = %q, want %q", h.Steps[1].Name, "end")
	}

	header := rr.Header().Get("Server-Timing")
	if header == "" {
		t.Fatalf("expected Server-Timing header to be set")
	}
	if !strings.Contains(header, "start;dur=") || !strings.Contains(header, "end;dur=") {
		t.Fatalf("unexpected Server-Timing header: %q", header)
	}
}
