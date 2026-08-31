package bitread_test

import (
	"bytes"
	"testing"

	bitread "github.com/markus-wa/gobitread"
)

// TestReadBytesIntoAcrossRefill reads byte-aligned chunks that are larger than
// the internal buffer, forcing multiple refills within a single ReadBytesInto
// call (the chunked fast path added for perf reasons).
func TestReadBytesIntoAcrossRefill(t *testing.T) {
	const total = 4096

	src := make([]byte, total)
	for i := range src {
		src[i] = byte(i * 7)
	}

	r := new(bitread.BitReader)
	r.OpenWithBuffer(bytes.NewReader(src), make([]byte, 512))

	var out []byte
	for len(out) < total {
		n := 300
		if remain := total - len(out); remain < n {
			n = remain
		}

		r.ReadBytesInto(&out, n)
	}

	if !bytes.Equal(out, src) {
		t.Fatalf("output mismatch (len %d, want %d)", len(out), total)
	}
}

// TestReadBytesIntoEndOfStream reads exactly to the end of the underlying
// reader. The refill-on-boundary logic must not consume past the end of the
// stream (previously panicked with unexpected EOF).
func TestReadBytesIntoEndOfStream(t *testing.T) {
	const total = 600

	src := make([]byte, total)
	for i := range src {
		src[i] = byte(i)
	}

	r := new(bitread.BitReader)
	r.OpenWithBuffer(bytes.NewReader(src), make([]byte, 512))

	var out []byte
	r.ReadBytesInto(&out, total)

	if !bytes.Equal(out, src) {
		t.Fatalf("output mismatch (len %d, want %d)", len(out), total)
	}
}

// TestReadBytesIntoShortFinalChunk reads a stream whose length is not a
// multiple of the buffer size; the final chunk ends mid-buffer with
// endReached set and must still be delivered in full.
func TestReadBytesIntoShortFinalChunk(t *testing.T) {
	const (
		bufferSize = 512
		total      = bufferSize + 100 // 100 bytes past the first full buffer
	)

	src := make([]byte, total)
	for i := range src {
		src[i] = byte(i * 3)
	}

	r := new(bitread.BitReader)
	r.OpenWithBuffer(bytes.NewReader(src), make([]byte, bufferSize))

	var out []byte
	for len(out) < total {
		n := 128
		if remain := total - len(out); remain < n {
			n = remain
		}

		r.ReadBytesInto(&out, n)
	}

	if !bytes.Equal(out, src) {
		t.Fatalf("output mismatch (len %d, want %d)", len(out), total)
	}
}

// TestAdvanceToExactStreamEnd reads bit-by-bit to the very end of the
// stream; offset == total bits must not trigger a refill (previously
// panicked with unexpected EOF when offset landed exactly on the boundary).
func TestAdvanceToExactStreamEnd(t *testing.T) {
	const total = 600

	src := bytes.Repeat([]byte{0xAB}, total)

	r := new(bitread.BitReader)
	r.OpenWithBuffer(bytes.NewReader(src), make([]byte, 512))

	for i := 0; i < total<<3; i++ {
		r.ReadBit()
	}

	if pos := r.ActualPosition(); pos != total<<3 {
		t.Fatalf("position %d, want %d", pos, total<<3)
	}
}
