package graph_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Rishab-Kumar-R/nano-faas/gateway/graph"
	"github.com/Rishab-Kumar-R/nano-faas/gateway/graph/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type testEnv struct {
	r    *graph.Resolver
	rdb  *redis.Client
	mini *miniredis.Miniredis
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return testEnv{r: &graph.Resolver{Redis: rdb}, rdb: rdb, mini: mr}
}

func (e testEnv) mut() graph.MutationResolver { return e.r.Mutation() }
func (e testEnv) qry() graph.QueryResolver    { return e.r.Query() }

func TestCreateFunction(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	fn, err := e.mut().CreateFunction(ctx, "hello", model.LanguagePython, `print("hi")`)
	if err != nil {
		t.Fatalf("CreateFunction: %v", err)
	}
	if fn.ID == "" {
		t.Error("expected non-empty ID")
	}
	if fn.Name != "hello" {
		t.Errorf("got name %q, want %q", fn.Name, "hello")
	}
	if fn.Language != model.LanguagePython {
		t.Errorf("got language %v, want PYTHON", fn.Language)
	}
}

func TestCreateFunction_StoredInRedis(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	fn, _ := e.mut().CreateFunction(ctx, "stored", model.LanguageNodejs, `console.log("ok")`)

	val := e.mini.HGet("function:"+fn.ID, "meta")
	var stored model.Function
	if err := json.Unmarshal([]byte(val), &stored); err != nil {
		t.Fatalf("unmarshal stored function: %v", err)
	}
	if stored.Name != "stored" {
		t.Errorf("stored name %q, want %q", stored.Name, "stored")
	}
}

func TestRunFunction_NotFound(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	_, err := e.mut().RunFunction(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing function, got nil")
	}
}

func TestRunFunction_CreatesExecution(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	fn, _ := e.mut().CreateFunction(ctx, "runner", model.LanguagePython, `print("x")`)
	exec, err := e.mut().RunFunction(ctx, fn.ID)
	if err != nil {
		t.Fatalf("RunFunction: %v", err)
	}
	if exec.Status != model.StatusQueued {
		t.Errorf("got status %v, want QUEUED", exec.Status)
	}
	if exec.FunctionID != fn.ID {
		t.Errorf("got functionId %q, want %q", exec.FunctionID, fn.ID)
	}

	// execution stored in Redis
	if e.mini.HGet("execution:"+exec.ID, "status") != "QUEUED" {
		t.Error("expected execution status QUEUED in Redis")
	}

	// execution indexed under its function
	ids, _ := e.rdb.LRange(ctx, "executions:"+fn.ID, 0, -1).Result()
	if len(ids) == 0 || ids[0] != exec.ID {
		t.Error("expected execution ID in executions list")
	}

	// job pushed to the queue
	jobs, _ := e.rdb.LRange(ctx, "job_queue", 0, -1).Result()
	if len(jobs) == 0 {
		t.Error("expected job in job_queue")
	}
}

func TestQueryExecution(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	fn, _ := e.mut().CreateFunction(ctx, "q", model.LanguagePython, `print("q")`)
	created, _ := e.mut().RunFunction(ctx, fn.ID)

	exec, err := e.qry().Execution(ctx, created.ID)
	if err != nil {
		t.Fatalf("Execution query: %v", err)
	}
	if exec.ID != created.ID {
		t.Errorf("got id %q, want %q", exec.ID, created.ID)
	}
	if exec.Status != model.StatusQueued {
		t.Errorf("got status %v, want QUEUED", exec.Status)
	}
}

func TestQueryExecution_NotFound(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	_, err := e.qry().Execution(ctx, "missing")
	if err == nil {
		t.Fatal("expected error for missing execution")
	}
}

func TestQueryFunctions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	e.mut().CreateFunction(ctx, "a", model.LanguagePython, `1`)
	e.mut().CreateFunction(ctx, "b", model.LanguageNodejs, `2`)

	fns, err := e.qry().Functions(ctx)
	if err != nil {
		t.Fatalf("Functions query: %v", err)
	}
	if len(fns) != 2 {
		t.Errorf("got %d functions, want 2", len(fns))
	}
}

func TestExecutionsByFunction(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	fn, _ := e.mut().CreateFunction(ctx, "multi", model.LanguagePython, `print("x")`)
	e.mut().RunFunction(ctx, fn.ID)
	e.mut().RunFunction(ctx, fn.ID)

	execs, err := e.qry().ExecutionsByFunction(ctx, fn.ID)
	if err != nil {
		t.Fatalf("ExecutionsByFunction: %v", err)
	}
	if len(execs) != 2 {
		t.Errorf("got %d executions, want 2", len(execs))
	}
	for _, ex := range execs {
		if ex.FunctionID != fn.ID {
			t.Errorf("execution has functionId %q, want %q", ex.FunctionID, fn.ID)
		}
	}
}

func TestExecutionsByFunction_Empty(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	execs, err := e.qry().ExecutionsByFunction(ctx, "nofunc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(execs) != 0 {
		t.Errorf("expected empty slice, got %d", len(execs))
	}
}
