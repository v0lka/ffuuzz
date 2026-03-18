package mutate

import (
	"math/rand"
	"testing"
)

func TestBitFlip(t *testing.T) {
	data := []byte{0b00000000}
	result := BitFlip(data, 0)
	if result[0] != 0b00000001 {
		t.Fatalf("BitFlip(0x00, bit 0) = 0x%02x, want 0x01", result[0])
	}

	data = []byte{0b00000001}
	result = BitFlip(data, 0)
	if result[0] != 0b00000000 {
		t.Fatalf("BitFlip(0x01, bit 0) = 0x%02x, want 0x00", result[0])
	}

	data = []byte{0x00}
	result = BitFlip(data, 7)
	if result[0] != 0x80 {
		t.Fatalf("BitFlip(0x00, bit 7) = 0x%02x, want 0x80", result[0])
	}
}

func TestBitFlipMultiByte(t *testing.T) {
	data := []byte{0x00, 0x00}
	// Flip bit 8 -> byte 1, bit 0
	result := BitFlip(data, 8)
	if result[0] != 0x00 || result[1] != 0x01 {
		t.Fatalf("BitFlip(2-byte, bit 8) = %v, want [0x00, 0x01]", result)
	}
}

func TestBitFlipEmpty(t *testing.T) {
	result := BitFlip(nil, 5)
	if len(result) != 0 {
		t.Fatalf("BitFlip(nil) returned non-empty: %v", result)
	}
}

func TestBitFlipWraps(t *testing.T) {
	data := []byte{0x00}
	// bitPos 8 wraps to bit 0 of byte 0
	result := BitFlip(data, 8)
	if result[0] != 0x01 {
		t.Fatalf("BitFlip wrap: got 0x%02x, want 0x01", result[0])
	}
}

func TestByteFlip(t *testing.T) {
	data := []byte{0xAA}
	result := ByteFlip(data, 0)
	if result[0] != 0x55 {
		t.Fatalf("ByteFlip(0xAA) = 0x%02x, want 0x55", result[0])
	}
}

func TestByteFlipEmpty(t *testing.T) {
	result := ByteFlip(nil, 0)
	if len(result) != 0 {
		t.Fatalf("ByteFlip(nil) returned non-empty: %v", result)
	}
}

func TestByteFlipWraps(t *testing.T) {
	data := []byte{0xFF, 0x00}
	result := ByteFlip(data, 2) // wraps to index 0
	if result[0] != 0x00 {
		t.Fatalf("ByteFlip wrap: got 0x%02x, want 0x00", result[0])
	}
}

func TestArithmeticAdd(t *testing.T) {
	data := []byte{100}
	result := ArithmeticAdd(data, 0, 10)
	if result[0] != 110 {
		t.Fatalf("ArithmeticAdd(100, +10) = %d, want 110", result[0])
	}

	data = []byte{10}
	result = ArithmeticAdd(data, 0, -5)
	if result[0] != 5 {
		t.Fatalf("ArithmeticAdd(10, -5) = %d, want 5", result[0])
	}
}

func TestArithmeticAddWraps(t *testing.T) {
	data := []byte{0xFF}
	result := ArithmeticAdd(data, 0, 1)
	if result[0] != 0x00 {
		t.Fatalf("ArithmeticAdd(0xFF, +1) = 0x%02x, want 0x00 (wrap)", result[0])
	}
}

func TestArithmeticAddEmpty(t *testing.T) {
	result := ArithmeticAdd(nil, 0, 5)
	if len(result) != 0 {
		t.Fatalf("ArithmeticAdd(nil) returned non-empty: %v", result)
	}
}

func TestInterestingReplace(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, 10)
	for i := range data {
		data[i] = 0xAA
	}
	original := make([]byte, len(data))
	copy(original, data)

	result := InterestingReplace(data, rng)
	if len(result) != len(original) {
		t.Fatalf("InterestingReplace changed length: %d -> %d", len(original), len(result))
	}

	// Should have at least one byte different
	same := true
	for i := range result {
		if result[i] != original[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("InterestingReplace produced no change")
	}
}

func TestInterestingReplaceEmpty(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	result := InterestingReplace(nil, rng)
	if len(result) != 0 {
		t.Fatalf("InterestingReplace(nil) returned non-empty: %v", result)
	}
}

func TestBlockInsert(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	data := []byte("hello")
	result := BlockInsert(data, rng)
	if len(result) <= len(data) {
		t.Fatalf("BlockInsert should increase length: %d -> %d", len(data), len(result))
	}
}

func TestBlockDelete(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	data := []byte("hello world")
	result := BlockDelete(data, rng)
	if len(result) >= len(data) {
		t.Fatalf("BlockDelete should decrease length: %d -> %d", len(data), len(result))
	}
}

func TestBlockDeleteSingleByte(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	data := []byte{0x42}
	result := BlockDelete(data, rng)
	if len(result) != 1 {
		t.Fatalf("BlockDelete single byte should return as-is, got len %d", len(result))
	}
}

func TestBlockDuplicate(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	data := []byte("abcd")
	result := BlockDuplicate(data, rng)
	if len(result) <= len(data) {
		t.Fatalf("BlockDuplicate should increase length: %d -> %d", len(data), len(result))
	}
}

func TestBlockReplace(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, 20)
	for i := range data {
		data[i] = 0xAA
	}
	original := make([]byte, len(data))
	copy(original, data)

	result := BlockReplace(data, rng)
	if len(result) != len(original) {
		t.Fatalf("BlockReplace changed length: %d -> %d", len(original), len(result))
	}

	same := true
	for i := range result {
		if result[i] != original[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("BlockReplace produced no change")
	}
}

func TestSplice(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	a := []byte("AAAA")
	b := []byte("BBBB")
	result := Splice(a, b, rng)
	if len(result) == 0 {
		t.Fatal("Splice returned empty result")
	}
}

func TestSpliceEmptyA(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	result := Splice(nil, []byte("hello"), rng)
	if string(result) != "hello" {
		t.Fatalf("Splice(nil, hello) = %q, want %q", result, "hello")
	}
}

func TestSpliceEmptyB(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	result := Splice([]byte("hello"), nil, rng)
	if string(result) != "hello" {
		t.Fatalf("Splice(hello, nil) = %q, want %q", result, "hello")
	}
}

func TestBlockOperation(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	data := []byte("test data for block ops")
	result := BlockOperation(data, rng)
	// Just verify it doesn't panic and returns something
	if result == nil {
		t.Fatal("BlockOperation returned nil")
	}
}

func TestBlockOperationEmpty(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	result := BlockOperation(nil, rng)
	if len(result) != 0 {
		t.Fatalf("BlockOperation(nil) returned non-empty: %v", result)
	}
}
