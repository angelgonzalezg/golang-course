package textutil

import "testing"

func TestCleanName(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "", false},
		{" \t\u2003\n", "", false},
		{"Ana", "Ana", true},
		{" \u2003Ana\t María\n", "Ana María", true},
		{"李 明", "李 明", true},
	} {
		got, ok := CleanName(tt.input)
		if got != tt.want || ok != tt.ok {
			t.Errorf("CleanName(%q) = (%q, %t), want (%q, %t)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}
