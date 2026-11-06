package main

import "testing"

// These sinks deliberately retain results. In particular, storing a fresh
// pointer globally models an escaping result; it is not a neutral benchmark aid.
var (
	smallValueSink    SmallPayload
	smallPointerSink  *SmallPayload
	mediumValueSink   MediumPayload
	mediumPointerSink *MediumPayload
	largeValueSink    LargePayload
	largePointerSink  *LargePayload
	checksumSink      byte
)

func TestExistingPayloadOwnership(t *testing.T) {
	t.Run("16 bytes", func(t *testing.T) {
		source := newSmallValue(1)
		value := existingSmallValue(&source)
		pointer := existingSmallPointer(&source)
		value.Data[0] = 10
		if source.Data[0] != 1 || pointer != &source {
			t.Fatal("value copy or pointer identity is incorrect")
		}
		pointer.Data[0] = 20
		if source.Data[0] != 20 || value.Data[0] != 10 {
			t.Fatal("pointer mutation did not preserve the independent value")
		}
	})
	t.Run("4 KiB", func(t *testing.T) {
		var source MediumPayload
		source.Data[len(source.Data)-1] = 1
		value := existingMediumValue(&source)
		pointer := existingMediumPointer(&source)
		value.Data[len(value.Data)-1] = 10
		pointer.Data[len(pointer.Data)-1] = 20
		if pointer != &source || source.Data[len(source.Data)-1] != 20 || value.Data[len(value.Data)-1] != 10 {
			t.Fatal("medium payload copy or alias is incorrect")
		}
	})
	t.Run("1 MiB", func(t *testing.T) {
		var source LargePayload
		source.Data[len(source.Data)-1] = 1
		value := existingLargeValue(&source)
		pointer := existingLargePointer(&source)
		value.Data[len(value.Data)-1] = 10
		pointer.Data[len(pointer.Data)-1] = 20
		if pointer != &source || source.Data[len(source.Data)-1] != 20 || value.Data[len(value.Data)-1] != 10 {
			t.Fatal("large payload copy or alias is incorrect")
		}
	})
}

func TestFreshPayloadsAreIndependent(t *testing.T) {
	first := newSmallPointer(1)
	second := newSmallPointer(1)
	value := newSmallValue(1)
	if *first != value || *second != value {
		t.Fatal("value and pointer constructors initialize different contents")
	}
	first.Data[0] = 99
	if first == second || second.Data[0] != 1 || value.Data[0] != 1 {
		t.Fatal("fresh results unexpectedly share mutable data")
	}
}

// b.Loop excludes setup automatically and keeps loop work observable.
// Allocation counts and timing depend on the compiler, architecture, and use site.
// These benchmarks are intentionally sequential; -cpu does not make them parallel.
func BenchmarkExistingData(b *testing.B) {
	small := newSmallValue(1)
	var medium MediumPayload
	medium.Data[0] = 1
	var large LargePayload
	large.Data[0] = 1

	b.Run("16B/Value", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			smallValueSink = existingSmallValue(&small)
		}
	})
	b.Run("16B/Pointer", func(b *testing.B) {
		b.ReportAllocs()
		// Reuse an existing object: this case does not include constructing a payload.
		for b.Loop() {
			smallPointerSink = existingSmallPointer(&small)
		}
		smallPointerSink = nil
	})
	b.Run("4KiB/Value", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mediumValueSink = existingMediumValue(&medium)
		}
	})
	b.Run("4KiB/Pointer", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mediumPointerSink = existingMediumPointer(&medium)
		}
		mediumPointerSink = nil
	})
	b.Run("1MiB/Value", func(b *testing.B) {
		b.ReportAllocs()
		// Inspect both copy time and allocations: this large return can need a heap temporary.
		for b.Loop() {
			largeValueSink = existingLargeValue(&large)
		}
	})
	b.Run("1MiB/Pointer", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			largePointerSink = existingLargePointer(&large)
		}
		largePointerSink = nil
	})
}

func BenchmarkNewSmall(b *testing.B) {
	b.Run("ValueRetained", func(b *testing.B) {
		b.ReportAllocs()
		// Store the value in existing global storage; copying need not allocate a new object.
		for b.Loop() {
			smallValueSink = newSmallValue(1)
		}
	})
	b.Run("PointerRetained", func(b *testing.B) {
		b.ReportAllocs()
		// Retain a fresh object's address globally: the object escapes this caller.
		// Replacing it permits later GC of the old target once no other references retain it.
		for b.Loop() {
			smallPointerSink = newSmallPointer(1)
		}
		smallPointerSink = nil
	})
	b.Run("ValueLocal", func(b *testing.B) {
		b.ReportAllocs()
		var checksum byte
		for b.Loop() {
			payload := newSmallValue(1)
			checksum += payload.Data[0] + payload.Data[len(payload.Data)-1]
		}
		checksumSink = checksum
	})
	b.Run("PointerLocal", func(b *testing.B) {
		b.ReportAllocs()
		var checksum byte
		// Only the scalar checksum survives. Inlining/escape analysis may avoid
		// a heap allocation even though the constructor returns a pointer.
		for b.Loop() {
			payload := newSmallPointer(1)
			checksum += payload.Data[0] + payload.Data[len(payload.Data)-1]
		}
		checksumSink = checksum
	})
}
