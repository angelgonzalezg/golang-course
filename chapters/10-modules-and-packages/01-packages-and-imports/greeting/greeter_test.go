package greeting_test

import (
	"errors"
	"testing"

	"golangcourse/chapters/10-modules-and-packages/01-packages-and-imports/greeting"
)

// An external test package exercises only the same public API as the command.
func TestPublicGreetingAPI(t *testing.T) {
	tests := []struct {
		name     string
		language greeting.Language
		input    string
		want     string
	}{
		{"English", greeting.English, "Ana", "Hello, Ana!"},
		{"Spanish", greeting.Spanish, "Luis", "Hola, Luis!"},
		{"Unicode whitespace", greeting.Spanish, "\u2003Ana\t María\n", "Hola, Ana María!"},
		{"Unicode name", greeting.English, "李 明", "Hello, 李 明!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			greeter, err := greeting.New(tt.language)
			if err != nil {
				t.Fatal(err)
			}
			got, err := greeter.Greet(tt.input)
			if err != nil || got != tt.want {
				t.Fatalf("Greet(%q) = (%q, %v), want (%q, nil)", tt.input, got, err, tt.want)
			}
		})
	}
}

func TestZeroValueGreeter(t *testing.T) {
	var greeter greeting.Greeter
	got, err := greeter.Greet("Ana")
	if err != nil || got != "Hello, Ana!" {
		t.Fatalf("zero-value greeting = (%q, %v)", got, err)
	}
}

func TestPublicErrors(t *testing.T) {
	_, err := greeting.New(greeting.Language(99))
	if !errors.Is(err, greeting.ErrUnsupportedLanguage) {
		t.Fatalf("unsupported language error = %v", err)
	}
	var greeter greeting.Greeter
	for _, name := range []string{"", " \t\n\u2003"} {
		got, err := greeter.Greet(name)
		if got != "" || !errors.Is(err, greeting.ErrBlankName) {
			t.Errorf("blank greeting = (%q, %v), want empty result and ErrBlankName", got, err)
		}
	}
}
