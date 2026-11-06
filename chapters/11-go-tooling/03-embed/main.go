// Embedded Resources
// Topic: include static text and a read-only filesystem in the compiled program.
// Usage: go run ./chapters/11-go-tooling/03-embed
package main

import (
	"embed"
	"fmt"
)

// Use a string for one fixed text resource. Paths are relative to this package's
// source directory at build time, regardless of the executable's working directory.
//
//go:embed assets/welcome.txt
var welcome string

// Use embed.FS for multiple files accessed by name. The patterns select resources
// at build time, not files to search for later on the host filesystem.
//
//go:embed assets/*.txt
var lessonFiles embed.FS

func main() {
	fmt.Print(welcome)
	help, err := lessonFiles.ReadFile("assets/help.txt")
	if err != nil {
		fmt.Println("read embedded help:", err)
		return
	}
	fmt.Print(string(help))

	entries, err := lessonFiles.ReadDir("assets")
	if err != nil {
		fmt.Println("list embedded assets:", err)
		return
	}
	fmt.Println("Embedded files:")
	for _, entry := range entries {
		fmt.Println("-", entry.Name())
	}
}
