// Package examples contains sample code demonstrating performance anti-patterns.
package examples

import (
	"io"
	"net/http"
)

// HTTPClientInLoop creates a new client for each request.
// This defeats connection pooling and reuse.
func HTTPClientInLoop(urls []string) error {
	for _, url := range urls {
		client := &http.Client{} // BAD: new client each iteration
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return nil
}

// HTTPClientReused shares a single client across requests.
func HTTPClientReused(urls []string) error {
	client := &http.Client{} // GOOD: client reused
	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return nil
}

// ResponseBodyNotClosed forgets to close the response body.
// This leaks connections and file descriptors.
func ResponseBodyNotClosed(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	// BAD: resp.Body never closed
	return io.ReadAll(resp.Body)
}

// ResponseBodyClosed properly closes the response body.
func ResponseBodyClosed(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // GOOD: always close
	return io.ReadAll(resp.Body)
}

// ResponseBodyNotDrained doesn't fully read the body.
// This prevents connection reuse in the pool.
func ResponseBodyNotDrained(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// BAD: body not fully read, connection can't be reused
	return nil
}

// ResponseBodyDrained fully reads and discards the body.
func ResponseBodyDrained(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // GOOD: drain body for reuse
	return nil
}
