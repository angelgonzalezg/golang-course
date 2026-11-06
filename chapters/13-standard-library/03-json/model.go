package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidTask  = errors.New("invalid task")
	ErrInvalidState = errors.New("invalid task state")
)

type TaskState uint8

const (
	StatePending TaskState = iota
	StateDone
)

// MarshalJSON uses a value receiver because encoding only reads the state.
// Use custom encoding when a type's wire representation differs from its Go
// representation; tags alone cannot turn this numeric enum into named strings.
func (state TaskState) MarshalJSON() ([]byte, error) {
	switch state {
	case StatePending:
		return json.Marshal("pending")
	case StateDone:
		return json.Marshal("done")
	default:
		return nil, fmt.Errorf("encode state %d: %w", state, ErrInvalidState)
	}
}

// UnmarshalJSON needs a pointer receiver to update the destination. Decode into
// a temporary value and validate before assignment, preserving this state on
// failure. An enclosing struct can still be partially changed by a decoder.
func (state *TaskState) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return fmt.Errorf("decode state: %w", err)
	}
	var parsed TaskState
	switch name {
	case "pending":
		parsed = StatePending
	case "done":
		parsed = StateDone
	default:
		return fmt.Errorf("decode state %q: %w", name, ErrInvalidState)
	}
	*state = parsed
	return nil
}

// taskRequest is the incoming JSON shape, separate from the resulting Task.
type taskRequest struct {
	Title string `json:"title"`
	// A value bool is enough when absent and false mean the same thing. Here
	// a pointer distinguishes explicit false from the default. Missing and null
	// both mean "use the default"; this is not a three-state PATCH field.
	Notifications *bool    `json:"notifications,omitzero"`
	Tags          []string `json:"tags,omitempty"`
}

type Task struct {
	Title         string    `json:"title"`
	Notifications bool      `json:"notifications"` // Always represent false explicitly.
	Tags          []string  `json:"tags"`
	State         TaskState `json:"state"`
}

func taskFromRequest(request taskRequest) (Task, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return Task{}, fmt.Errorf("title must not be blank: %w", ErrInvalidTask)
	}
	notifications := true
	if request.Notifications != nil {
		notifications = *request.Notifications
	}
	// Task owns an independent slice. make also normalizes absent/null tags to
	// an empty, non-nil slice; elements are strings, so a shallow copy suffices.
	tags := make([]string, len(request.Tags))
	copy(tags, request.Tags)
	return Task{Title: title, Notifications: notifications, Tags: tags, State: StatePending}, nil
}
