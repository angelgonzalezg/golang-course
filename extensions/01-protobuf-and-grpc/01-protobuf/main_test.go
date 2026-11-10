package main

import (
	"bytes"
	json "encoding/json/v2"
	"testing"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func TestBinaryRoundTripAndIndependentClone(t *testing.T) {
	original := &contactsv1.Contact{Id: 42, Name: "Ana López", Email: "ana@example.com", Version: "opaque-version"}
	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(contactsv1.Contact)
	if err := proto.Unmarshal(data, decoded); err != nil || !proto.Equal(original, decoded) {
		t.Fatalf("binary round-trip = %v; equal=%t", err, proto.Equal(original, decoded))
	}
	clone := proto.Clone(original).(*contactsv1.Contact)
	clone.Name = "Edited copy"
	if original.GetName() != "Ana López" || proto.Equal(original, clone) {
		t.Fatal("clone edits must be independent")
	}
}

func TestOptionalPresenceSurvivesTheWire(t *testing.T) {
	absent := &contactsv1.DeleteContactRequest{Id: 42}
	present := &contactsv1.DeleteContactRequest{Id: 42, ExpectedVersion: proto.String("")}
	encode := func(message proto.Message) []byte {
		t.Helper()
		data, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if absent.GetExpectedVersion() != present.GetExpectedVersion() || bytes.Equal(encode(absent), encode(present)) {
		t.Fatal("presence must distinguish absent and explicitly empty despite equal getter values")
	}
	for _, original := range []*contactsv1.DeleteContactRequest{absent, present} {
		decoded := new(contactsv1.DeleteContactRequest)
		if err := proto.Unmarshal(encode(original), decoded); err != nil {
			t.Fatal(err)
		}
		if (decoded.ExpectedVersion == nil) != (original.ExpectedVersion == nil) || !proto.Equal(decoded, original) {
			t.Fatal("optional presence was lost")
		}
	}
}

func TestMaskedClearSurvivesBinaryAndJSON(t *testing.T) {
	mask, err := fieldmaskpb.New(&contactsv1.ContactInput{}, "email")
	if err != nil {
		t.Fatal(err)
	}
	message := &contactsv1.UpdateContactRequest{Id: 42, Input: &contactsv1.ContactInput{}, UpdateMask: mask}
	data, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(contactsv1.UpdateContactRequest)
	if err := proto.Unmarshal(data, decoded); err != nil || !proto.Equal(message, decoded) {
		t.Fatalf("masked zero value did not survive binary encoding: %v", err)
	}
	jsonData, err := protojson.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(jsonData, &object); err != nil {
		t.Fatal(err)
	}
	if object["updateMask"] != "email" || object["id"] != "42" {
		t.Fatalf("unexpected ProtoJSON mapping: %s", jsonData)
	}
	fromJSON := new(contactsv1.UpdateContactRequest)
	if err := protojson.Unmarshal(jsonData, fromJSON); err != nil || !proto.Equal(message, fromJSON) {
		t.Fatalf("masked clear did not survive ProtoJSON: %v", err)
	}
	if _, err := fieldmaskpb.New(&contactsv1.ContactInput{}, "id"); err == nil {
		t.Fatal("server-owned ID is not a writable input field")
	}
}

func TestBinaryCompatibilityFixtureAndUnknownFieldRetention(t *testing.T) {
	// Field 1, varint 150. This fixed wire fixture protects the existing ID
	// number/type against an accidental incompatible contract change.
	wire := []byte{0x08, 0x96, 0x01}
	request := new(contactsv1.GetContactRequest)
	if err := proto.Unmarshal(wire, request); err != nil || request.GetId() != 150 {
		t.Fatalf("existing wire fixture changed: id=%d, error=%v", request.GetId(), err)
	}
	unknown := protowire.AppendTag(nil, 100, protowire.BytesType)
	unknown = protowire.AppendString(unknown, "future field")
	wire = append(wire, unknown...)
	if err := proto.Unmarshal(wire, request); err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	again := new(contactsv1.GetContactRequest)
	if err := proto.Unmarshal(encoded, again); err != nil || !bytes.Equal(again.ProtoReflect().GetUnknown(), unknown) {
		t.Fatalf("unknown binary field was not preserved: %v", err)
	}
	jsonData, err := protojson.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON := new(contactsv1.GetContactRequest)
	if err := protojson.Unmarshal(jsonData, fromJSON); err != nil {
		t.Fatal(err)
	}
	if len(fromJSON.ProtoReflect().GetUnknown()) != 0 {
		t.Fatal("ProtoJSON cannot represent unknown binary fields")
	}
}

func TestDecodeSyntaxAndBusinessValidationAreSeparate(t *testing.T) {
	// An invalid application ID is nevertheless a valid protobuf int64.
	data, err := proto.Marshal(&contactsv1.GetContactRequest{Id: -1})
	if err != nil {
		t.Fatal(err)
	}
	request := new(contactsv1.GetContactRequest)
	if err := proto.Unmarshal(data, request); err != nil || request.GetId() != -1 {
		t.Fatalf("decoding unexpectedly applied contact validation: %v", err)
	}
	if err := proto.Unmarshal([]byte{0x08, 0x80}, request); err == nil {
		t.Fatal("truncated varint must fail decoding")
	}
	if _, err := proto.Marshal(&contactsv1.ContactInput{Name: "\xff"}); err == nil {
		t.Fatal("proto3 strings must contain valid UTF-8")
	}
	if err := protojson.Unmarshal([]byte(`{"id":"42","futureField":true}`), request); err == nil {
		t.Fatal("ProtoJSON rejects unknown members by default")
	}
}

func TestServiceIdentityAndRetiredFieldGuard(t *testing.T) {
	if contactsv1.ContactService_GetContact_FullMethodName != "/contacts.v1.ContactService/GetContact" {
		t.Fatal("RPC identity changed; existing clients use this service and method name")
	}
	descriptor := (&contactsv1.Contact{}).ProtoReflect().Descriptor()
	if !descriptor.ReservedRanges().Has(6) || !descriptor.ReservedNames().Has("legacy_notes") {
		t.Fatal("retired field number and name must remain unavailable for reuse")
	}
}
