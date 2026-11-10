package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func run(out io.Writer) error {
	fmt.Fprintln(out, "== 1. A message and its binary representation ==")
	contact := &contactsv1.Contact{Id: 42, Name: "Ana López", Email: "ana@example.com", Version: "example-version"}
	data, err := proto.Marshal(contact)
	if err != nil {
		return err
	}
	decoded := new(contactsv1.Contact)
	if err := proto.Unmarshal(data, decoded); err != nil {
		return err
	}
	if !proto.Equal(contact, decoded) {
		return errors.New("round-trip changed the contact")
	}
	fmt.Fprintf(out, "Decoded #%d: %s; binary round-trip preserved the message.\n", decoded.GetId(), decoded.GetName())

	fmt.Fprintln(out, "\n== 2. ProtoJSON is a separate representation ==")
	jsonData, err := (protojson.MarshalOptions{Indent: "  "}).Marshal(contact)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(jsonData))
	fmt.Fprintln(out, "ProtoJSON writes int64 IDs as strings; REST's JSON representation uses numbers.")

	fmt.Fprintln(out, "\n== 3. Presence is different from a zero value ==")
	absent := &contactsv1.DeleteContactRequest{Id: 42}
	presentEmpty := &contactsv1.DeleteContactRequest{Id: 42, ExpectedVersion: proto.String("")}
	fmt.Fprintf(out, "Absent version: getter=%q, present=%t\n", absent.GetExpectedVersion(), absent.ExpectedVersion != nil)
	fmt.Fprintf(out, "Explicitly empty: getter=%q, present=%t\n", presentEmpty.GetExpectedVersion(), presentEmpty.ExpectedVersion != nil)
	fmt.Fprintln(out, "The getter returns the same text; presence lets the service distinguish the requests.")
	fmt.Fprintln(out, "The unary service rejects a present empty version rather than treating it as unconditional.")

	fmt.Fprintln(out, "\n== 4. A mask selects a field even when its value is empty ==")
	mask, err := fieldmaskpb.New(&contactsv1.ContactInput{}, "email")
	if err != nil {
		return err
	}
	update := &contactsv1.UpdateContactRequest{
		Id: 42, Input: &contactsv1.ContactInput{Email: ""}, UpdateMask: mask,
	}
	jsonData, err = (protojson.MarshalOptions{Indent: "  "}).Marshal(update)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(jsonData))
	fmt.Fprintln(out, "The mask selects email; its empty value requests clearing it. Other fields stay unchanged.")

	fmt.Fprintln(out, "\n== 5. Clone generated messages before independent edits ==")
	// Generated messages contain runtime state. Pass pointers and use proto.Clone
	// for an independent message, rather than copying a generated struct value.
	clone := proto.Clone(contact).(*contactsv1.Contact)
	clone.Name = "Edited copy"
	fmt.Fprintf(out, "Original: %s; clone: %s\n", contact.GetName(), clone.GetName())
	fmt.Fprintln(out, "\nThis lesson serializes messages locally. The next stage implements the generated service interfaces.")
	return nil
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Protobuf lesson:", err)
		os.Exit(1)
	}
}
