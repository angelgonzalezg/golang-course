package greeting

import (
	"errors"
	"fmt"

	"golangcourse/chapters/10-modules-and-packages/01-packages-and-imports/internal/textutil"
)

var (
	ErrBlankName           = errors.New("name must not be blank")
	ErrUnsupportedLanguage = errors.New("unsupported language")
)

// Greeter holds a small configuration value. Its zero value greets in English.
// The prefix field is unexported, so callers choose configuration through New.
type Greeter struct {
	prefix string
}

// New returns a configured Greeter or an error for an unsupported language.
// Explicit initialization lets the caller handle errors without package init hooks.
func New(language Language) (Greeter, error) {
	prefix, ok := prefixFor(language)
	if !ok {
		return Greeter{}, fmt.Errorf("new greeter for language %d: %w", language, ErrUnsupportedLanguage)
	}
	return Greeter{prefix: prefix}, nil
}

// Greet returns a greeting after normalizing whitespace in the name.
// A blank name returns an empty result and ErrBlankName.
func (g Greeter) Greet(name string) (string, error) {
	name, ok := textutil.CleanName(name)
	if !ok {
		return "", ErrBlankName
	}
	prefix := g.prefix
	if prefix == "" {
		// defaultPrefix is private to this package, even though another file defines it.
		prefix = defaultPrefix
	}
	return prefix + ", " + name + "!", nil
}
