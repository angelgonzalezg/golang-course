// JSON
// Topic: map Go types to JSON, choose field semantics, and validate input.
// Usage: go run ./chapters/13-standard-library/03-json
package main

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func showStructs() error {
	fmt.Println("== 1. Exported fields, tags, and typed destinations ==")
	type profile struct {
		Name      string    `json:"name"`
		Email     string    `json:"email,omitempty"`
		CreatedAt time.Time `json:"created_at"`
		Secret    string    `json:"-"` // Excluded from this wire representation.
		internal  string    // Unexported fields are not JSON fields.
	}
	person := profile{
		Name: "Ana", CreatedAt: time.Date(2024, time.July, 1, 12, 0, 0, 0, time.UTC),
		Secret: "example only", internal: "local metadata",
	}
	// Marshal accepts values and pointers. A pointer provides v2 an addressable
	// value, as recommended by its API; it does not transfer ownership.
	data, err := json.Marshal(&person)
	if err != nil {
		return err
	}
	fmt.Println("encoded profile:", string(data))
	var decoded profile
	// Unmarshal requires a non-nil pointer so it can populate the destination.
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	fmt.Println("decoded name:", decoded.Name)
	fmt.Println("decoded timestamp:", decoded.CreatedAt.Format(time.RFC3339))

	fmt.Println("\n== 2. Omission rules depend on the JSON API ==")
	disabled := false
	options := struct {
		Enabled   bool      `json:"enabled,omitempty"`
		Count     int       `json:"count,omitzero"`
		Explicit  *bool     `json:"explicit,omitzero"`
		EmptyTime time.Time `json:"empty_time,omitempty"`
		ZeroTime  time.Time `json:"zero_time,omitzero"`
	}{Explicit: &disabled}
	// In v1, omitempty omits false. In v2 it follows the JSON representation:
	// false is not an empty JSON value. omitzero uses Go's zero value/IsZero.
	oldData, err := jsonv1.Marshal(&options)
	if err != nil {
		return err
	}
	newData, err := json.Marshal(&options)
	if err != nil {
		return err
	}
	fmt.Println("v1 omission:", string(oldData))
	fmt.Println("v2 omission:", string(newData))

	fmt.Println("\n== 3. Nil collections and binary data ==")
	collections := struct {
		Items  []string          `json:"items"`
		Labels map[string]string `json:"labels"`
		Data   []byte            `json:"data"`
	}{Data: []byte("Go")}
	oldData, err = jsonv1.Marshal(&collections)
	if err != nil {
		return err
	}
	newData, err = json.Marshal(&collections)
	if err != nil {
		return err
	}
	fmt.Println("v1 nil collections:", string(oldData))
	fmt.Println("v2 nil collections:", string(newData))
	fmt.Println("[]byte represents binary data and encodes as a base64 string.")
	return nil
}

func showRequests() error {
	fmt.Println("\n== 4. Defaults and explicit false are different ==")
	for _, input := range []string{
		`{"title":"  Learn JSON  "}`,
		`{"title":"Learn JSON","notifications":false}`,
		`{"title":"Learn JSON","notifications":null}`,
	} {
		task, err := readTask(strings.NewReader(input))
		if err != nil {
			return err
		}
		data, err := json.Marshal(&task)
		if err != nil {
			return err
		}
		fmt.Println("validated task:", string(data))
	}

	fmt.Println("\n== 5. JSON decoding and domain validation ==")
	for _, input := range []string{
		`{"title":"ok","unexpected":true}`,
		`{"Title":"wrong case"}`,
		`{"title":"first","title":"duplicate"}`,
		`{"title":"ok"} {"title":"extra document"}`,
		`null`,
		`{"title":"  "}`,
	} {
		if _, err := readTask(strings.NewReader(input)); err == nil {
			return fmt.Errorf("expected rejection for %s", input)
		} else {
			fmt.Printf("rejected %s\n", input)
		}
	}
	_, err := readTask(strings.NewReader(`{"title":42}`))
	var semanticErr *json.SemanticError
	if !errors.As(err, &semanticErr) {
		return fmt.Errorf("expected a JSON type error, got %v", err)
	}
	fmt.Println("wrapped type error at:", semanticErr.JSONPointer)
	return nil
}

func showNumbers() error {
	fmt.Println("\n== 6. Dynamic numbers can lose integer precision ==")
	const input = `{"id":9007199254740993}`
	// Use typed structs when the schema is known. Both APIs decode numbers into
	// float64 inside a fresh any, so a large integer can be rounded there.
	var dynamic map[string]any
	if err := json.Unmarshal([]byte(input), &dynamic); err != nil {
		return err
	}
	fmt.Printf("dynamic value: %.0f\n", dynamic["id"])
	var typed struct {
		ID int64 `json:"id,string"` // Quoting also preserves this ID for float-based clients.
	}
	// This input has a numeric ID, so decode with a numeric field first.
	var numeric struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(input), &numeric); err != nil {
		return err
	}
	typed.ID = numeric.ID
	data, err := json.Marshal(&typed)
	if err != nil {
		return err
	}
	fmt.Println("typed int64:", numeric.ID)
	fmt.Println("quoted wire ID:", string(data))

	// When maintaining a v1 Decoder with dynamic input, UseNumber preserves
	// number text. Int64 still needs checking: fractions and overflow can fail.
	decoder := jsonv1.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	var preserved map[string]any
	if err := decoder.Decode(&preserved); err != nil {
		return err
	}
	number, ok := preserved["id"].(jsonv1.Number)
	if !ok {
		return fmt.Errorf("expected json.Number for id")
	}
	id, err := number.Int64()
	if err != nil {
		return err
	}
	fmt.Println("v1 UseNumber and checked Int64:", id)
	return nil
}

func showCustomAndRaw() error {
	fmt.Println("\n== 7. Custom JSON methods encode domain values ==")
	state := StateDone
	data, err := json.Marshal(&state)
	if err != nil {
		return err
	}
	fmt.Println("numeric enum on the wire:", string(data))
	if err := json.Unmarshal([]byte(`"unknown"`), &state); !errors.Is(err, ErrInvalidState) {
		return fmt.Errorf("expected an invalid state, got %v", err)
	}
	fmt.Println("failed decode preserved the previous state:", state == StateDone)

	fmt.Println("\n== 8. Delay payload interpretation with raw JSON ==")
	var envelope struct {
		Kind    string         `json:"kind"`
		Payload jsontext.Value `json:"payload"` // v2 counterpart of v1 json.RawMessage.
	}
	const input = `{"kind":"task-state","payload":"done"}`
	if err := json.Unmarshal([]byte(input), &envelope, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	// Inspect the discriminator before choosing the payload's Go type. Raw JSON
	// preserves its JSON representation; it is not a quoted or base64 []byte field.
	switch envelope.Kind {
	case "task-state":
		var payload TaskState
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return err
		}
		fmt.Println("selected payload is done:", payload == StateDone)
	default:
		return fmt.Errorf("unsupported envelope kind %q", envelope.Kind)
	}
	return nil
}

func showStreams() error {
	fmt.Println("\n== 9. One JSON document and a stream of records ==")
	tasks := []Task{
		{Title: "Read", Notifications: true, Tags: []string{}, State: StatePending},
		{Title: "Practice", Notifications: false, Tags: []string{"Go"}, State: StateDone},
	}
	var single bytes.Buffer
	// MarshalWrite borrows the writer and writes one value without a newline.
	if err := json.MarshalWrite(&single, &tasks[0]); err != nil {
		return err
	}
	fmt.Println("single document has trailing newline:", bytes.HasSuffix(single.Bytes(), []byte{'\n'}))
	// UnmarshalRead consumes one complete document through EOF. For already
	// available []byte, use Unmarshal instead, as readTask does after its bound.
	var document Task
	if err := json.UnmarshalRead(&single, &document, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	fmt.Println("single document task:", document.Title)

	var stream bytes.Buffer
	encoder := jsontext.NewEncoder(&stream)
	for index := range tasks {
		// MarshalEncode writes via a shared text encoder, which terminates each
		// top-level record with a newline. v1 uses Encoder.Encode for this job.
		if err := json.MarshalEncode(encoder, &tasks[index]); err != nil {
			return err
		}
	}
	fmt.Println("stream has trailing newline:", bytes.HasSuffix(stream.Bytes(), []byte{'\n'}))
	decoder := jsontext.NewDecoder(&stream)
	count := 0
	for {
		// Fresh storage for each record prevents absent fields from retaining
		// values from the previous record. This stream is generated locally.
		var task Task
		err := json.UnmarshalDecode(decoder, &task, json.RejectUnknownMembers(true))
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("decode stream record: %w", err)
		}
		fmt.Println("stream task:", task.Title)
		count++
	}
	fmt.Println("records received:", count)
	return nil
}

func run() error {
	for _, section := range []func() error{showStructs, showRequests, showNumbers, showCustomAndRaw, showStreams} {
		if err := section(); err != nil {
			return err
		}
	}
	fmt.Println("\n== 10. JSON rules ==")
	fmt.Println("Use typed fields for known schemas; use pointers when presence changes the meaning.")
	fmt.Println("Choose omission and null behavior explicitly, and check the API version's defaults.")
	fmt.Println("Bound input, require the intended document framing, validate domain rules, and check errors.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "JSON lesson:", err)
		os.Exit(1)
	}
}
