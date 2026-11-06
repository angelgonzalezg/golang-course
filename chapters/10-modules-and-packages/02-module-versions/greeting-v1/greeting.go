// Package greeting provides the version 1 API used by the local module lab.
package greeting

import "strings"

// Hello greets the trimmed name, using "friend" when the name is blank.
func Hello(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "friend"
	}
	return "Hello, " + name + "!"
}
