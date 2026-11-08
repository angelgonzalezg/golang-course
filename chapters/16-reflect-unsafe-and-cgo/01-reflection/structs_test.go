package main

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestDescribeStruct(t *testing.T) {
	profile := Profile{Name: "Ana", Age: 30, token: "local-example"}
	want := []fieldInfo{
		{Name: "Name", Type: reflect.TypeFor[DisplayName](), Exported: true, Display: "Name", HasDisplay: true},
		{Name: "Email", Type: reflect.TypeFor[string](), Exported: true, HasDisplay: true},
		{Name: "Age", Type: reflect.TypeFor[int](), Exported: true},
		{Name: "token", Type: reflect.TypeFor[string](), Display: "Internal", HasDisplay: true},
	}
	for _, tt := range []struct {
		name  string
		input any
	}{
		{name: "value", input: profile},
		{name: "pointer", input: &profile},
		{name: "typed nil pointer", input: (*Profile)(nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := describeStruct(tt.input)
			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("metadata=(%+v, %v), want %+v", got, err, want)
			}
		})
	}
	fields, err := describeStruct(struct{}{})
	if err != nil || len(fields) != 0 {
		t.Fatalf("empty struct metadata=(%+v, %v), want no fields and nil error", fields, err)
	}
}

func TestDescribeStructRejectsOtherShapes(t *testing.T) {
	profile := &Profile{}
	for _, input := range []any{nil, 42, "name", []Profile{}, map[string]string{}, (*int)(nil), &profile} {
		fields, err := describeStruct(input)
		if fields != nil || !errors.Is(err, ErrExpectedStruct) {
			t.Fatalf("input %T: metadata=(%+v, %v), want nil and ErrExpectedStruct", input, fields, err)
		}
	}
}

func TestSetStringFieldUpdatesTheOriginal(t *testing.T) {
	profile := Profile{Name: "Ana", Email: "ana@example.com", Age: 30, token: "local-example"}
	if err := setStringField(&profile, "Name", "Grace"); err != nil {
		t.Fatal(err)
	}
	if err := setStringField(&profile, "Email", ""); err != nil {
		t.Fatal(err)
	}
	want := Profile{Name: "Grace", Age: 30, token: "local-example"}
	if profile != want {
		t.Fatalf("updated profile=%#v, want %#v", profile, want)
	}
	// The helper works on another model; it is not a switch over Profile fields.
	other := struct{ Title string }{Title: "Old"}
	if err := setStringField(&other, "Title", "New"); err != nil || other.Title != "New" {
		t.Fatalf("other model=(%+v, %v), want Title=New", other, err)
	}
}

func TestSetStringFieldRejectsWithoutMutation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		target func(*Profile) any
		field  string
		err    error
	}{
		{name: "nil interface", target: func(*Profile) any { return nil }, field: "Name", err: ErrInvalidTarget},
		{name: "typed nil", target: func(*Profile) any { return (*Profile)(nil) }, field: "Name", err: ErrInvalidTarget},
		{name: "struct copy", target: func(p *Profile) any { return *p }, field: "Name", err: ErrInvalidTarget},
		{name: "pointer to scalar", target: func(*Profile) any { return new(int) }, field: "Name", err: ErrInvalidTarget},
		{name: "two pointer levels", target: func(p *Profile) any { return &p }, field: "Name", err: ErrInvalidTarget},
		{name: "missing field", target: func(p *Profile) any { return p }, field: "Missing", err: ErrFieldNotFound},
		{name: "empty field name", target: func(p *Profile) any { return p }, err: ErrFieldNotFound},
		{name: "unexported field", target: func(p *Profile) any { return p }, field: "token", err: ErrReadOnlyField},
		{name: "wrong kind", target: func(p *Profile) any { return p }, field: "Age", err: ErrNotString},
	} {
		t.Run(tt.name, func(t *testing.T) {
			profile := Profile{Name: "Ana", Email: "ana@example.com", Age: 30, token: "local-example"}
			before := profile
			err := setStringField(tt.target(&profile), tt.field, "Replacement")
			if !errors.Is(err, tt.err) {
				t.Fatalf("error=%v, want %v", err, tt.err)
			}
			if profile != before {
				t.Fatalf("rejected operation changed data: %#v", profile)
			}
		})
	}
}

func TestEmbeddedFieldsAreNotTraversed(t *testing.T) {
	type Embedded struct{ Name string }
	type Wrapper struct {
		*Embedded
		Note string
	}
	present := &Embedded{Name: "Original"}
	for _, embedded := range []*Embedded{nil, present} {
		wrapper := Wrapper{Embedded: embedded, Note: "unchanged"}
		if err := setStringField(&wrapper, "Name", "Replacement"); !errors.Is(err, ErrFieldNotFound) {
			t.Fatalf("promoted lookup error=%v, want ErrFieldNotFound", err)
		}
		if wrapper.Embedded != embedded || wrapper.Note != "unchanged" || (embedded != nil && embedded.Name != "Original") {
			t.Fatal("rejected promoted lookup changed data")
		}
		fields, err := describeStruct(&wrapper)
		if err != nil || len(fields) != 2 || fields[0].Name != "Embedded" || fields[0].Type != reflect.TypeFor[*Embedded]() || fields[1].Name != "Note" {
			t.Fatalf("metadata must describe direct fields without flattening: %+v, %v", fields, err)
		}
	}
}
