package fscache

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ServerTimingHeader struct {
	Start time.Time
	Steps []ServerTimingSteps
}

type ServerTimingSteps struct {
	Name        string
	Now         time.Time
	Description *string
}

// NewServerTimingHeader creates a new ServerTimingHeader with the current time as start.
func NewServerTimingHeader() *ServerTimingHeader {
	return &ServerTimingHeader{
		Start: time.Now(),
		Steps: []ServerTimingSteps{},
	}
}

// AddStep adds a new step to the ServerTimingHeader.
func (h *ServerTimingHeader) AddStep(name string) {
	h.Steps = append(h.Steps, ServerTimingSteps{
		Name: name,
		Now:  time.Now(),
	})
}

// AddStepWithDescription adds a new step with description to the ServerTimingHeader.
func (h *ServerTimingHeader) AddStepWithDescription(name string, description string) {
	h.Steps = append(h.Steps, ServerTimingSteps{
		Name:        name,
		Now:         time.Now(),
		Description: &description,
	})
}

// String generates the Server-Timing header value.
func (h *ServerTimingHeader) String() string {
	// Generate the Server-Timing header value
	var parts []string
	for i, step := range h.Steps {
		// If it's the first step, duration is from start to step.Now
		// Otherwise, duration is from previous step to step.Now
		var duration int64
		if i == 0 {
			duration = step.Now.Sub(h.Start).Milliseconds()
		} else {
			duration = step.Now.Sub(h.Steps[i-1].Now).Milliseconds()
		}

		if step.Description != nil {
			parts = append(parts,
				fmt.Sprintf("%s;dur=%d;desc=\"%s\"", step.Name, duration, *step.Description))
		} else {
			parts = append(parts,
				fmt.Sprintf("%s;dur=%d", step.Name, duration))
		}
	}

	return strings.Join(parts, ", ")
}

// WriteToResponse writes the Server-Timing header to the given http.ResponseWriter.
func (h *ServerTimingHeader) WriteToResponse(w http.ResponseWriter) {
	// Add the end step
	h.AddStep("end")

	// Set the Server-Timing header
	w.Header().Set("Server-Timing", h.String())
}
