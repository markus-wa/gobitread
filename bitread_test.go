package bitread_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"testing"

	"github.com/markus-wa/gobitread"
)

func TestReadBit(t *testing.T) {
	b := make([]byte, 0xff)
	for n := byte(0); n < byte(len(b)); n++ {
		b[n] = n
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	res := make([]byte, 8)
	var exp string
	for i := 0; i < len(b); i++ {
		for i := 0; i < 8; i++ {
			// Least significant bit first
			if br.ReadBit() {
				res[7-i] = '1'
			} else {
				res[7-i] = '0'
			}
		}

		exp = fmt.Sprintf("%b", b[i])
		// Pad cut off bits
		for len(exp) < 8 {
			exp = "0" + exp
		}
		if string(res) != exp {
			t.Fatalf("Expected %s got %s", exp, res)
		}
	}
}

func TestReadBytes(t *testing.T) {
	b := make([]byte, 1<<8)
	for n := 0; n < len(b); n++ {
		b[n] = byte(n)
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	r := br.ReadBytes(len(b))

	for i := 0; i < len(b); i++ {
		if b[i] != r[i] {
			t.Fatalf("Expected %b got %b", b[i], r[i])
		}
	}
}

func TestReadBytesBuffered(t *testing.T) {
	b := make([]byte, 1<<8)
	for n := 0; n < len(b); n++ {
		b[n] = byte(n)
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), len(b)<<1) // Make buffer large enough to hold all bytes

	r := br.ReadBytes(len(b))

	for i := 0; i < len(b); i++ {
		if b[i] != r[i] {
			t.Fatalf("Expected %b got %b", b[i], r[i])
		}
	}
}

func TestReadCString(t *testing.T) {
	s := "test"
	b := []byte(s)

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	r := br.ReadCString(len(s))
	if r != s {
		t.Fatalf("Expected %q got %q", s, r)
	}
}

func TestBitsToByte(t *testing.T) {
	b := make([]byte, 0x0f)
	for n := byte(0); n < byte(len(b)); n++ {
		b[n] = n
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	for i := byte(0); i < byte(len(b)); i++ {
		r := br.ReadBitsToByte(4)
		if r != b[i] {
			t.Fatalf("Expected %b got %b", b[i], r)
		}
		br.ReadBitsToByte(4)
	}
}

func TestReadBits(t *testing.T) {
	nums := []uint64{0xa3dc, 0x48c1}
	b := make([]byte, len(nums)<<3)
	for i := 0; i < len(nums); i++ {
		binary.LittleEndian.PutUint64(b[i<<3:], nums[i])
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	for i := 0; i < len(nums); i++ {
		r := binary.LittleEndian.Uint64(br.ReadBits(64))
		if r != nums[i] {
			t.Fatalf("Expected %d got %d", nums[0], r)
		}
	}
}

func TestReadInt(t *testing.T) {
	nums := []uint32{0, math.MaxUint32, 0x61cb83f0}
	b := make([]byte, len(nums)<<2)
	for i := 0; i < len(nums); i++ {
		binary.LittleEndian.PutUint32(b[i<<2:], nums[i])
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	for i := 0; i < len(nums); i++ {
		r := br.ReadInt(32)
		if r != uint(nums[i]) {
			t.Fatalf("Expected %q got %q", nums[i], r)
		}
	}
}

func TestReadSignedInt(t *testing.T) {
	nums := []int32{math.MaxInt32, math.MinInt32, 0, 0x4ac71bf}
	b := make([]byte, len(nums)<<2)
	for i := 0; i < len(nums); i++ {
		binary.LittleEndian.PutUint32(b[i<<2:], uint32(nums[i]))
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	for i := 0; i < len(nums); i++ {
		r := br.ReadSignedInt(32)
		if r != int(nums[i]) {
			t.Fatalf("Expected %q got %q", nums[i], r)
		}
	}
}

func TestPositions(t *testing.T) {
	b := []byte{0xac}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	// Positions
	if br.LazyPosition() != 0 {
		t.Fatalf("LazyPosition is %d expected 0", br.LazyPosition())
	}

	if br.ActualPosition() != 0 {
		t.Fatalf("ActualPostition is %d expected 0", br.ActualPosition())
	}

	br.ReadBit()

	if br.LazyPosition() != 0 {
		t.Fatalf("LazyPosition is %d expected 0", br.LazyPosition())
	}

	if br.ActualPosition() != 1 {
		t.Fatalf("ActualPostition is %d expected 1", br.ActualPosition())
	}
}

// Tests Open, Close & Chunks
func TestLifecycle(t *testing.T) {
	b := []byte("a")

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	br.BeginChunk(2)

	br.ReadBit()
	if br.ChunkFinished() {
		t.Error("ChunkFinished() should have returned false but returned true")
	}

	br.ReadBit()
	if !br.ChunkFinished() {
		t.Error("ChunkFinished() should have returned true but returned false")
	}

	br.EndChunk()
	br.Close()

	// Test reopen
	br.Open(bytes.NewReader(b), 32)
	r := br.ReadBytes(len(b))
	for i := 0; i < len(b); i++ {
		if r[i] != b[i] {
			t.Fatal("Expected", r[i], "got", b[i])
		}
	}
}

// Tests seeking at EndChunk()
func TestChunkSeek(t *testing.T) {
	b := make([]byte, 0xff)
	for n := byte(0); n < byte(len(b)); n++ {
		b[n] = n
	}

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	br.BeginChunk(2)
	br.ReadBit()
	br.EndChunk()

	if br.ActualPosition() != 2 {
		t.Errorf("ActualPostition is %d expected 2", br.ActualPosition())
	}

	// Beyond buffer
	br.BeginChunk(32 << 3)
	br.EndChunk()

	// Check if lazyPos got updated when refilling the buffer
	if br.LazyPosition() != 32<<3 {
		t.Errorf("LazyPosition is %d expected %d", br.LazyPosition(), 16<<3)
	}

	if br.ActualPosition() != 2+(32<<3) {
		t.Errorf("ActualPostition is %d expected %d", br.ActualPosition(), 2+(16<<3))
	}

	// Get to an even position (in front of 18th byte)
	br.ReadBits(6)
	// Read 34th byte
	r := br.ReadSingleByte()
	if r != 33 {
		t.Fatalf("Expected 34th byte (33) got %d", r)
	}
}

func TestChunkExceeded(t *testing.T) {
	b := []byte("a")

	br := new(bitread.BitReader)
	br.Open(bytes.NewReader(b), 32)

	br.BeginChunk(0)
	br.ReadBit()
	func() {
		defer func() {
			err := recover()
			if err == nil {
				t.Error("EndChunk() should have paniced after reading beyond chunk boundary")
			}
		}()
		br.EndChunk()
	}()
}

// TODO: The following type of tests
// Reading beyond a chunk boundary
// Not reading all the data in a chunk (seeking)
// With and without io.Seeker as underlying
// With data in sled
// Reopen after close

// TODO: Add Skip function (alias for private advance()) or make advance public? - BeginChunk followed by endchunk already does this tho - with more overhead
