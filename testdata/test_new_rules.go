package testdata

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"runtime/pprof"
	"time"
)

// Test context rules
func handleRequest(w http.ResponseWriter, r *http.Request) {
	// BAD: context.Background in handler
	ctx := context.Background()
	doWork(ctx)

	// BAD: missing timeout
	ctx2, cancel := context.WithCancel(context.Background())
	_ = ctx2
	_ = cancel
	// No defer cancel() - context leak
}

// Test database rules
func initDB() {
	// BAD: no pool configuration
	db, _ := sql.Open("postgres", "connection-string")
	_ = db
}

func badPoolConfig() {
	db, _ := sql.Open("postgres", "connection-string")
	// BAD: unlimited connections
	db.SetMaxOpenConns(0)
}

// Test memory rules
func processItems(items []LargeStruct) {
	for _, item := range items {
		// BAD: large struct passed by value
		processLargeStruct(item)
	}
}

type LargeStruct struct {
	Field1  [100]byte
	Field2  [100]byte
	Field3  [100]byte
	Field4  [100]byte
	Field5  string
	Field6  string
	Field7  string
	Field8  string
	Field9  string
	Field10 string
}

func processLargeStruct(s LargeStruct) {}

func hotPath() {
	// BAD: pprof in hot path
	for i := 0; i < 1000; i++ {
		pprof.Lookup("heap")
	}
}

// Test time rules
func parseTimesInLoop(dates []string) {
	for _, d := range dates {
		// BAD: time.Parse in loop
		t, _ := time.Parse("2006-01-02", d)
		_ = t
	}
}

func loadLocationInLoop(zones []string) {
	for _, zone := range zones {
		// BAD: time.LoadLocation in loop
		loc, _ := time.LoadLocation(zone)
		_ = loc
	}
}

// Test HTTP rules
func handleUpload(w http.ResponseWriter, r *http.Request) {
	// BAD: no MaxBytesReader
	body := r.Body
	_ = body
}

func makeRequest() {
	// BAD: missing body close
	resp, _ := http.Get("http://example.com")
	_ = resp
	// No defer resp.Body.Close()
}

// Test cache rules
func validateInLoop(items []string) {
	for _, item := range items {
		// BAD: regexp.MatchString in loop
		matched, _ := regexp.MatchString(`^\d+$`, item)
		_ = matched
	}
}

func compileInFunc() {
	// BAD: regexp.MustCompile inside function
	re := regexp.MustCompile(`\d+`)
	_ = re
}

// Test error wrapping rules
func processWithErrors(items []string) error {
	for _, item := range items {
		// BAD: fmt.Errorf in loop
		err := fmt.Errorf("failed to process %s", item)
		_ = err
	}
	return nil
}

// Test interface boxing rules
func logInLoop(items []int) {
	for _, item := range items {
		// BAD: Printf with many args in loop
		fmt.Printf("item: %d, squared: %d, cubed: %d, fourth: %d\n",
			item, item*item, item*item*item, item*item*item*item)
	}
}

// Helper to avoid unused warnings
func doWork(ctx context.Context) {}
