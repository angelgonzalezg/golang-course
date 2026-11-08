package main

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrExpectedStruct = errors.New("expected a struct or one pointer to a struct")
	ErrInvalidTarget  = errors.New("expected a non-nil pointer to a struct")
	ErrFieldNotFound  = errors.New("direct field not found")
	ErrReadOnlyField  = errors.New("field cannot be set")
	ErrNotString      = errors.New("field must have string kind")
)

type fieldInfo struct {
	Name       string
	Type       reflect.Type
	Exported   bool
	Display    string
	HasDisplay bool
}

// describeStruct inspects direct fields in declaration order, without reading
// their values. A typed nil *Struct is enough: metadata belongs to the type.
// It accepts one pointer level and reports embedded fields without flattening.
func describeStruct(input any) ([]fieldInfo, error) {
	typ := reflect.TypeOf(input)
	if typ == nil {
		return nil, ErrExpectedStruct
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil, ErrExpectedStruct
	}

	fields := make([]fieldInfo, 0, typ.NumField())
	for index := range typ.NumField() {
		field := typ.Field(index)
		display, present := field.Tag.Lookup("display")
		fields = append(fields, fieldInfo{
			Name:       field.Name,
			Type:       field.Type,
			Exported:   field.IsExported(),
			Display:    display,
			HasDisplay: present,
		})
	}
	return fields, nil
}

// setStringField changes one direct, exported string field in the caller's
// existing struct. Choose it when the field name is determined at runtime.
// For a known model and field, ordinary assignment or a typed setter is clearer.
// Defined string types are accepted; nested and promoted fields are excluded.
// Every validation happens before the single write, so failures leave data intact.
func setStringField(target any, name, text string) error {
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return ErrInvalidTarget
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return ErrInvalidTarget
	}

	fieldType, exists := value.Type().FieldByName(name)
	if !exists || len(fieldType.Index) != 1 {
		// A promoted field can cross a nil embedded pointer. Restrict lookup to
		// direct fields rather than traversing an object graph implicitly.
		return fmt.Errorf("%w: %q", ErrFieldNotFound, name)
	}
	field := value.Field(fieldType.Index[0])
	if !field.CanSet() {
		return fmt.Errorf("%w: %q", ErrReadOnlyField, name)
	}
	if field.Kind() != reflect.String {
		return fmt.Errorf("%w: %q has kind %s", ErrNotString, name, field.Kind())
	}
	field.SetString(text)
	return nil
}
