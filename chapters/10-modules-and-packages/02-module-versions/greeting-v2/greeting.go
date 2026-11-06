// Package greeting provides the incompatible version 2 API for the local module lab.
package greeting

import (
	"errors"
	"strings"
)

var ErrBlankName = errors.New("name must not be blank")

// Hello greets the trimmed name, returning an error for blank names.
// The added error result breaks callers of v1, so v2 has a distinct module path.
func Hello(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrBlankName
	}
	return "Hello, " + name + "!", nil
}
