# Rules

This document describes every rule in the `rules` package, what it detects, and how to address it. Severity values reflect the rule's default behavior and may vary with heuristics.

## Expected false positive rate
Dogfooding indicates an expected false positive rate of about 77%. Treat findings as signals that need developer judgment and context, not absolute defects.

## nested-range
**Category:** algorithm | **Severity:** Low-Medium-High
**What it detects:** Nested `range` loops over collections, especially when inner loops iterate over the same or related collection without map lookup optimization.
**Why it's bad:** Nested iteration can be O(n^2) and becomes very slow as data grows.
**Example bad code:**
```go
for _, user := range users {
	for _, order := range orders {
		if order.UserID == user.ID {
			process(order)
		}
	}
}
```
**Example fix:**
```go
ordersByUser := make(map[int][]Order, len(orders))
for _, order := range orders {
	ordersByUser[order.UserID] = append(ordersByUser[order.UserID], order)
}
for _, user := range users {
	for _, order := range ordersByUser[user.ID] {
		process(order)
	}
}
```
**False positives:** When the inner loop is bounded to a tiny size, exits early, or uses an optimization pattern not recognized by the heuristic.

## linear-search-in-loop
**Category:** algorithm | **Severity:** Medium
**What it detects:** A linear search loop nested inside another loop, using a comparison and a `break`/`return` pattern.
**Why it's bad:** O(n) lookup inside an outer loop becomes O(n*m), often replaceable with a map lookup.
**Example bad code:**
```go
for _, u := range users {
	for _, id := range activeIDs {
		if u.ID == id {
			markActive(u)
			break
		}
	}
}
```
**Example fix:**
```go
active := make(map[int]struct{}, len(activeIDs))
for _, id := range activeIDs {
	active[id] = struct{}{}
}
for _, u := range users {
	if _, ok := active[u.ID]; ok {
		markActive(u)
	}
}
```
**False positives:** When the inner loop is not actually a search or when the collection sizes are tiny and performance is irrelevant.

## unpreallocated-slice
**Category:** allocation | **Severity:** Low-Medium
**What it detects:** `append` to a slice inside a loop when the slice was not preallocated with capacity.
**Why it's bad:** Repeated reallocations and copies cause unnecessary CPU and memory overhead.
**Example bad code:**
```go
var out []Item
for _, in := range inputs {
	out = append(out, transform(in))
}
```
**Example fix:**
```go
out := make([]Item, 0, len(inputs))
for _, in := range inputs {
	out = append(out, transform(in))
}
```
**False positives:** When the output size is unknown or very small, or if preallocation is handled in another scope that the rule does not see.

## string-concat-loop
**Category:** allocation | **Severity:** Medium
**What it detects:** String concatenation with `+=` inside loops.
**Why it's bad:** Strings are immutable, so each `+=` copies all previous content, causing O(n^2) allocation behavior.
**Example bad code:**
```go
var s string
for _, part := range parts {
	s += part
}
```
**Example fix:**
```go
var b strings.Builder
for _, part := range parts {
	b.WriteString(part)
}
result := b.String()
```
**False positives:** When the loop is very small or the concatenated strings are tiny; also if a builder is used but not detected by the heuristic.

## map-without-size
**Category:** allocation | **Severity:** Low
**What it detects:** `make(map[K]V)` without a size hint when the map is populated inside a loop.
**Why it's bad:** Maps rehash and grow as they fill, which can be avoided when the size is known or estimable.
**Example bad code:**
```go
m := make(map[int]User)
for _, u := range users {
	m[u.ID] = u
}
```
**Example fix:**
```go
m := make(map[int]User, len(users))
for _, u := range users {
	m[u.ID] = u
}
```
**False positives:** When the size is truly unknown or map growth is minimal compared to other costs.

## error-wrap-in-loop
**Category:** allocation | **Severity:** Low
**What it detects:** `errors.Wrap`, `errors.Wrapf`, or `fmt.Errorf` inside loops.
**Why it's bad:** Error wrapping/formatting allocates on every iteration, adding GC pressure in hot paths.
**Example bad code:**
```go
for _, item := range items {
	if err := do(item); err != nil {
		return errors.Wrap(err, "failed")
	}
}
```
**Example fix:**
```go
for _, item := range items {
	if err := do(item); err != nil {
		return err
	}
}
```
**False positives:** When the error path is rare or the loop is not performance-critical.

## fmt-errorf-wrap-loop
**Category:** allocation | **Severity:** Medium
**What it detects:** `fmt.Errorf` with `%w` inside loops (error wrapping chains).
**Why it's bad:** Wrapping with `%w` allocates a chain of errors each iteration.
**Example bad code:**
```go
for _, item := range items {
	if err := do(item); err != nil {
		return fmt.Errorf("item %s: %w", item.ID, err)
	}
}
```
**Example fix:**
```go
if err := processItems(items); err != nil {
	return fmt.Errorf("process items: %w", err)
}
```
**False positives:** When detailed per-iteration context is required and the loop is not hot.

## context-background-in-handler
**Category:** context | **Severity:** Medium
**What it detects:** `context.Background()` or `context.TODO()` inside HTTP handlers.
**Why it's bad:** Handler work should use `r.Context()` so it is cancelled on client disconnect.
**Example bad code:**
```go
func handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	_ = doWork(ctx)
}
```
**Example fix:**
```go
func handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = doWork(ctx)
}
```
**False positives:** When work is intentionally detached from the request lifecycle (background jobs), and that intent is documented.

## missing-context-timeout
**Category:** context | **Severity:** Medium
**What it detects:** Use of `http.NewRequest` (without context) for outbound calls.
**Why it's bad:** Requests without contexts cannot be cancelled or timed out, leading to resource leaks and hangs.
**Example bad code:**
```go
req, _ := http.NewRequest("GET", url, nil)
```
**Example fix:**
```go
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
```
**False positives:** When a context is intentionally not required or timeouts are applied elsewhere (e.g., transport-level timeouts).

## context-leak
**Category:** context | **Severity:** High
**What it detects:** `context.WithCancel`, `WithTimeout`, or `WithDeadline` where the returned `cancel` function is never called.
**Why it's bad:** Unused cancel functions leak timers, goroutines, and other resources tied to the context.
**Example bad code:**
```go
ctx, cancel := context.WithTimeout(ctx, time.Second)
_ = ctx
```
**Example fix:**
```go
ctx, cancel := context.WithTimeout(ctx, time.Second)
defer cancel()
```
**False positives:** When cancel is invoked in another function or stored for later use, and the heuristic cannot see it.

## missing-connection-pool-config
**Category:** database | **Severity:** Medium
**What it detects:** `sql.Open` without any pool configuration calls on the resulting `*sql.DB`.
**Why it's bad:** Default pool settings may be inappropriate, causing connection exhaustion or wasted resources.
**Example bad code:**
```go
db, _ := sql.Open("pgx", dsn)
```
**Example fix:**
```go
db, _ := sql.Open("pgx", dsn)
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```
**False positives:** When configuration happens in another file or function that the rule does not scan.

## unlimited-connection-pool
**Category:** database | **Severity:** High
**What it detects:** `SetMaxOpenConns(0)` which allows unlimited connections.
**Why it's bad:** Unlimited connections can overwhelm the database during traffic spikes.
**Example bad code:**
```go
db.SetMaxOpenConns(0)
```
**Example fix:**
```go
db.SetMaxOpenConns(25)
```
**False positives:** When running against a database with very high limits and deliberate unlimited pools, which is uncommon.

## sql-in-loop
**Category:** database | **Severity:** Medium-High-Critical
**What it detects:** `Query`, `QueryRow`, `Exec`, `Get`, or similar SQL calls inside loops, excluding prepared statement use.
**Why it's bad:** This is the classic N+1 query pattern and causes many round trips. Writes are especially costly.
**Example bad code:**
```go
for _, id := range ids {
	row := db.QueryRow("SELECT name FROM users WHERE id = ?", id)
	_ = row
}
```
**Example fix:**
```go
rows, _ := db.Query("SELECT id, name FROM users WHERE id IN (?)", ids)
_ = rows
```
**False positives:** When the loop is small, the receiver is not actually a database object, or the query cost is negligible compared to other work.

## indirect-sql-in-loop
**Category:** database | **Severity:** Medium-High
**What it detects:** Calling a function inside a loop when that function itself contains SQL calls.
**Why it's bad:** Hides N+1 patterns in helper functions, still creating repeated round trips.
**Example bad code:**
```go
for _, id := range ids {
	user, _ := loadUser(id) // loadUser executes SQL
	_ = user
}
```
**Example fix:**
```go
users, _ := loadUsers(ids) // batch query inside
_ = users
```
**False positives:** When the helper function only conditionally hits the database or the loop is trivially small.

## unbatched-insert
**Category:** database | **Severity:** Medium-High
**What it detects:** ORM-style `Create`, `Insert`, or `Save` calls inside loops.
**Why it's bad:** Single-row inserts are much slower than batch inserts or bulk copy operations.
**Example bad code:**
```go
for _, row := range rows {
	db.Create(&row)
}
```
**Example fix:**
```go
db.CreateInBatches(rows, 100)
```
**False positives:** When each insert must be independent (e.g., requires per-row transactions or side effects).

## missing-max-bytes-reader
**Category:** io | **Severity:** Medium
**What it detects:** Reading an HTTP request body without `http.MaxBytesReader` in handlers.
**Why it's bad:** Allows unbounded request bodies, which can lead to denial-of-service via memory exhaustion.
**Example bad code:**
```go
body, _ := io.ReadAll(r.Body)
```
**Example fix:**
```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
body, _ := io.ReadAll(r.Body)
```
**False positives:** When body size is already enforced by a reverse proxy or server configuration.

## missing-body-close
**Category:** io | **Severity:** High
**What it detects:** HTTP client response bodies that are never closed.
**Why it's bad:** Leaks connections, exhausts the HTTP client's connection pool, and can stall future requests.
**Example bad code:**
```go
resp, _ := http.Get(url)
_ = resp
```
**Example fix:**
```go
resp, _ := http.Get(url)
if resp != nil {
	defer resp.Body.Close()
}
```
**False positives:** When the body is closed in helper functions not visible to the rule.

## response-writer-buffering
**Category:** io | **Severity:** Low
**What it detects:** Writing to `http.ResponseWriter` in a loop without using `http.Flusher`.
**Why it's bad:** Writes may be buffered and not reach the client until the handler returns, breaking streaming responses.
**Example bad code:**
```go
for _, chunk := range chunks {
	_, _ = w.Write(chunk)
}
```
**Example fix:**
```go
if f, ok := w.(http.Flusher); ok {
	for _, chunk := range chunks {
		_, _ = w.Write(chunk)
		f.Flush()
	}
}
```
**False positives:** When writes are small and buffering is acceptable or another flush mechanism is already in place.

## json-in-loop
**Category:** io | **Severity:** Low-Medium
**What it detects:** `json.Marshal`/`json.Unmarshal` in loops or `json.NewEncoder().Encode()` created inside the loop.
**Why it's bad:** JSON uses reflection and allocates; repeated setup wastes CPU and memory.
**Example bad code:**
```go
for _, item := range items {
	data, _ := json.Marshal(item)
	_ = data
}
```
**Example fix:**
```go
enc := json.NewEncoder(w)
for _, item := range items {
	_ = enc.Encode(item)
}
```
**False positives:** When the loop is very small or the encoder/decoder must be tied to per-iteration resources.

## http-client-creation
**Category:** io | **Severity:** Low-Medium-High
**What it detects:** Creating `http.Client{}` inside functions, especially inside loops.
**Why it's bad:** Each client has its own connection pool, so per-request creation loses pooling benefits and adds overhead.
**Example bad code:**
```go
func fetch(url string) {
	client := http.Client{}
	_, _ = client.Get(url)
}
```
**Example fix:**
```go
var httpClient = &http.Client{}

func fetch(url string) {
	_, _ = httpClient.Get(url)
}
```
**False positives:** When distinct clients are required for different transports, or creation is intentionally rare.

## read-all
**Category:** io | **Severity:** Low-Medium
**What it detects:** `io.ReadAll`/`ioutil.ReadAll`, especially when reading HTTP response bodies.
**Why it's bad:** Reads entire content into memory, which can cause high memory usage or OOM on large inputs.
**Example bad code:**
```go
data, _ := io.ReadAll(resp.Body)
_ = data
```
**Example fix:**
```go
_, _ = io.Copy(dst, resp.Body)
```
**False positives:** When content size is small, bounded, or guaranteed by protocol constraints.

## interface-boxing-loop
**Category:** allocation | **Severity:** Low
**What it detects:** Passing concrete values to `interface{}` parameters in loops, causing boxing.
**Why it's bad:** Boxing allocates interface headers and can add GC pressure in hot loops.
**Example bad code:**
```go
for _, v := range values {
	useAny(v)
}
```
**Example fix:**
```go
for _, v := range values {
	useTyped(v)
}
```
**False positives:** When the called function truly needs `interface{}` or the loop is not performance-critical.

## variadic-interface
**Category:** allocation | **Severity:** Low
**What it detects:** Printf-style variadic calls in loops with many complex arguments.
**Why it's bad:** Each argument is boxed to `interface{}`, and complex expressions can allocate twice.
**Example bad code:**
```go
for _, v := range values {
	log.Printf("v=%v, a=%v, b=%v", v, computeA(), computeB())
}
```
**Example fix:**
```go
for _, v := range values {
	if logEnabled {
		log.Printf("v=%v", v)
	}
}
```
**False positives:** When logging is essential and the overhead is acceptable or gated by log levels.

## type-assertion-loop
**Category:** allocation | **Severity:** Low
**What it detects:** Multiple type assertions or type switches inside a loop body.
**Why it's bad:** Repeated assertions add overhead and may indicate a hint for a type-specific path.
**Example bad code:**
```go
for _, v := range values {
	if s, ok := v.(string); ok {
		_ = s
	}
	if n, ok := v.(int); ok {
		_ = n
	}
}
```
**Example fix:**
```go
typeValues := splitByType(values)
for _, s := range typeValues.strings {
	_ = s
}
for _, n := range typeValues.ints {
	_ = n
}
```
**False positives:** When the loop is short or type checks are rare and necessary.

## time-parse-in-loop
**Category:** allocation | **Severity:** Low-Medium
**What it detects:** `time.Parse` calls inside loops.
**Why it's bad:** Parsing time layouts repeatedly is relatively expensive.
**Example bad code:**
```go
for _, s := range timestamps {
	_, _ = time.Parse(time.RFC3339, s)
}
```
**Example fix:**
```go
layout := time.RFC3339
for _, s := range timestamps {
	_, _ = time.Parse(layout, s)
}
```
**False positives:** When inputs or layouts are unique per iteration and caching provides no benefit.

## time-location-in-loop
**Category:** allocation | **Severity:** Medium
**What it detects:** `time.LoadLocation` inside loops.
**Why it's bad:** Loading time zones reads data from disk or tzdata and is slow.
**Example bad code:**
```go
for _, name := range zones {
	_, _ = time.LoadLocation(name)
}
```
**Example fix:**
```go
loc, _ := time.LoadLocation("America/New_York")
_ = loc
```
**False positives:** When loading different zones dynamically is required and caching is not feasible.

## time-format-loop
**Category:** io | **Severity:** Low
**What it detects:** Multiple `time.Time.Format` calls inside a loop body.
**Why it's bad:** Formatting allocates strings; repeated calls per iteration multiply allocations.
**Example bad code:**
```go
for _, t := range times {
	_ = t.Format(time.RFC3339)
	_ = t.Format(time.RFC3339Nano)
}
```
**Example fix:**
```go
for _, t := range times {
	_ = t.Format(time.RFC3339)
}
```
**False positives:** When multiple formats are truly required and the loop is small.

## repeated-regexp-compile
**Category:** cache | **Severity:** Medium
**What it detects:** `regexp.Compile` or `regexp.MustCompile` inside functions.
**Why it's bad:** Compiling regexes is expensive and should be done once and reused.
**Example bad code:**
```go
func isValid(s string) bool {
	re := regexp.MustCompile(`^[a-z]+$`)
	return re.MatchString(s)
}
```
**Example fix:**
```go
var re = regexp.MustCompile(`^[a-z]+$`)

func isValid(s string) bool {
	return re.MatchString(s)
}
```
**False positives:** When the pattern is truly dynamic and cannot be compiled once.

## repeated-template-parse
**Category:** cache | **Severity:** Medium
**What it detects:** `template.Parse`, `ParseFiles`, or `ParseGlob` inside functions.
**Why it's bad:** Template parsing is expensive and should be done at startup.
**Example bad code:**
```go
func render(w io.Writer, name string) {
	_, _ = template.New("x").ParseFiles(name)
}
```
**Example fix:**
```go
var templates = template.Must(template.ParseGlob("*.html"))
```
**False positives:** When templates are intentionally dynamic or user-provided at runtime.

## regexp-match-string-loop
**Category:** cache | **Severity:** High
**What it detects:** `regexp.MatchString` or related package-level helpers inside loops.
**Why it's bad:** These helpers compile the pattern on every call.
**Example bad code:**
```go
for _, s := range inputs {
	if regexp.MatchString("^foo", s) {
		use(s)
	}
}
```
**Example fix:**
```go
re := regexp.MustCompile("^foo")
for _, s := range inputs {
	if re.MatchString(s) {
		use(s)
	}
}
```
**False positives:** When the pattern changes per iteration or the loop size is tiny.

## json-schema-in-loop
**Category:** cache | **Severity:** Medium
**What it detects:** JSON schema compilation or validation calls inside loops.
**Why it's bad:** Schema compilation is expensive and should be done once.
**Example bad code:**
```go
for _, doc := range docs {
	_ = jsonschema.Compile(schemaText)
	_ = doc
}
```
**Example fix:**
```go
schema, _ := jsonschema.Compile(schemaText)
for _, doc := range docs {
	_ = schema.Validate(doc)
}
```
**False positives:** When schema changes per iteration and cannot be cached.

## benchmark-suggestion
**Category:** benchmark | **Severity:** Low
**What it detects:** Functions containing performance-sensitive patterns such as loops, allocations, SQL, or reflection.
**Why it's bad:** Lacking benchmarks makes it harder to track performance regressions in hot code paths.
**Example bad code:**
```go
func Process(items []Item) {
	for _, item := range items {
		_ = json.Marshal(item)
	}
}
```
**Example fix:**
```go
func BenchmarkProcess(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Process(testItems)
	}
}
```
**False positives:** When the function is not performance-critical or already covered by other benchmarks.

## pprof-in-hot-path
**Category:** memory | **Severity:** Medium-High
**What it detects:** `pprof` calls inside loops or HTTP handlers.
**Why it's bad:** Profiling operations are expensive and add significant overhead in hot paths.
**Example bad code:**
```go
for i := 0; i < n; i++ {
	pprof.StartCPUProfile(w)
	pprof.StopCPUProfile()
}
```
**Example fix:**
```go
if enableProfile {
	pprof.StartCPUProfile(w)
	defer pprof.StopCPUProfile()
}
```
**False positives:** When profiling is used in debug-only or test-only paths.

## large-struct-copy
**Category:** memory | **Severity:** Medium
**What it detects:** Passing large structs by value or copying large structs inside loops.
**Why it's bad:** Copying large structs costs CPU and memory bandwidth.
**Example bad code:**
```go
func process(s BigStruct) {
	_ = s
}
```
**Example fix:**
```go
func process(s *BigStruct) {
	_ = s
}
```
**False positives:** When value semantics are required or the struct is not actually large (size estimate heuristic).

## escape-to-heap
**Category:** memory | **Severity:** Low
**What it detects:** Intended to flag pointer creation in loops that likely causes heap escapes, but currently no issues are emitted.
**Why it's bad:** Heap escapes increase GC pressure and reduce cache locality.
**Example bad code:**
```go
for i := 0; i < n; i++ {
	x := i
	_ = &x
}
```
**Example fix:**
```go
for i := 0; i < n; i++ {
	useValue(i)
}
```
**False positives:** The current heuristic is a placeholder; if enabled, false positives are expected without type and escape analysis.

## reflection-in-loop
**Category:** io | **Severity:** Medium
**What it detects:** Reflection calls like `reflect.ValueOf`, `reflect.TypeOf`, or `reflect.MakeSlice` inside loops.
**Why it's bad:** Reflection is slow and magnifies cost when repeated per iteration.
**Example bad code:**
```go
for _, v := range values {
	_ = reflect.ValueOf(v)
}
```
**Example fix:**
```go
for _, v := range values {
	useTyped(v)
}
```
**False positives:** When reflection results cannot be cached or the loop is tiny.

## sync-pool-opportunity
**Category:** allocation | **Severity:** Low
**What it detects:** Repeated buffer allocations like `make([]byte, ...)` or `bytes.NewBuffer` inside loops.
**Why it's bad:** Frequent allocations increase GC pressure. `sync.Pool` can reuse buffers.
**Example bad code:**
```go
for i := 0; i < n; i++ {
	buf := make([]byte, 4096)
	_ = buf
}
```
**Example fix:**
```go
var bufPool = sync.Pool{New: func() any { return make([]byte, 4096) }}
for i := 0; i < n; i++ {
	buf := bufPool.Get().([]byte)
	// use buf
	bufPool.Put(buf)
}
```
**False positives:** When pooling hurts performance (very small allocations or single-use buffers).

## unbuffered-channel
**Category:** concurrency | **Severity:** Low
**What it detects:** Creating unbuffered channels (`make(chan T)` or `make(chan T, 0)`) that are not used in select or common signal patterns.
**Why it's bad:** Unbuffered channels block senders, which can cause deadlocks if not coordinated carefully.
**Example bad code:**
```go
ch := make(chan int)
```
**Example fix:**
```go
ch := make(chan int, 1)
```
**False positives:** When the channel is intentionally unbuffered for synchronization or used in patterns the heuristic does not recognize.

## mutex-in-loop
**Category:** concurrency | **Severity:** Low-Medium
**What it detects:** `Lock` or `RLock` calls inside loops.
**Why it's bad:** Repeated locking adds overhead and can cause contention.
**Example bad code:**
```go
for _, v := range values {
	mu.Lock()
	store[v.ID] = v
	mu.Unlock()
}
```
**Example fix:**
```go
mu.Lock()
for _, v := range values {
	store[v.ID] = v
}
mu.Unlock()
```
**False positives:** When each iteration must be isolated for correctness or the loop is very small.

## goroutine-leak
**Category:** concurrency | **Severity:** High
**What it detects:** Goroutines running infinite loops without a cancellation mechanism (context or done channel).
**Why it's bad:** Goroutines that never terminate leak memory and CPU.
**Example bad code:**
```go
go func() {
	for {
		work()
	}
}()
```
**Example fix:**
```go
go func(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			work()
		}
	}
}(ctx)
```
**False positives:** When termination is handled by another mechanism not visible to the heuristic.
