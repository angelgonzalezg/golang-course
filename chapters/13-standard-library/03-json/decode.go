package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
)

const maxJSONBytes = 4 * 1024

var (
	ErrJSONTooLarge   = errors.New("JSON document exceeds the 4096-byte limit")
	ErrObjectRequired = errors.New("JSON object required")
)

// readTask borrows r and expects one complete, small request, then EOF. Limit
// the source before decoding; an extra byte detects overflow instead of silently
// accepting a truncated prefix. The limit includes whitespace and uses bytes.
func readTask(r io.Reader) (Task, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxJSONBytes+1))
	if err != nil {
		return Task{}, fmt.Errorf("read task JSON: %w", err)
	}
	if len(data) > maxJSONBytes {
		return Task{}, ErrJSONTooLarge
	}

	// Use Unmarshal for the bounded bytes already in memory. It expects exactly
	// one value with optional surrounding whitespace, rejecting trailing input.
	// Passing &request lets the decoder allocate an object or leave nil for null;
	// the outer pointer is required to populate this local pointer variable.
	var request *taskRequest
	if err := json.Unmarshal(data, &request, json.RejectUnknownMembers(true)); err != nil {
		return Task{}, fmt.Errorf("decode task JSON: %w", err)
	}
	if request == nil {
		return Task{}, ErrObjectRequired
	}
	// Decode into fresh local storage. No partially decoded or invalid Task is
	// exposed on failure. Decoding JSON and validating domain rules are distinct.
	return taskFromRequest(*request)
}
