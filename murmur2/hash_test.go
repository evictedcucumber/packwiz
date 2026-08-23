package murmur2

import (
	"testing"

	"github.com/aviddiviner/go-murmur"
)

func TestWhitespaceIsStripped(t *testing.T) {
	h1 := New()
	_, _ = h1.Write([]byte("hello world\tfoo\nbar\r"))

	h2 := New()
	_, _ = h2.Write([]byte("helloworldfoobar"))

	if h1.Sum32() != h2.Sum32() {
		t.Errorf("expected whitespace-stripped hashes to match: %d != %d", h1.Sum32(), h2.Sum32())
	}
}

func TestWriteChunkingDoesNotAffectHash(t *testing.T) {
	data := []byte("the quick brown fox jumps over the lazy dog")

	whole := New()
	_, _ = whole.Write(data)

	chunked := New()
	for _, b := range data {
		_, _ = chunked.Write([]byte{b})
	}

	if whole.Sum32() != chunked.Sum32() {
		t.Errorf("expected chunked writes to produce the same hash as a single write: %d != %d", chunked.Sum32(), whole.Sum32())
	}
}

func TestMatchesUnderlyingMurmurHash2(t *testing.T) {
	// Murmur2CF strips whitespace before hashing, so the expected value must
	// be computed from the whitespace-stripped bytes.
	data := []byte("packwiztestdata123")

	h := New()
	_, _ = h.Write(data)

	expected := murmur.MurmurHash2(data, 1)
	if h.Sum32() != expected {
		t.Errorf("expected %d, got %d", expected, h.Sum32())
	}
}

func TestDifferentInputsHashDifferently(t *testing.T) {
	h1 := New()
	_, _ = h1.Write([]byte("input one"))

	h2 := New()
	_, _ = h2.Write([]byte("input two"))

	if h1.Sum32() == h2.Sum32() {
		t.Error("expected different inputs to produce different hashes")
	}
}

func TestReset(t *testing.T) {
	h := New()
	_, _ = h.Write([]byte("some data"))
	h.Reset()

	fresh := New()

	if h.Sum32() != fresh.Sum32() {
		t.Errorf("expected reset hash to match a fresh hash: %d != %d", h.Sum32(), fresh.Sum32())
	}
}

func TestSizeAndBlockSize(t *testing.T) {
	h := New()
	if h.Size() != 4 {
		t.Errorf("expected Size() to be 4, got %d", h.Size())
	}
	if h.BlockSize() != 4 {
		t.Errorf("expected BlockSize() to be 4, got %d", h.BlockSize())
	}
}

func TestSumNilAllocatesFourBytes(t *testing.T) {
	h := New()
	_, _ = h.Write([]byte("nil sum test"))

	result := h.Sum(nil)
	if len(result) != 4 {
		t.Fatalf("expected 4 bytes, got %d", len(result))
	}
}
