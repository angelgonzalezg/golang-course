package main

import "testing"

// The shared sink models retained results. Both cases retain the same label;
// benchmarks run sequentially and make no fixed allocation or speed assertions.
var retainedLabel string

func BenchmarkBuildLabel(b *testing.B) {
	b.Run("OrdinaryConversion", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			retainedLabel = buildLabel("task", 42)
		}
		if retainedLabel != "task:42" {
			b.Fatal("unexpected label:", retainedLabel)
		}
	})
	b.Run("PrivateBufferView", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			retainedLabel = buildLabelUnsafe("task", 42)
		}
		if retainedLabel != "task:42" {
			b.Fatal("unexpected label:", retainedLabel)
		}
	})
}
