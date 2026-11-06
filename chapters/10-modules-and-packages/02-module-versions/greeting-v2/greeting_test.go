package greeting_test

import (
	"errors"
	"testing"

	"example.com/course/greeting/v2"
)

func TestHello(t *testing.T) {
	for _, name := range []string{"Ana", "  Ana  "} {
		got, err := greeting.Hello(name)
		if err != nil || got != "Hello, Ana!" {
			t.Errorf("Hello(%q) = (%q, %v)", name, got, err)
		}
	}
	for _, name := range []string{"", " \t\u2003"} {
		got, err := greeting.Hello(name)
		if got != "" || !errors.Is(err, greeting.ErrBlankName) {
			t.Errorf("Hello(%q) = (%q, %v), want empty result and ErrBlankName", name, got, err)
		}
	}
}
