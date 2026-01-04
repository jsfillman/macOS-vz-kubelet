package oci

import (
	"io"

	"github.com/pierrec/lz4/v4"
)

// LZ4ReadCloser wraps an LZ4 reader with the underlying ReadCloser to ensure
// proper cleanup when closed.
type LZ4ReadCloser struct {
	reader io.Reader
	closer io.Closer
}

// NewLZ4ReadCloser creates a new LZ4 decompressing ReadCloser that wraps
// the provided ReadCloser. When closed, it will close the underlying stream.
func NewLZ4ReadCloser(rc io.ReadCloser) *LZ4ReadCloser {
	return &LZ4ReadCloser{
		reader: lz4.NewReader(rc),
		closer: rc,
	}
}

// Read implements io.Reader by reading decompressed data.
func (r *LZ4ReadCloser) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

// Close implements io.Closer by closing the underlying stream.
func (r *LZ4ReadCloser) Close() error {
	return r.closer.Close()
}

// DecompressLZ4 creates an LZ4 decompressing reader from the provided reader.
// The returned ReadCloser must be closed when done reading.
func DecompressLZ4(r io.Reader) (io.ReadCloser, error) {
	lz4Reader := lz4.NewReader(r)
	return &lz4ReaderCloser{reader: lz4Reader}, nil
}

// lz4ReaderCloser wraps an LZ4 reader to implement io.ReadCloser.
type lz4ReaderCloser struct {
	reader *lz4.Reader
}

func (r *lz4ReaderCloser) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

func (r *lz4ReaderCloser) Close() error {
	// LZ4 reader doesn't have a Close method, so this is a no-op.
	return nil
}
