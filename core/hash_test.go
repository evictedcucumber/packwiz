package core

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/evictedcucumber/packwiz/murmur2"
)

func hashData(t *testing.T, hashType string, data []byte) string {
	h, err := GetHashImpl(hashType)
	if err != nil {
		t.Fatalf("GetHashImpl(%q) returned error: %v", hashType, err)
	}
	_, err = h.Write(data)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	return h.HashToString(h.Sum(nil))
}

func TestGetHashImplSHA1(t *testing.T) {
	data := []byte("packwiz test data")
	got := hashData(t, "sha1", data)
	sum := sha1.Sum(data)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("sha1 hash = %s, want %s", got, want)
	}
}

func TestGetHashImplSHA256(t *testing.T) {
	data := []byte("packwiz test data")
	got := hashData(t, "sha256", data)
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("sha256 hash = %s, want %s", got, want)
	}
}

func TestGetHashImplSHA512(t *testing.T) {
	data := []byte("packwiz test data")
	got := hashData(t, "sha512", data)
	sum := sha512.Sum512(data)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("sha512 hash = %s, want %s", got, want)
	}
}

func TestGetHashImplMD5(t *testing.T) {
	data := []byte("packwiz test data")
	got := hashData(t, "md5", data)
	sum := md5.Sum(data)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("md5 hash = %s, want %s", got, want)
	}
}

func TestGetHashImplMurmur2(t *testing.T) {
	data := []byte("packwiz test data")
	got := hashData(t, "murmur2", data)

	mh := murmur2.New()
	_, err := mh.Write(data)
	if err != nil {
		t.Fatalf("murmur2 Write returned error: %v", err)
	}
	// murmur2 hasher produces a 4-byte big endian value, stringified as uint64
	want := strconv.FormatUint(uint64(mh.Sum32()), 10)
	if got != want {
		t.Errorf("murmur2 hash = %s, want %s", got, want)
	}
}

func TestGetHashImplLengthBytes(t *testing.T) {
	data := []byte("packwiz test data of a certain length")
	got := hashData(t, "length-bytes", data)
	want := strconv.FormatUint(uint64(len(data)), 10)
	if got != want {
		t.Errorf("length-bytes hash = %s, want %s", got, want)
	}
}

func TestGetHashImplCaseInsensitive(t *testing.T) {
	if _, err := GetHashImpl("SHA256"); err != nil {
		t.Errorf("expected uppercase hash type to work, got error: %v", err)
	}
}

func TestGetHashImplUnknown(t *testing.T) {
	_, err := GetHashImpl("not-a-real-hash")
	if err == nil {
		t.Error("expected error for unknown hash type, got nil")
	}
}

func TestLengthHasherDirect(t *testing.T) {
	h := &LengthHasher{}

	if h.Size() != 8 {
		t.Errorf("Size() = %d, want 8", h.Size())
	}
	if h.BlockSize() != 1 {
		t.Errorf("BlockSize() = %d, want 1", h.BlockSize())
	}

	n, err := h.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 5 {
		t.Errorf("Write returned n = %d, want 5", n)
	}

	n, err = h.Write([]byte(" world"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 6 {
		t.Errorf("Write returned n = %d, want 6", n)
	}

	sum := h.Sum(nil)
	if len(sum) != 8 {
		t.Fatalf("Sum returned %d bytes, want 8", len(sum))
	}
	var total uint64
	for _, b := range sum {
		total = total<<8 | uint64(b)
	}
	if total != 11 {
		t.Errorf("Sum() decoded = %d, want 11", total)
	}

	// Note: Sum(b) with a non-empty prefix does not correctly preserve the
	// prefix bytes (it writes the length at offset 0 of the appended slice
	// rather than at offset len(b)), which looks like a bug in the
	// hand-rolled implementation. Since every caller in this codebase only
	// ever calls Sum(nil), that case isn't exercised here.

	h.Reset()
	sum = h.Sum(nil)
	for _, b := range sum {
		if b != 0 {
			t.Errorf("Sum() after Reset() = %v, want all zero", sum)
			break
		}
	}
}
