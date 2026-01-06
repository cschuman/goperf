package rules

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestContextBackgroundInHandler(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantHits int
	}{
		{
			name: "context.Background in HTTP handler",
			code: `package main

import (
	"context"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background() // should flag
	_ = ctx
}
`,
			wantHits: 1,
		},
		{
			name: "context.TODO in HTTP handler",
			code: `package main

import (
	"context"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.TODO() // should flag
	_ = ctx
}
`,
			wantHits: 1,
		},
		{
			name: "context.Background in non-handler",
			code: `package main

import "context"

func regularFunc() {
	ctx := context.Background() // should NOT flag - not a handler
	_ = ctx
}
`,
			wantHits: 0,
		},
		{
			name: "r.Context() in handler - correct pattern",
			code: `package main

import "net/http"

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context() // correct - should NOT flag
	_ = ctx
}
`,
			wantHits: 0,
		},
	}

	rule := &ContextBackgroundInHandlerRule{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.code, 0)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			issues := rule.Check(f, fset, []byte(tt.code))
			if len(issues) != tt.wantHits {
				t.Errorf("got %d issues, want %d", len(issues), tt.wantHits)
				for _, issue := range issues {
					t.Logf("  issue: %s at line %d", issue.Message, issue.Line)
				}
			}
		})
	}
}

func TestContextLeak(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantHits int
	}{
		{
			name: "uncalled cancel function",
			code: `package main

import "context"

func foo() {
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx
	// cancel is never called - should flag
	_ = cancel
}
`,
			wantHits: 1,
		},
		{
			name: "cancel called via defer",
			code: `package main

import "context"

func foo() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
}
`,
			wantHits: 0,
		},
		{
			name: "cancel called directly",
			code: `package main

import "context"

func foo() {
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx
	cancel()
}
`,
			wantHits: 0,
		},
		{
			name: "cancel assigned to underscore - ignored",
			code: `package main

import "context"

func foo() {
	ctx, _ := context.WithCancel(context.Background())
	_ = ctx
}
`,
			wantHits: 0,
		},
		{
			name: "WithTimeout uncalled",
			code: `package main

import (
	"context"
	"time"
)

func foo() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = ctx
	// cancel is never called - should flag
	_ = cancel
}
`,
			wantHits: 1,
		},
		{
			name: "function with same name as cancel - no false positive",
			code: `package main

import "context"

func cancel() {}

func foo() {
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel() // calls the correct cancel
	_ = ctx
	cancel() // different function - should not affect detection
}
`,
			wantHits: 0,
		},
	}

	rule := &ContextLeakRule{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.code, 0)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			issues := rule.Check(f, fset, []byte(tt.code))
			if len(issues) != tt.wantHits {
				t.Errorf("got %d issues, want %d", len(issues), tt.wantHits)
				for _, issue := range issues {
					t.Logf("  issue: %s at line %d", issue.Message, issue.Line)
				}
			}
		})
	}
}

func TestIsHTTPHandler(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{
			name: "standard http handler",
			code: `package main
import "net/http"
func handler(w http.ResponseWriter, r *http.Request) {}
`,
			want: true,
		},
		{
			name: "echo handler",
			code: `package main
import "github.com/labstack/echo/v4"
func handler(c echo.Context) error { return nil }
`,
			want: true,
		},
		{
			name: "gin handler",
			code: `package main
import "github.com/gin-gonic/gin"
func handler(c *gin.Context) {}
`,
			want: true,
		},
		{
			name: "fiber handler",
			code: `package main
import "github.com/gofiber/fiber/v2"
func handler(c *fiber.Ctx) error { return nil }
`,
			want: true,
		},
		{
			name: "regular function",
			code: `package main
func regularFunc(a int, b string) {}
`,
			want: false,
		},
		{
			name: "function with context param (not http)",
			code: `package main
import "context"
func regularFunc(ctx context.Context) {}
`,
			want: true, // This will match due to "Context" suffix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.code, 0)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			var funcDecl *ast.FuncDecl
			for _, decl := range f.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok {
					funcDecl = fd
					break
				}
			}

			if funcDecl == nil {
				t.Fatal("no function found")
			}

			got := isHTTPHandler(funcDecl)
			if got != tt.want {
				t.Errorf("isHTTPHandler() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEstimateStructSizes(t *testing.T) {
	code := `package main

type SmallStruct struct {
	a int
	b int
}

type LargeStruct struct {
	a, b, c, d, e, f, g, h int64
	name string
	data []byte
}

type EmbeddedStruct struct {
	SmallStruct
	extra int
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, 0)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	sizes := estimateStructSizes(f)

	if size, ok := sizes["SmallStruct"]; !ok {
		t.Error("SmallStruct not found")
	} else if size != 16 { // 2 * 8 bytes
		t.Errorf("SmallStruct size = %d, want 16", size)
	}

	if size, ok := sizes["LargeStruct"]; !ok {
		t.Error("LargeStruct not found")
	} else if size < 64 { // 8*8 + 16 + 8 = 88 bytes minimum
		t.Errorf("LargeStruct size = %d, want >= 64", size)
	}
}

func TestGetTypeName(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{`var x int`, "int"},
		{`var x string`, "string"},
		{`var x *int`, "*int"},
		{`var x []string`, "[]string"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			fset := token.NewFileSet()
			code := "package main\n" + tt.code
			f, err := parser.ParseFile(fset, "test.go", code, 0)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			for _, decl := range f.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok {
					for _, spec := range genDecl.Specs {
						if valueSpec, ok := spec.(*ast.ValueSpec); ok {
							got := getTypeName(valueSpec.Type)
							if got != tt.want {
								t.Errorf("getTypeName() = %q, want %q", got, tt.want)
							}
						}
					}
				}
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-1, "-1"},
		{100, "100"},
		{999, "999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := itoa(tt.input)
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPprofInHotPath(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantHits int
	}{
		{
			name: "pprof in loop",
			code: `package main

import "runtime/pprof"

func foo() {
	for i := 0; i < 100; i++ {
		pprof.WriteHeapProfile(nil) // should flag
	}
}
`,
			wantHits: 1,
		},
		{
			name: "pprof outside loop",
			code: `package main

import "runtime/pprof"

func foo() {
	pprof.WriteHeapProfile(nil) // should NOT flag
}
`,
			wantHits: 0,
		},
		{
			name: "pprof in HTTP handler",
			code: `package main

import (
	"net/http"
	"runtime/pprof"
)

func handler(w http.ResponseWriter, r *http.Request) {
	pprof.WriteHeapProfile(nil) // should flag - in handler
}
`,
			wantHits: 1,
		},
	}

	rule := &PprofInHotPathRule{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.code, 0)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			issues := rule.Check(f, fset, []byte(tt.code))
			if len(issues) != tt.wantHits {
				t.Errorf("got %d issues, want %d", len(issues), tt.wantHits)
				for _, issue := range issues {
					t.Logf("  issue: %s at line %d", issue.Message, issue.Line)
				}
			}
		})
	}
}

func TestMissingContextTimeout(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantHits int
	}{
		{
			name: "http.NewRequest without context",
			code: `package main

import "net/http"

func foo() {
	req, _ := http.NewRequest("GET", "http://example.com", nil) // should flag
	_ = req
}
`,
			wantHits: 1,
		},
		{
			name: "http.NewRequestWithContext - correct",
			code: `package main

import (
	"context"
	"net/http"
)

func foo() {
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_ = req
}
`,
			wantHits: 0,
		},
	}

	rule := &MissingContextTimeoutRule{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tt.code, 0)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			issues := rule.Check(f, fset, []byte(tt.code))
			if len(issues) != tt.wantHits {
				t.Errorf("got %d issues, want %d", len(issues), tt.wantHits)
				for _, issue := range issues {
					t.Logf("  issue: %s at line %d", issue.Message, issue.Line)
				}
			}
		})
	}
}
