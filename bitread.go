// Package bitread provides a bit level reader
package bitread

import (
	"bytes"
	"encoding/binary"
	"io"
)

const (
	sled = 4
)

// A simple int stack
type stack []int

// push returns a stack with the value v added on top of the original stack
func (s stack) push(v int) stack {
	return append(s, v)
}

// pop removes the last added item from the stack.
// Returns the new stack and the item that was removed.
// Attention: panics when the stack is empty
func (s stack) pop() (stack, int) {
	// FIXME: CBA to handle empty stacks rn
	l := len(s)
	return s[:l-1], s[l-1]
}

// top returns the top element without removing it
func (s stack) top() int {
	return s[len(s)-1]
}

type BitReader struct {
	underlying   io.Reader
	buffer       []byte
	offset       int
	bitsInBuffer int
	lazyPosition int
	chunkTargets stack
}

// LazyPosition returns the offset at the time of the last time the buffer was refilled
func (r *BitReader) LazyPosition() int {
	return r.lazyPosition
}

// ActualPosition returns the offset from the start in bits
func (r *BitReader) ActualPosition() int {
	return r.lazyPosition + r.offset
}

// Open sets the underlying io.Reader and internal buffer, making the reader ready to use.
// bufferSize is in bytes
func (r *BitReader) Open(underlying io.Reader, bufferSize int) {
	r.OpenWithBuffer(underlying, make([]byte, bufferSize))
}

// OpenWithBuffer is like Open but allows to provide the internal byte buffer.
// Could be useful to pool buffers of short living BitReaders for example
func (r *BitReader) OpenWithBuffer(underlying io.Reader, buffer []byte) {
	r.underlying = underlying
	r.buffer = buffer
	r.refillBuffer()
	r.offset = sled << 3
}

// Close resets the BitReader. Open() may be used again after Close()
func (r *BitReader) Close() {
	r.underlying = nil
	r.buffer = nil
	r.offset = 0
	r.bitsInBuffer = 0
	r.chunkTargets = stack{}
	r.lazyPosition = 0
}

// ReadBit reads a single bit
func (r *BitReader) ReadBit() bool {
	res := (r.buffer[r.offset>>3] & (1 << uint(r.offset&7))) != 0
	r.advance(1)
	return res
}

// ReadBits reads n bits into a []byte.
func (r *BitReader) ReadBits(n uint) []byte {
	b := make([]byte, (n+7)>>3)
	bitLevel := r.offset&7 != 0
	for i := uint(0); i < n>>3; i++ {
		b[i] = r.readByteInternal(bitLevel)
	}
	if n&7 != 0 {
		b[n>>3] = r.ReadBitsToByte(n & 7)
	}
	return b
}

// ReadSingleByte reads one byte
// Not called ReadByte as it does not comply with the standard library interface
func (r *BitReader) ReadSingleByte() byte {
	return r.readByteInternal(r.offset&7 != 0)
}

func (r *BitReader) readByteInternal(bitLevel bool) byte {
	if !bitLevel {
		res := r.buffer[r.offset>>3]
		r.advance(8)
		return res
	}
	return r.ReadBitsToByte(8)
}

// ReadBitsToByte reads n bits into a byte.
// Undefined for n > 8.
func (r *BitReader) ReadBitsToByte(n uint) byte {
	return byte(r.ReadInt(n))
}

// ReadInt reads the next n bits as an int.
// Undefined for n > 32
func (r *BitReader) ReadInt(n uint) uint {
	val := binary.LittleEndian.Uint64(r.buffer[r.offset>>3&^3:])
	res := uint(val << (64 - (uint(r.offset) & 31) - n) >> (64 - n))
	// Advance after using offset!
	r.advance(n)
	return res
}

// ReadBytes reads n bytes.
// Ease of use wrapper for ReadBytesInto()
func (r *BitReader) ReadBytes(n int) []byte {
	res := make([]byte, 0, n)
	r.ReadNBytesInto(&res, n)
	return res
}

// ReadBytesInto reads cap(out) bytes into out
func (r *BitReader) ReadBytesInto(out *[]byte) {
	r.ReadNBytesInto(out, cap(*out))
}

// ReadNBytesInto reads n bytes into out
// Useful for pooling []byte slices
func (r *BitReader) ReadNBytesInto(out *[]byte, n int) {
	bitLevel := r.offset&7 != 0
	if !bitLevel && r.offset+(n<<3) < r.bitsInBuffer {
		// Shortcut if offset%8 = 0 and all bytes are already buffered
		*out = append(*out, r.buffer[r.offset>>3:r.offset>>3+n]...)
		r.advance(uint(n) << 3)
	} else {
		for i := 0; i < n; i++ {
			*out = append(*out, r.readByteInternal(bitLevel))
		}
	}
}

// ReadCString reads n bytes as characters into a C string.
// String is terminated by zero
func (r *BitReader) ReadCString(n int) string {
	b := r.ReadBytes(n)
	end := bytes.IndexByte(b, 0)
	if end < 0 {
		end = n
	}
	return string(b[:end])
}

// ReadSignedInt is like ReadInt but returns signed int.
// Undefined for n > 32
func (r *BitReader) ReadSignedInt(n uint) int {
	val := binary.LittleEndian.Uint64(r.buffer[r.offset>>3&^3:])
	// Cast to int64 before right shift & use offset before advance
	res := int(int64(val<<(64-(uint(r.offset)&31)-n)) >> (64 - n))
	r.advance(n)
	return res
}

// BeginChunk starts a new chunk with lenght bytes
// Useful to make sure the position in the bit stream is correct
func (r *BitReader) BeginChunk(length int) {
	r.chunkTargets = r.chunkTargets.push(r.ActualPosition() + length)
}

// EndChunk attempts to 'end' the last chunk
// Seeks to the end of the chunk if not already reached
// Panics if the chunk boundary was exceeded while reading
func (r *BitReader) EndChunk() {
	var target int
	r.chunkTargets, target = r.chunkTargets.pop()
	delta := target - r.ActualPosition()
	if delta < 0 {
		panic("Someone read beyond a chunk boundary, what a dick")
	} else if delta > 0 {
		// Seek for the end of the chunk
		seeker, ok := r.underlying.(io.Seeker)
		if ok {
			bufferBits := r.bitsInBuffer - r.offset
			if delta > bufferBits+sled<<3 {
				unbufferedSkipBits := delta - bufferBits
				seeker.Seek(int64((unbufferedSkipBits>>3)-sled), io.SeekCurrent)

				newBytes, _ := r.underlying.Read(r.buffer)

				r.bitsInBuffer = (newBytes - sled) << 3
				if newBytes <= sled {
					// TODO: Maybe do this even if newBytes is <= bufferSize - sled like in refillBuffer
					// Consume sled
					// Shouldn't really happen unless we reached the end of the stream
					// In that case bitsInBuffer should be 0 after this line (newBytes=0 - sled + sled)
					r.bitsInBuffer += sled << 3
				}

				r.offset = unbufferedSkipBits & 7
				r.lazyPosition = target - r.offset
			} else {
				// no seek necessary
				r.advance(uint(delta))
			}
		} else {
			// Canny seek, do it manually
			r.advance(uint(delta))
		}
	}
}

// ChunkFinished returns true if the current position is at the end of the chunk
func (r *BitReader) ChunkFinished() bool {
	return r.chunkTargets.top() == r.ActualPosition()
}

func (r *BitReader) advance(bits uint) {
	r.offset += int(bits)
	for r.offset >= r.bitsInBuffer {
		// Refill if we reached the sled
		r.refillBuffer()
	}
}

func (r *BitReader) refillBuffer() {
	// Copy sled to beginning
	copy(r.buffer[0:sled], r.buffer[r.bitsInBuffer>>3:(r.bitsInBuffer>>3)+sled])

	r.offset -= r.bitsInBuffer // sled bits used remain in offset
	r.lazyPosition += r.bitsInBuffer

	newBytes, _ := r.underlying.Read(r.buffer[sled:])

	r.bitsInBuffer = newBytes << 3
	if newBytes < len(r.buffer)-2*sled {
		// we're done here, consume sled
		r.bitsInBuffer += sled << 3
	}
}
