package synk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSized(t *testing.T) {
	b := bytesPool.GetSized(2 * SizedPoolThreshold)
	assert.Equal(t, cap(b), 2*SizedPoolThreshold)
	bytesPool.Put(b)
	assert.Equal(t, underlyingPtr(b), underlyingPtr(bytesPool.GetSized(SizedPoolThreshold)))
}

func TestUnsized(t *testing.T) {
	b := bytesPool.Get()
	assert.Equal(t, cap(b), UnsizedAvg)
	bytesPool.Put(b)
	assert.Equal(t, underlyingPtr(b), underlyingPtr(bytesPool.Get()))
}

func TestGetSizedExactMatch(t *testing.T) {
	// Test exact size match reuse
	size := SizedPoolThreshold
	b1 := bytesPool.GetSized(size)
	assert.Equal(t, size, len(b1))
	assert.Equal(t, size, cap(b1))

	// Put back into pool
	bytesPool.Put(b1)

	// Get same size - should reuse the same buffer
	b2 := bytesPool.GetSized(size)
	assert.Equal(t, size, len(b2))
	assert.Equal(t, size, cap(b2))
	assert.Equal(t, underlyingPtr(b1), underlyingPtr(b2))
}

func TestGetSizedBufferSplit(t *testing.T) {
	// Test buffer splitting when capacity > requested size
	largeSize := 2 * SizedPoolThreshold
	requestedSize := SizedPoolThreshold

	// Create a large buffer and put it in pool
	b1 := bytesPool.GetSized(largeSize)
	assert.Equal(t, largeSize, len(b1))
	assert.Equal(t, largeSize, cap(b1))

	bytesPool.Put(b1)

	// Request smaller size - should split the buffer
	b2 := bytesPool.GetSized(requestedSize)
	assert.Equal(t, requestedSize, len(b2))
	assert.Equal(t, requestedSize, cap(b2)) // capacity should remain the original
	assert.Equal(t, underlyingPtr(b1), underlyingPtr(b2))

	// The remaining part should be put back in pool
	// Request the remaining size to verify
	remainingSize := largeSize - requestedSize
	b3 := bytesPool.GetSized(remainingSize)
	assert.Equal(t, remainingSize, len(b3))
	assert.Equal(t, remainingSize, cap(b3))

	// Verify the remaining buffer points to the correct memory location
	originalPtr := underlyingPtr(b1)
	remainingPtr := underlyingPtr(b3)

	// The remaining buffer should start at original + requestedSize
	expectedOffset := uintptr(originalPtr) + uintptr(requestedSize)
	actualOffset := uintptr(remainingPtr)
	assert.Equal(t, expectedOffset, actualOffset, "Remaining buffer should point to correct offset")
}

func TestGetSizedSmallRemainder(t *testing.T) {
	// Test when remaining size is smaller than SizedPoolThreshold
	poolSize := SizedPoolThreshold + 100 // Just slightly larger than threshold
	requestedSize := SizedPoolThreshold

	// Create buffer and put in pool
	b1 := bytesPool.GetSized(poolSize)
	bytesPool.Put(b1)

	// Request size that leaves small remainder
	b2 := bytesPool.GetSized(requestedSize)
	assert.Equal(t, requestedSize, len(b2))
	assert.Equal(t, requestedSize, cap(b2))

	// The small remainder (100 bytes) should NOT be put back in sized pool
	// Try to get the remainder size - should create new buffer
	b3 := bytesPool.GetSized(100)
	assert.Equal(t, 100, len(b3))
	assert.Equal(t, 100, cap(b3))
	assert.NotEqual(t, underlyingPtr(b2), underlyingPtr(b3))
}

func TestGetSizedSmallBufferBypass(t *testing.T) {
	// Test that small buffers (< SizedPoolThreshold) don't use sized pool
	smallSize := SizedPoolThreshold - 1

	b1 := bytesPool.GetSized(smallSize)
	assert.Equal(t, smallSize, len(b1))
	assert.Equal(t, smallSize, cap(b1))

	b2 := bytesPool.GetSized(smallSize)
	assert.Equal(t, smallSize, len(b2))
	assert.Equal(t, smallSize, cap(b2))

	// Should be different buffers (not pooled)
	assert.NotEqual(t, underlyingPtr(b1), underlyingPtr(b2))
}

func TestGetSizedBufferTooSmall(t *testing.T) {
	// Test when pool buffer is smaller than requested size
	smallSize := SizedPoolThreshold
	largeSize := 2 * SizedPoolThreshold

	// Put small buffer in pool
	b1 := bytesPool.GetSized(smallSize)
	bytesPool.Put(b1)

	// Request larger size - should create new buffer, not reuse small one
	b2 := bytesPool.GetSized(largeSize)
	assert.Equal(t, largeSize, len(b2))
	assert.Equal(t, largeSize, cap(b2))
	assert.NotEqual(t, underlyingPtr(b1), underlyingPtr(b2))

	// The small buffer should still be in pool
	b3 := bytesPool.GetSized(smallSize)
	assert.Equal(t, underlyingPtr(b1), underlyingPtr(b3))
}

func TestGetSizedMultipleSplits(t *testing.T) {
	// Test multiple sequential splits of the same buffer
	hugeSize := 4 * SizedPoolThreshold
	splitSize := SizedPoolThreshold

	// Create huge buffer
	b1 := bytesPool.GetSized(hugeSize)
	originalPtr := underlyingPtr(b1)
	bytesPool.Put(b1)

	// Split it into smaller pieces
	pieces := make([][]byte, 0, 4)
	for i := range 4 {
		piece := bytesPool.GetSized(splitSize)
		pieces = append(pieces, piece)

		// Each piece should point to the correct offset
		expectedOffset := uintptr(originalPtr) + uintptr(i*splitSize)
		actualOffset := uintptr(underlyingPtr(piece))
		assert.Equal(t, expectedOffset, actualOffset, "Piece %d should point to correct offset", i)
		assert.Equal(t, splitSize, len(piece))
		assert.Equal(t, splitSize, cap(piece))
	}

	// All pieces should have the same underlying capacity
	for i, piece := range pieces {
		assert.Equal(t, splitSize, cap(piece), "Piece %d should have correct capacity", i)
	}
}

func TestGetSizedMemorySafety(t *testing.T) {
	// Test that split buffers don't interfere with each other
	totalSize := 3 * SizedPoolThreshold
	firstSize := SizedPoolThreshold

	// Create buffer and split it
	b1 := bytesPool.GetSized(totalSize)
	// Fill with test data
	for i := range len(b1) {
		b1[i] = byte(i % 256)
	}

	bytesPool.Put(b1)

	// Get first part
	first := bytesPool.GetSized(firstSize)
	assert.Equal(t, firstSize, len(first))

	// Verify data integrity
	for i := range len(first) {
		assert.Equal(t, byte(i%256), first[i], "Data should be preserved after split")
	}

	// Get remaining part
	remainingSize := totalSize - firstSize
	remaining := bytesPool.GetSized(remainingSize)
	assert.Equal(t, remainingSize, len(remaining))

	// Verify remaining data
	for i := range len(remaining) {
		expected := byte((i + firstSize) % 256)
		assert.Equal(t, expected, remaining[i], "Remaining data should be preserved")
	}
}

func TestGetSizedCapacityLimiting(t *testing.T) {
	// Test that returned buffers have limited capacity to prevent overwrites
	largeSize := 2 * SizedPoolThreshold
	requestedSize := SizedPoolThreshold

	// Create large buffer and put in pool
	b1 := bytesPool.GetSized(largeSize)
	bytesPool.Put(b1)

	// Get smaller buffer from the split
	b2 := bytesPool.GetSized(requestedSize)
	assert.Equal(t, requestedSize, len(b2))
	assert.Equal(t, requestedSize, cap(b2), "Returned buffer should have limited capacity")

	// Try to append data - should not be able to overwrite beyond capacity
	original := make([]byte, len(b2))
	copy(original, b2)

	// This append should force a new allocation since capacity is limited
	b2 = append(b2, 1, 2, 3, 4, 5)
	assert.Greater(t, len(b2), requestedSize, "Buffer should have grown")

	// Get the remaining buffer to verify it wasn't affected
	remainingSize := largeSize - requestedSize
	b3 := bytesPool.GetSized(remainingSize)
	assert.Equal(t, remainingSize, len(b3))
	assert.Equal(t, remainingSize, cap(b3), "Remaining buffer should have limited capacity")
}

func TestGetSizedAppendSafety(t *testing.T) {
	// Test that appending to returned buffer doesn't affect remaining buffer
	totalSize := 4 * SizedPoolThreshold
	firstSize := SizedPoolThreshold

	// Create buffer with specific pattern
	b1 := bytesPool.GetSized(totalSize)
	for i := range len(b1) {
		b1[i] = byte(100 + i%100)
	}
	bytesPool.Put(b1)

	// Get first part
	first := bytesPool.GetSized(firstSize)
	assert.Equal(t, firstSize, cap(first), "First part should have limited capacity")

	// Store original first part content
	originalFirst := make([]byte, len(first))
	copy(originalFirst, first)

	// Get remaining part to establish its state
	remaining := bytesPool.GetSized(SizedPoolThreshold)

	// Store original remaining content
	originalRemaining := make([]byte, len(remaining))
	copy(originalRemaining, remaining)

	// Now try to append to first - this should not affect remaining buffers
	// since capacity is limited
	first = append(first, make([]byte, 1000)...)

	// Verify remaining buffer content is unchanged
	for i := range len(originalRemaining) {
		assert.Equal(t, originalRemaining[i], remaining[i],
			"Remaining buffer should be unaffected by append to first buffer")
	}
}
