package main

import "fmt"

// Generation runs from this package's directory. The local tool needs no download.
//go:generate go run ./internal/cmd/statusgen -in=statuses.txt -out=status_names_gen.go

// Status is an integer enum whose constants and names come from statuses.txt.
// Generated declarations are checked in; ordinary builds do not run the generator.
type Status int

func (s Status) String() string {
	if s < 0 || int(s) >= len(statusNames) {
		return fmt.Sprintf("Status(%d)", s)
	}
	return statusNames[s]
}
