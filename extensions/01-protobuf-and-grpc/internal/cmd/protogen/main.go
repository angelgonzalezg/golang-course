// Command protogen builds pinned Go plugins and runs the recorded protoc
// version. -check compares regenerated output without changing tracked files.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const compilerVersion = "libprotoc 35.1"

var generatedFiles = []string{
	"contacts/v1/contacts.pb.go",
	"contacts/v1/contacts_grpc.pb.go",
}

func run(check bool, include string) error {
	// Running from the module root also keeps generated source paths stable.
	if _, err := os.Stat("proto/contacts/v1/contacts.proto"); err != nil {
		return fmt.Errorf("run this command from the gRPC module root: %w", err)
	}
	compiler, err := exec.LookPath("protoc")
	if err != nil {
		return fmt.Errorf("generation needs protoc 35.1 on PATH: %w", err)
	}
	version, err := exec.Command(compiler, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect protoc: %w: %s", err, version)
	}
	if strings.TrimSpace(string(version)) != compilerVersion {
		return fmt.Errorf("generation requires %s; found %s", compilerVersion, strings.TrimSpace(string(version)))
	}
	if include == "" {
		// Resolve package-manager symlinks before locating protoc's standard
		// include directory. -include supports installations with another layout.
		realCompiler, err := filepath.EvalSymlinks(compiler)
		if err != nil {
			return err
		}
		include = filepath.Join(filepath.Dir(realCompiler), "..", "include")
	}
	if _, err := os.Stat(filepath.Join(include, "google/protobuf/field_mask.proto")); err != nil {
		return fmt.Errorf("find well-known proto definitions; supply -include with protoc's include directory: %w", err)
	}
	temporary, err := os.MkdirTemp("", "grpc-protogen-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	plugins := []struct{ name, path string }{
		{"protoc-gen-go", "google.golang.org/protobuf/cmd/protoc-gen-go"},
		{"protoc-gen-go-grpc", "google.golang.org/grpc/cmd/protoc-gen-go-grpc"},
	}
	arguments := []string{}
	for _, plugin := range plugins {
		binary := filepath.Join(temporary, plugin.name+suffix)
		// Tool directives retain these commands and their pinned module versions.
		// No installation to a global GOBIN or PATH modification is needed.
		command := exec.Command("go", "build", "-o", binary, plugin.path)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("build %s: %w", plugin.name, err)
		}
		arguments = append(arguments, "--plugin="+plugin.name+"="+binary)
	}
	output := filepath.Join(temporary, "gen")
	if err := os.Mkdir(output, 0o755); err != nil {
		return err
	}
	arguments = append(arguments,
		"--proto_path=proto", "--proto_path="+include,
		"--go_out="+output, "--go_opt=paths=source_relative",
		"--go-grpc_out="+output, "--go-grpc_opt=paths=source_relative",
		"contacts/v1/contacts.proto",
	)
	command := exec.Command(compiler, arguments...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("generate contact bindings: %w", err)
	}
	for _, relative := range generatedFiles {
		data, err := os.ReadFile(filepath.Join(output, relative))
		if err != nil {
			return err
		}
		destination := filepath.Join("gen", relative)
		if check {
			previous, err := os.ReadFile(destination)
			if err != nil {
				return fmt.Errorf("read %s: %w", destination, err)
			}
			if !bytes.Equal(data, previous) {
				return fmt.Errorf("generated output differs: %s; run go generate .", destination)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o644); err != nil {
			return err
		}
	}
	if check {
		fmt.Println("Generated Go files match the recorded compiler and pinned plugins.")
	} else {
		fmt.Println("Generated contact message types and gRPC client/server interfaces.")
	}
	return nil
}

func main() {
	check := flag.Bool("check", false, "compare generated files without changing them")
	include := flag.String("include", "", "directory containing google/protobuf well-known definitions")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "protogen does not accept positional arguments")
		os.Exit(1)
	}
	if err := run(*check, *include); err != nil {
		fmt.Fprintln(os.Stderr, "proto generation:", err)
		os.Exit(1)
	}
}
