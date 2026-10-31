package main

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDecodeProfileDestination(t *testing.T) {
	const input = `{"name":"Mia","age":27}`
	var profile Profile
	if err := decodeProfile(input, &profile); err != nil {
		t.Fatal(err)
	}
	if profile != (Profile{Name: "Mia", Age: 27}) {
		t.Fatalf("decoded profile = %+v", profile)
	}

	tests := []struct {
		name        string
		destination any
	}{
		{name: "struct value", destination: profile},
		{name: "nil pointer", destination: (*Profile)(nil)},
		{name: "nil interface"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := decodeProfile(input, tt.destination)
			var invalid *json.InvalidUnmarshalError
			if !errors.As(err, &invalid) {
				t.Fatalf("got %v, want *json.InvalidUnmarshalError", err)
			}
		})
	}
}

func TestDecodeOptionalProfileFields(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPresent  bool
		wantNickname string
		wantAge      int
	}{
		{name: "omitted", input: `{"name":"Guest"}`},
		{name: "null", input: `{"nickname":null,"age":null}`},
		{name: "explicit zero values", input: `{"nickname":"","age":0}`, wantPresent: true},
		{name: "nonzero values", input: `{"nickname":"Mia","age":27}`, wantPresent: true, wantNickname: "Mia", wantAge: 27},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var profile APIProfile
			if err := decodeProfile(tt.input, &profile); err != nil {
				t.Fatal(err)
			}
			if !tt.wantPresent {
				if profile.Nickname != nil || profile.Age != nil {
					t.Fatalf("expected nil optional fields, got %+v", profile)
				}
				return
			}
			if profile.Nickname == nil || profile.Age == nil {
				t.Fatal("explicit values must produce non-nil pointers")
			}
			if *profile.Nickname != tt.wantNickname || *profile.Age != tt.wantAge {
				t.Errorf("decoded nickname=%q age=%d, want nickname=%q age=%d", *profile.Nickname, *profile.Age, tt.wantNickname, tt.wantAge)
			}
		})
	}
}

func TestDecodeOptionalFieldsIntoReusedProfile(t *testing.T) {
	nickname, age := "Mia", 27
	profile := APIProfile{Nickname: &nickname, Age: &age}
	if err := decodeProfile(`{"name":"Updated"}`, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Nickname != &nickname || profile.Age != &age {
		t.Fatal("omitted fields must keep the existing pointers")
	}
	if err := decodeProfile(`{"nickname":null,"age":null}`, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Nickname != nil || profile.Age != nil {
		t.Fatal("null must clear existing pointer fields")
	}
}
