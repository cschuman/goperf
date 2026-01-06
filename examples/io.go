// Package examples contains sample code demonstrating performance anti-patterns.
package examples

import (
	"bufio"
	"io"
	"os"
)

// UnbufferedRead reads a file byte-by-byte.
// This causes excessive system calls.
func UnbufferedRead(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var data []byte
	buf := make([]byte, 1) // BAD: tiny buffer
	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if n > 0 {
			data = append(data, buf[0])
		}
	}
	return data, nil
}

// BufferedRead uses a buffered reader for efficiency.
func BufferedRead(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file) // GOOD: buffered I/O
	return io.ReadAll(reader)
}

// SmallBufferCopy uses a tiny buffer for copying.
func SmallBufferCopy(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 64) // BAD: very small buffer
	_, err := io.CopyBuffer(dst, src, buf)
	return err
}

// OptimalBufferCopy uses an appropriately sized buffer.
func OptimalBufferCopy(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 32*1024) // GOOD: 32KB buffer
	_, err := io.CopyBuffer(dst, src, buf)
	return err
}

// ReadFileInLoop opens and reads a file on each iteration.
func ReadFileInLoop(filename string, count int) error {
	for i := 0; i < count; i++ {
		// BAD: opening file repeatedly
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		_ = data
	}
	return nil
}

// ReadFileOnce reads the file once and reuses the data.
func ReadFileOnce(filename string, count int) error {
	// GOOD: read once, use many times
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	for i := 0; i < count; i++ {
		_ = data
	}
	return nil
}
