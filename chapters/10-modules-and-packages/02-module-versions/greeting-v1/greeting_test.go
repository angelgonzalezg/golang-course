package greeting_test

import (
	"testing"

	"example.com/course/greeting"
)

func TestHello(t *testing.T) {
	for _, tt := range []struct{ name, want string }{
		{"Ana", "Hello, Ana!"},
		{"  Ana  ", "Hello, Ana!"},
		{"", "Hello, friend!"},
		{" \t\u2003", "Hello, friend!"},
	} {
		if got := greeting.Hello(tt.name); got != tt.want {
			t.Errorf("Hello(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
