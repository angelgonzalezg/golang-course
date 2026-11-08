package measurement

import (
	"testing"

	"golangcourse/chapters/15-testing/01-unit-tests"
)

func BenchmarkNormalize(b *testing.B) {
	for _, input := range []struct {
		name string
		text string
	}{
		{name: "canonical", text: "go testing"},
		{name: "ASCII mixed case", text: " Go Testing "},
		{name: "Unicode", text: " NIÑO 🐹 "},
	} {
		b.Run(input.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(input.text)))
			// Loop excludes the setup above and keeps the measured calls observable.
			// No global sink is needed for this direct b.Loop form.
			for b.Loop() {
				if _, err := labels.Normalize(input.text); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCounterSerial(b *testing.B) {
	var counter Counter
	b.ReportAllocs()
	for b.Loop() {
		counter.Increment()
	}
	if got := counter.Value(); got != b.N {
		b.Fatalf("counter = %d, want %d", got, b.N)
	}
}

func BenchmarkCounterParallel(b *testing.B) {
	var counter Counter
	b.ReportAllocs()
	// RunParallel distributes b.N total operations among workers. It owns this
	// loop; do not combine it with b.Loop in the same benchmark.
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Increment()
		}
	})
	b.StopTimer()
	if got := counter.Value(); got != b.N {
		b.Fatalf("counter = %d, want %d", got, b.N)
	}
}
