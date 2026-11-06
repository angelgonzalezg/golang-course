package main

import (
	jsonv1 "encoding/json"
	json "encoding/json/v2"
	"errors"
	"slices"
	"testing"
)

func TestTaskFromRequestOwnsValuesAndAppliesDefaults(t *testing.T) {
	request := taskRequest{Title: "  Learn JSON  ", Tags: []string{"Go"}}
	task, err := taskFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Learn JSON" || !task.Notifications || task.State != StatePending {
		t.Fatalf("unexpected normalized task: %+v", task)
	}
	request.Tags[0] = "changed by caller"
	if !slices.Equal(task.Tags, []string{"Go"}) {
		t.Fatal("Task retained the caller's mutable tag storage")
	}
	task.Tags[0] = "changed by task"
	if request.Tags[0] != "changed by caller" {
		t.Fatal("Task mutation changed the request")
	}

	disabled := false
	task, err = taskFromRequest(taskRequest{Title: "Explicit choice", Notifications: &disabled})
	if err != nil || task.Notifications || task.Tags == nil || len(task.Tags) != 0 {
		t.Fatalf("explicit false/empty tags not preserved: %+v, %v", task, err)
	}
	disabled = true
	if task.Notifications {
		t.Fatal("Task must store a bool value, not retain the request's pointer")
	}
}

func TestTaskStateWireContractWorksWithBothAPIs(t *testing.T) {
	codecs := []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"v1", jsonv1.Marshal, jsonv1.Unmarshal},
		{"v2", func(v any) ([]byte, error) { return json.Marshal(v) }, func(data []byte, v any) error { return json.Unmarshal(data, v) }},
	}
	for _, codec := range codecs {
		t.Run(codec.name, func(t *testing.T) {
			for _, tt := range []struct {
				state TaskState
				wire  string
			}{{StatePending, `"pending"`}, {StateDone, `"done"`}} {
				data, err := codec.encode(tt.state)
				if err != nil || string(data) != tt.wire {
					t.Fatalf("encode %d = (%s, %v), want %s", tt.state, data, err, tt.wire)
				}
				var decoded TaskState
				if err := codec.decode(data, &decoded); err != nil || decoded != tt.state {
					t.Fatalf("decode %s = (%d, %v), want %d", data, decoded, err, tt.state)
				}
			}
			if _, err := codec.encode(TaskState(99)); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("invalid enum encoding lost its cause: %v", err)
			}
			for _, input := range []string{`"unknown"`, `"DONE"`, `""`, `null`, `42`, `{}`} {
				state := StateDone
				if err := codec.decode([]byte(input), &state); err == nil || state != StateDone {
					t.Fatalf("%s: got state %d, err=%v; want unchanged state and an error", input, state, err)
				}
			}
			state := StateDone
			if err := codec.decode([]byte(`"unknown"`), &state); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("invalid enum decoding lost its cause: %v", err)
			}
		})
	}
}

func TestTaskOutputKeepsExplicitFalseAndEmptyArray(t *testing.T) {
	task, err := taskFromRequest(taskRequest{Title: "Send response", Notifications: new(bool)})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(&task)
	if err != nil {
		t.Fatal(err)
	}
	// Compare members semantically, independent of JSON object member ordering.
	var fields map[string]jsonv1.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["notifications"]) != "false" || string(fields["tags"]) != "[]" || string(fields["state"]) != `"pending"` {
		t.Fatalf("unexpected response representation: %s", data)
	}
}
