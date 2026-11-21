package bitread_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"testing/iotest"

	bitread "github.com/markus-wa/gobitread"
)

// bitsAt returns n bytes of data starting at bit pos (LSB first), as ReadInt(8) would read them.
func bitsAt(data []byte, pos, n int) []byte {
	res := make([]byte, n)
	for i := range res {
		p := pos + 8*i
		b := data[p>>3] >> uint(p&7)
		if p&7 != 0 {
			b |= data[p>>3+1] << uint(8-p&7)
		}
		res[i] = b
	}
	return res
}

// TestReadBytesIntoUnaligned checks unaligned reads against the bitstream across refills,
// buffer sizes, short reads and existing contents of out.
func TestReadBytesIntoUnaligned(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	readers := map[string]func([]byte) io.Reader{
		"full": func(b []byte) io.Reader { return bytes.NewReader(b) },
		"half": func(b []byte) io.Reader { return iotest.HalfReader(bytes.NewReader(b)) },
	}
	for name, newReader := range readers {
		for iter := 0; iter < 1000; iter++ {
			data := make([]byte, 1+rng.Intn(3000))
			rng.Read(data)
			bufSize := 24 + 8*rng.Intn(64)

			r := new(bitread.BitReader)
			r.OpenWithBuffer(newReader(data), make([]byte, bufSize))

			total := len(data) * 8
			for {
				skip := 1 + rng.Intn(7)
				pos := r.ActualPosition() + skip
				if pos+8 > total {
					break
				}
				r.ReadInt(skip)
				if pos&7 == 0 {
					continue
				}

				n := 1 + rng.Intn((total-pos)/8)
				if rng.Intn(3) > 0 && n > 40 {
					n = 1 + rng.Intn(40)
				}
				prefix := []byte{1, 2, 3}[:rng.Intn(4)]
				out := make([]byte, len(prefix), len(prefix)+rng.Intn(64))
				copy(out, prefix)

				r.ReadBytesInto(&out, n)

				want := append(append([]byte(nil), prefix...), bitsAt(data, pos, n)...)
				if !bytes.Equal(out, want) {
					t.Fatalf("%s iter %d: ReadBytesInto(%d) at bit %d (buffer %d) = %x, want %x", name, iter, n, pos, bufSize, out, want)
				}
				if got := r.ActualPosition(); got != pos+8*n {
					t.Fatalf("%s iter %d: position %d, want %d", name, iter, got, pos+8*n)
				}
			}
		}
	}
}

// A reader that returns its last bytes together with io.EOF; per-byte reads used to index past the buffer here.
func TestReadBytesIntoUnalignedDataWithEOF(t *testing.T) {
	data := make([]byte, 40)
	for i := range data {
		data[i] = byte(i*37 + 11)
	}

	r := new(bitread.BitReader)
	r.OpenWithBuffer(iotest.DataErrReader(bytes.NewReader(data)), make([]byte, 24))
	r.ReadInt(1)

	var out []byte
	r.ReadBytesInto(&out, 39)

	if want := bitsAt(data, 1, 39); !bytes.Equal(out, want) {
		t.Fatalf("got %x, want %x", out, want)
	}
}

func TestReadBytesIntoUnalignedPastEnd(t *testing.T) {
	r := new(bitread.BitReader)
	r.OpenWithBuffer(bytes.NewReader(make([]byte, 100)), make([]byte, 32))
	r.ReadInt(3)

	defer func() {
		if p := recover(); p != io.ErrUnexpectedEOF {
			t.Fatalf("reading past the end panicked with %v, want io.ErrUnexpectedEOF", p)
		}
	}()

	var out []byte
	r.ReadBytesInto(&out, 101)
}

// Many small unaligned reads appended to one slice must grow it amortized, like append.
func TestReadBytesIntoUnalignedGrowth(t *testing.T) {
	data := make([]byte, 64*1024)

	allocs := testing.AllocsPerRun(5, func() {
		r := new(bitread.BitReader)
		r.OpenWithBuffer(bytes.NewReader(data), make([]byte, 512))
		r.ReadInt(3)

		var out []byte
		for i := 0; i < 10000; i++ {
			r.ReadBytesInto(&out, 3)
		}
	})

	if allocs > 50 {
		t.Fatalf("%v allocations for 10000 appends, want amortized growth", allocs)
	}
}

func BenchmarkReadBytesIntoUnaligned(b *testing.B) {
	data := make([]byte, 1<<20)
	rand.New(rand.NewSource(1)).Read(data)

	for _, n := range []int{1, 2, 4, 8, 16, 64, 128, 256, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			r := new(bitread.BitReader)
			buf := make([]byte, 512)
			out := make([]byte, 0, n)
			b.SetBytes(int64(n))

			for i := 0; i < b.N; i++ {
				if i == 0 || r.ActualPosition()+8*n+64 > len(data)*8 {
					_ = r.Close()
					r.OpenWithBuffer(bytes.NewReader(data), buf)
					r.ReadInt(3) // whole-byte reads keep the offset unaligned
				}
				out = out[:0]
				r.ReadBytesInto(&out, n)
			}
		})
	}
}
