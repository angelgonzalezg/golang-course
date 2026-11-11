// Command sqlgen runs a pinned sqlc release. -check compares expected output
// with the checked-in files without rewriting them; CI uses the same command.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

const sqlcVersion = "v1.31.1"

// run is development tooling, not application startup. An explicit module
// version keeps the generator's dependency graph separate from the SQL lessons.
func run(check bool) error {
	if _, err := os.Stat("sqlc.yaml"); err != nil {
		return fmt.Errorf("run sqlgen from the SQL module root: %w", err)
	}
	operation := "generate"
	if check {
		operation = "diff"
	}
	// Go caches the pinned tool and verifies downloaded modules. No global
	// installation, PATH change, database connection, or cloud account is needed.
	command := exec.Command("go", "run", "github.com/sqlc-dev/sqlc/cmd/sqlc@"+sqlcVersion, operation)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("sqlc %s (%s): %w", operation, sqlcVersion, err)
	}
	if check {
		fmt.Println("Generated SQL bindings match the pinned sqlc output.")
	} else {
		fmt.Println("Generated SQL bindings from the shared schema and named queries.")
	}
	return nil
}

func main() {
	check := flag.Bool("check", false, "compare generated files without changing them")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "sqlgen does not accept positional arguments")
		os.Exit(1)
	}
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
