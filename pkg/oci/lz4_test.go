package oci

import (
	"bytes"
	"io"
	"testing"

	"github.com/pierrec/lz4/v4"
)

func TestDecompressLZ4(t *testing.T) {
	// Create test data
	original := []byte("Hello, LZ4 compression! This is test data for decompression.")

	// Compress the data with LZ4
	var compressed bytes.Buffer
	w := lz4.NewWriter(&compressed)
	if _, err := w.Write(original); err != nil {
		t.Fatalf("Failed to compress test data: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close LZ4 writer: %v", err)
	}

	// Test decompression
	reader := bytes.NewReader(compressed.Bytes())
	decompressed, err := DecompressLZ4(reader)
	if err != nil {
		t.Fatalf("DecompressLZ4 failed: %v", err)
	}
	defer decompressed.Close()

	// Read the decompressed data
	result, err := io.ReadAll(decompressed)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	// Verify
	if !bytes.Equal(result, original) {
		t.Errorf("Decompressed data mismatch.\nGot:  %q\nWant: %q", result, original)
	}
}

func TestDecompressLZ4_LargeData(t *testing.T) {
	// Create larger test data (1MB)
	original := make([]byte, 1024*1024)
	for i := range original {
		original[i] = byte(i % 256)
	}

	// Compress the data
	var compressed bytes.Buffer
	w := lz4.NewWriter(&compressed)
	if _, err := w.Write(original); err != nil {
		t.Fatalf("Failed to compress test data: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close LZ4 writer: %v", err)
	}

	// Test decompression
	reader := bytes.NewReader(compressed.Bytes())
	decompressed, err := DecompressLZ4(reader)
	if err != nil {
		t.Fatalf("DecompressLZ4 failed: %v", err)
	}
	defer decompressed.Close()

	// Read the decompressed data
	result, err := io.ReadAll(decompressed)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	// Verify size and content
	if len(result) != len(original) {
		t.Errorf("Decompressed size = %d, want %d", len(result), len(original))
	}
	if !bytes.Equal(result, original) {
		t.Error("Decompressed data does not match original")
	}
}

func TestDecompressLZ4_EmptyData(t *testing.T) {
	// Compress empty data
	var compressed bytes.Buffer
	w := lz4.NewWriter(&compressed)
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close LZ4 writer: %v", err)
	}

	// Test decompression
	reader := bytes.NewReader(compressed.Bytes())
	decompressed, err := DecompressLZ4(reader)
	if err != nil {
		t.Fatalf("DecompressLZ4 failed: %v", err)
	}
	defer decompressed.Close()

	// Read the decompressed data
	result, err := io.ReadAll(decompressed)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d bytes", len(result))
	}
}

func TestNewLZ4ReadCloser(t *testing.T) {
	// Create test data
	original := []byte("Test data for LZ4ReadCloser")

	// Compress
	var compressed bytes.Buffer
	w := lz4.NewWriter(&compressed)
	w.Write(original)
	w.Close()

	// Create read closer
	innerRC := io.NopCloser(bytes.NewReader(compressed.Bytes()))
	lz4RC := NewLZ4ReadCloser(innerRC)

	// Read
	result, err := io.ReadAll(lz4RC)
	if err != nil {
		t.Fatalf("Failed to read from LZ4ReadCloser: %v", err)
	}

	// Verify
	if !bytes.Equal(result, original) {
		t.Errorf("Data mismatch.\nGot:  %q\nWant: %q", result, original)
	}

	// Close should not error
	if err := lz4RC.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}
