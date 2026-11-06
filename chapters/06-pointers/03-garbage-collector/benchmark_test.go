package main

import (
	"strings"
	"testing"
)

// Builders return retained results in both variants. Sinks preserve that lifetime.
// Timings and allocation counts are observations, not test assertions.
func BenchmarkReceipts(b *testing.B) {
	b.Run("Values", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkReceipts = buildReceiptValues(1000)
		}
		sinkReceipts = nil
	})
	b.Run("Pointers", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkReceiptPointers = buildReceiptPointers(1000)
		}
		sinkReceiptPointers = nil
	})
}

func BenchmarkIDs(b *testing.B) {
	b.Run("Grow", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIDs = buildIDsWithoutPrealloc(1000)
		}
		sinkIDs = nil
	})
	b.Run("Preallocate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkIDs = buildIDsWithPrealloc(1000)
		}
		sinkIDs = nil
	})
}

func BenchmarkLookup(b *testing.B) {
	b.Run("Grow", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkMap = buildLookupWithoutPrealloc(1000)
		}
		sinkMap = nil
	})
	b.Run("Preallocate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkMap = buildLookupWithPrealloc(1000)
		}
		sinkMap = nil
	})
}

func BenchmarkMessageProcessing(b *testing.B) {
	b.Run("NewBuffers", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkChecksum = processMessagesWithNewBuffers(1000)
		}
	})
	b.Run("ReuseBuffer", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sinkChecksum = processMessagesWithReusedBuffer(1000)
		}
	})
}

func BenchmarkMessageChecksum(b *testing.B) {
	message := strings.Repeat("temporary buffer ", 128)
	b.Run("LocalBuffer", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(message)))
		for b.Loop() {
			sinkChecksum = checksumWithLocalBuffer(message)
		}
	})
	b.Run("Pool", func(b *testing.B) {
		checksumWithPool(message) // Warm-up is excluded; reuse is never guaranteed.
		b.ReportAllocs()
		b.SetBytes(int64(len(message)))
		for b.Loop() {
			sinkChecksum = checksumWithPool(message)
		}
	})
}
