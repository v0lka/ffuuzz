package mutate

import (
	"encoding/base64"
	"math/rand"

	"ffuuzz/internal/model"
)

// interestingValues contains boundary values commonly used in fuzz testing.
var interestingValues = [][]byte{
	{0x00},
	{0x01},
	{0xFF},
	{0x7F},                   // 127
	{0x80},                   // 128
	{0x00, 0x00},             // 0 (16-bit)
	{0xFF, 0xFF},             // 65535
	{0x00, 0x01},             // 256
	{0x7F, 0xFF},             // 32767
	{0x80, 0x00},             // 32768
	{0xFF, 0xFF, 0xFF, 0xFF}, // max uint32
	{0x7F, 0xFF, 0xFF, 0xFF}, // max int32
	{0x80, 0x00, 0x00, 0x00}, // min int32 (abs)
	{0x00, 0x00, 0x00, 0x00}, // 0 (32-bit)
}

// PrimitiveMutator applies byte-level mutation primitives to exchange bodies.
type PrimitiveMutator struct{}

func (m *PrimitiveMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
	body, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
	if err != nil || len(body) == 0 {
		return MutationResult{Exchange: ex, Operators: []string{"primitive:noop"}}
	}

	mutated := make([]byte, len(body))
	copy(mutated, body)

	// Pick a random primitive based on RNG
	op := rng.Intn(6)
	var opName string

	switch op {
	case 0:
		opName = "bitflip"
		mutated = BitFlip(mutated, rng.Intn(len(mutated)*8))
	case 1:
		opName = "byteflip"
		mutated = ByteFlip(mutated, rng.Intn(len(mutated)))
	case 2:
		opName = "arith"
		pos := rng.Intn(len(mutated))
		delta := rng.Intn(35) - 17 // range [-17, 17]
		mutated = ArithmeticAdd(mutated, pos, delta)
	case 3:
		opName = "interesting"
		mutated = InterestingReplace(mutated, rng)
	case 4:
		opName = "block_op"
		mutated = BlockOperation(mutated, rng)
	case 5:
		opName = "splice"
		// Splice with the response body as a second source
		other, err := base64.StdEncoding.DecodeString(ex.Response.BodyB64)
		if err != nil {
			other = nil
		}
		if len(other) > 0 {
			mutated = Splice(mutated, other, rng)
		} else {
			opName = "bitflip"
			mutated = BitFlip(mutated, rng.Intn(len(mutated)*8))
		}
	}

	ex.Request.BodyB64 = base64.StdEncoding.EncodeToString(mutated)
	return MutationResult{Exchange: ex, Operators: []string{"primitive:" + opName}}
}

// BitFlip inverts a single bit at the given bit position.
func BitFlip(data []byte, bitPos int) []byte {
	if len(data) == 0 {
		return data
	}
	bitPos = bitPos % (len(data) * 8)
	byteIdx := bitPos / 8
	bitIdx := uint(bitPos % 8)
	data[byteIdx] ^= 1 << bitIdx
	return data
}

// ByteFlip inverts all bits in the byte at pos.
func ByteFlip(data []byte, pos int) []byte {
	if len(data) == 0 {
		return data
	}
	pos = pos % len(data)
	data[pos] ^= 0xFF
	return data
}

// ArithmeticAdd adds delta to the byte at pos (wrapping).
func ArithmeticAdd(data []byte, pos, delta int) []byte {
	if len(data) == 0 {
		return data
	}
	pos = pos % len(data)
	data[pos] = byte(int(data[pos]) + delta)
	return data
}

// InterestingReplace replaces bytes at a random position with an interesting value.
func InterestingReplace(data []byte, rng *rand.Rand) []byte {
	if len(data) == 0 {
		return data
	}
	val := interestingValues[rng.Intn(len(interestingValues))]
	pos := rng.Intn(len(data))
	for i := 0; i < len(val) && pos+i < len(data); i++ {
		data[pos+i] = val[i]
	}
	return data
}

// BlockOperation performs a random block operation: insert, delete, duplicate, or replace.
func BlockOperation(data []byte, rng *rand.Rand) []byte {
	if len(data) == 0 {
		return data
	}
	op := rng.Intn(4)
	switch op {
	case 0: // insert
		return BlockInsert(data, rng)
	case 1: // delete
		return BlockDelete(data, rng)
	case 2: // duplicate
		return BlockDuplicate(data, rng)
	case 3: // replace
		return BlockReplace(data, rng)
	}
	return data
}

// BlockInsert inserts a random block at a random position.
func BlockInsert(data []byte, rng *rand.Rand) []byte {
	maxBlockSize := min(32, len(data)/2+1)
	if maxBlockSize < 1 {
		maxBlockSize = 1
	}
	blockLen := rng.Intn(maxBlockSize) + 1
	block := make([]byte, blockLen)
	rng.Read(block)
	pos := rng.Intn(len(data) + 1)
	result := make([]byte, 0, len(data)+blockLen)
	result = append(result, data[:pos]...)
	result = append(result, block...)
	result = append(result, data[pos:]...)
	return result
}

// BlockDelete removes a random block from data.
func BlockDelete(data []byte, rng *rand.Rand) []byte {
	if len(data) <= 1 {
		return data
	}
	maxBlockSize := min(32, len(data)/2)
	if maxBlockSize < 1 {
		maxBlockSize = 1
	}
	blockLen := rng.Intn(maxBlockSize) + 1
	pos := rng.Intn(len(data) - blockLen + 1)
	result := make([]byte, 0, len(data)-blockLen)
	result = append(result, data[:pos]...)
	result = append(result, data[pos+blockLen:]...)
	return result
}

// BlockDuplicate duplicates a block at its current position.
func BlockDuplicate(data []byte, rng *rand.Rand) []byte {
	maxBlockSize := min(32, len(data)/2+1)
	if maxBlockSize < 1 {
		maxBlockSize = 1
	}
	blockLen := rng.Intn(maxBlockSize) + 1
	pos := rng.Intn(len(data))
	end := min(pos+blockLen, len(data))
	block := data[pos:end]
	result := make([]byte, 0, len(data)+len(block))
	result = append(result, data[:end]...)
	result = append(result, block...)
	result = append(result, data[end:]...)
	return result
}

// BlockReplace replaces a block with random bytes.
func BlockReplace(data []byte, rng *rand.Rand) []byte {
	maxBlockSize := min(32, len(data))
	if maxBlockSize < 1 {
		return data
	}
	blockLen := rng.Intn(maxBlockSize) + 1
	pos := rng.Intn(len(data) - blockLen + 1)
	rng.Read(data[pos : pos+blockLen])
	return data
}

// Splice combines parts of two byte slices.
func Splice(a, b []byte, rng *rand.Rand) []byte {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	splitA := rng.Intn(len(a))
	splitB := rng.Intn(len(b))
	result := make([]byte, 0, splitA+len(b)-splitB)
	result = append(result, a[:splitA]...)
	result = append(result, b[splitB:]...)
	return result
}
