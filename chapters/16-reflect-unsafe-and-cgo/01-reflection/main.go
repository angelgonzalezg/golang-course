// Reflection
// Topic: inspect runtime types and values, read tags, and mutate deliberately.
// Usage: go run ./chapters/16-reflect-unsafe-and-cgo/01-reflection
// Key idea: use typed code first; choose reflection for runtime structure.
package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
)

type DisplayName string

type Profile struct {
	Name  DisplayName `display:"Name"`
	Email string      `display:""`
	Age   int
	token string `display:"Internal"`
}

func (p Profile) String() string { return string(p.Name) }

func showTypes() {
	fmt.Println("== 1. Type identifies a type; Kind classifies it ==")
	var input any = DisplayName("Ana")
	typ := reflect.TypeOf(input)
	fmt.Printf("Dynamic type: %s; kind: %s\n", typ, typ.Kind())
	fmt.Println("DisplayName is string:", typ == reflect.TypeFor[string]())
	fmt.Println("DisplayName has string kind:", typ.Kind() == reflect.String)
	// TypeFor obtains even an interface type without needing a sample value.
	fmt.Println("TypeFor[any] kind:", reflect.TypeFor[any]().Kind())
	fmt.Println("Profile implements fmt.Stringer:", reflect.TypeFor[Profile]().Implements(reflect.TypeFor[fmt.Stringer]()))
}

func showNilValues() {
	fmt.Println("\n== 2. Invalid values differ from typed nil pointers ==")
	var absent any
	invalid := reflect.ValueOf(absent)
	fmt.Println("TypeOf(nil) is nil:", reflect.TypeOf(absent) == nil)
	fmt.Println("ValueOf(nil) is valid:", invalid.IsValid())
	fmt.Println("ValueOf(nil) kind:", invalid.Kind()) // Kind is safe on an invalid Value.

	var pointer *Profile
	var boxed any = pointer
	fmt.Println("Interface holding *Profile(nil) is nil:", boxed == nil)
	value := reflect.ValueOf(boxed)
	if value.IsValid() && value.Kind() == reflect.Pointer {
		// IsNil requires a nil-capable kind; it is not a universal nil check.
		fmt.Println("Reflected pointer is nil:", value.IsNil())
		fmt.Println("Nil pointer's Elem is valid:", value.Elem().IsValid())
	}
}

func showAddressability() {
	fmt.Println("\n== 3. A struct copy and an addressable original ==")
	profile := Profile{Name: "Ana", token: "local-example"}
	copyValue := reflect.ValueOf(profile)
	original := reflect.ValueOf(&profile).Elem()
	fmt.Println("ValueOf(profile) can address/set:", copyValue.CanAddr(), copyValue.CanSet())
	fmt.Println("ValueOf(&profile).Elem() can address/set:", original.CanAddr(), original.CanSet())
	name := original.FieldByName("Name")
	if name.CanSet() && name.Kind() == reflect.String {
		name.SetString("Ana Updated")
	}
	fmt.Println("Original name after SetString:", profile.Name)
	private := original.FieldByName("token")
	fmt.Println("Private field can address/set/interface:", private.CanAddr(), private.CanSet(), private.CanInterface())
	// Addressable alone does not mean settable. Reflection respects unexported
	// fields even within this package; do not call private.Interface() here.
}

func showMetadataAndUpdates() error {
	fmt.Println("\n== 4. Struct tags are type metadata ==")
	var absent *Profile
	fields, err := describeStruct(absent)
	if err != nil {
		return err
	}
	for _, field := range fields {
		fmt.Printf("%s: type=%s exported=%t display=%q present=%t\n", field.Name, field.Type, field.Exported, field.Display, field.HasDisplay)
	}

	fmt.Println("\n== 5. Validate a runtime field selection before writing ==")
	profile := Profile{Name: "Ana", Email: "ana@example.com", Age: 30, token: "local-example"}
	if err := setStringField(&profile, "Name", "Grace"); err != nil {
		return err
	}
	fmt.Println("Updated defined string field:", profile.Name)
	fmt.Println("Missing field rejected:", errors.Is(setStringField(&profile, "Missing", "x"), ErrFieldNotFound))
	fmt.Println("Non-string field rejected:", errors.Is(setStringField(&profile, "Age", "31"), ErrNotString))
	fmt.Println("Unexported field rejected:", errors.Is(setStringField(&profile, "token", "x"), ErrReadOnlyField))
	fmt.Println("Struct value rejected:", errors.Is(setStringField(profile, "Name", "x"), ErrInvalidTarget))
	// When the model and field are known, this expresses the intent directly.
	profile.Name = "Direct Assignment"
	fmt.Println("Typed assignment:", profile.Name)
	return nil
}

func showSharedCollections() {
	fmt.Println("\n== 6. Reflected slices and maps still share data ==")
	names := []string{"Ana"}
	view := reflect.ValueOf(names)
	fmt.Println("Slice descriptor can set:", view.CanSet())
	fmt.Println("Slice element can set:", view.Index(0).CanSet())
	view.Index(0).SetString("Grace") // The copied slice descriptor refers to the same array.
	fmt.Println("Caller slice:", names)

	settings := map[string]string{"language": "Go"}
	mapValue := reflect.ValueOf(settings)
	key := reflect.ValueOf("language")
	fmt.Println("Map lookup result can set:", mapValue.MapIndex(key).CanSet())
	mapValue.SetMapIndex(key, reflect.ValueOf("Golang"))
	fmt.Println("Caller map entry:", settings["language"])
	fmt.Println("Missing map entry is valid:", mapValue.MapIndex(reflect.ValueOf("missing")).IsValid())
}

func showRuntimeConstruction() error {
	fmt.Println("\n== 7. Construct values from runtime types ==")
	created := reflect.New(reflect.TypeFor[Profile]()) // A Value containing *Profile.
	if err := setStringField(created.Interface(), "Name", "New Profile"); err != nil {
		return err
	}
	fmt.Println("Constructed pointer type:", created.Type())
	fmt.Println("Constructed name:", created.Elem().FieldByName("Name").String())
	sliceType := reflect.SliceOf(reflect.TypeFor[string]())
	list := reflect.MakeSlice(sliceType, 0, 2)
	list = reflect.Append(list, reflect.ValueOf("Go"), reflect.ValueOf("Reflection"))
	fmt.Println("Constructed slice:", list.Interface())
	// Use new(Profile) and []string directly when those types are known in code.
	return nil
}

func showCallsAndEquality() {
	fmt.Println("\n== 8. Dynamic calls and typed alternatives ==")
	function := reflect.ValueOf(strings.ToUpper)
	// This function and argument have a known matching signature. A dynamic
	// dispatcher would need to validate function kind, nil, arity, and argument
	// assignability before Call. Panics inside the called function can still occur.
	results := function.Call([]reflect.Value{reflect.ValueOf("Go")})
	fmt.Println("Reflective call:", results[0].String())
	fmt.Println("Direct call:", strings.ToUpper("Go"))
	var printable fmt.Stringer = Profile{Name: "Interface Call"}
	fmt.Println("Known behavior through an interface:", printable.String())

	fmt.Println("\n== 9. Choose equality semantics deliberately ==")
	var missing []string
	empty := []string{}
	fmt.Println("DeepEqual(nil slice, empty slice):", reflect.DeepEqual(missing, empty))
	fmt.Println("slices.Equal(nil slice, empty slice):", slices.Equal(missing, empty))
	// Use == for comparable values, slices.Equal/maps.Equal for suitable element
	// types, or a domain-specific comparator when equality needs application rules.
}

func run() error {
	showTypes()
	showNilValues()
	showAddressability()
	if err := showMetadataAndUpdates(); err != nil {
		return err
	}
	showSharedCollections()
	if err := showRuntimeConstruction(); err != nil {
		return err
	}
	showCallsAndEquality()
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
