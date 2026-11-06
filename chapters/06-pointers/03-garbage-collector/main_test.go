package main

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestMessageContentsAndOwnership(t *testing.T) {
	for _, id := range []int{0, 9, 10, 1234} {
		got := appendRequestID(nil, id)
		if want := fmt.Sprintf("request-id:%d", id); string(got) != want {
			t.Errorf("message for %d = %q, want %q", id, got, want)
		}
	}
	messages := buildMessagesWithNewBuffers(12)
	for id, message := range messages {
		if want := fmt.Sprintf("request-id:%d", id); string(message) != want {
			t.Errorf("retained message %d = %q, want %q", id, message, want)
		}
	}
	before := bytes.Clone(messages[1])
	messages[0][0] = 'X'
	if !bytes.Equal(messages[1], before) {
		t.Error("retained messages share mutable byte storage")
	}
}

func TestChecksumBytes(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  uint64
	}{{"", 0}, {"A", 65}, {"Go", 2312}, {"\x00\xff", 255}} {
		if got := checksumBytes([]byte(tt.input)); got != tt.want {
			t.Errorf("checksumBytes(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestMessageProcessingVariants(t *testing.T) {
	for _, count := range []int{0, 1, 12, 1000} {
		var want uint64
		for id := 0; id < count; id++ {
			want += checksumBytes([]byte(fmt.Sprintf("request-id:%d", id)))
		}
		if got := processMessagesWithNewBuffers(count); got != want {
			t.Errorf("new buffers for %d messages = %d, want %d", count, got, want)
		}
		if got := processMessagesWithReusedBuffer(count); got != want {
			t.Errorf("reused buffer for %d messages = %d, want %d", count, got, want)
		}
	}
}

func TestPrepareForPool(t *testing.T) {
	for _, capacity := range []int{0, 256, maxPooledBufferCapacity, maxPooledBufferCapacity + 1} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			buffer := &messageBuffer{data: make([]byte, capacity)}
			reuse := prepareForPool(buffer)
			if reuse != (capacity <= maxPooledBufferCapacity) {
				t.Fatalf("reuse = %t for capacity %d", reuse, capacity)
			}
			if len(buffer.data) != 0 {
				t.Error("buffer length was not reset")
			}
			if reuse && cap(buffer.data) != capacity {
				t.Error("reusable buffer lost its capacity")
			}
			if !reuse && buffer.data != nil {
				t.Error("oversized backing array is still referenced")
			}
		})
	}
}

func TestPoolChecksumDoesNotRequireRetention(t *testing.T) {
	for _, message := range []string{"", "a longer first message", "x", "café🙂", strings.Repeat("x", maxPooledBufferCapacity+1), "short again"} {
		want := checksumBytes([]byte(message))
		// Collection may discard pooled objects; do not assume an identity or hit rate.
		runtime.GC()
		if got := checksumWithPool(message); got != want {
			t.Errorf("pooled checksum for %d bytes = %d, want %d", len(message), got, want)
		}
		if got := checksumWithLocalBuffer(message); got != want {
			t.Errorf("local checksum for %d bytes = %d, want %d", len(message), got, want)
		}
	}
}

func TestPoolConcurrentOwnership(t *testing.T) {
	var workers sync.WaitGroup
	for id := 0; id < 8; id++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			message := strings.Repeat(fmt.Sprintf("worker %d café🙂 ", id), 20)
			want := checksumBytes([]byte(message))
			for i := 0; i < 20; i++ {
				if got := checksumWithPool(message); got != want {
					t.Errorf("worker %d checksum = %d, want %d", id, got, want)
					return
				}
			}
		}()
	}
	workers.Wait()
}

func TestClearObjectCache(t *testing.T) {
	cache := makeObjectCache(3)
	view := cache[:1]
	clearObjectCache(view)
	if cache[0] != nil || cache[1] == nil || cache[2] == nil {
		t.Fatal("clear should affect only the visible slots")
	}
	clearObjectCache(view[:cap(view)])
	for index, object := range cache {
		if object != nil {
			t.Errorf("reference in hidden slot %d was retained", index)
		}
	}
	if len(view) != 1 || cap(view) != 3 {
		t.Error("clearing changed the slice descriptor")
	}
}

func TestReceiptRepresentations(t *testing.T) {
	for _, count := range []int{0, 1, 10} {
		values := buildReceiptValues(count)
		pointers := buildReceiptPointers(count)
		if len(values) != count || len(pointers) != count {
			t.Fatal("receipt counts differ from the requested count")
		}
		for index, value := range values {
			want := Receipt{ID: index, TotalCents: 1000 + index, Paid: index%2 == 0}
			if value != want || pointers[index] == nil || *pointers[index] != want {
				t.Fatalf("receipt %d has incorrect contents", index)
			}
		}
		if count > 1 {
			pointers[0].ID = -1
			if pointers[1].ID != 1 || values[0].ID != 0 {
				t.Error("independent receipts unexpectedly alias")
			}
		}
	}
}

func TestPreallocationPreservesContents(t *testing.T) {
	for _, count := range []int{0, 1, 100} {
		for _, ids := range [][]int{buildIDsWithoutPrealloc(count), buildIDsWithPrealloc(count)} {
			if len(ids) != count {
				t.Fatal("incorrect number of IDs")
			}
			for index, id := range ids {
				if id != index {
					t.Errorf("ID at %d = %d", index, id)
				}
			}
		}
		for _, lookup := range []map[int]int{buildLookupWithoutPrealloc(count), buildLookupWithPrealloc(count)} {
			if len(lookup) != count {
				t.Fatal("incorrect number of lookup entries")
			}
			for id := 0; id < count; id++ {
				if value, ok := lookup[id]; !ok || value != id*10 {
					t.Errorf("lookup[%d] = (%d, %t)", id, value, ok)
				}
			}
		}
	}
}
