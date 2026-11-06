// Package textutil contains text helpers for the packages-and-imports lesson.
// The internal directory restricts importers to the tree rooted at its parent.
package textutil

import "strings"

// CleanName collapses Unicode whitespace and reports whether any name remains.
// It is exported for use across packages inside the allowed internal boundary.
func CleanName(name string) (string, bool) {
	name = strings.Join(strings.Fields(name), " ")
	return name, name != ""
}
