// Package sqllab records generation tooling for the optional SQL lab.
// Ordinary lessons compile the checked-in bindings without running sqlc.
package sqllab

//go:generate go run ./internal/cmd/sqlgen
