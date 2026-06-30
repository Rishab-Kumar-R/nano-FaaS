package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Rishab-Kumar-R/nano-faas/shared"
	"github.com/Rishab-Kumar-R/nano-faas/worker/internal/runtime"
	"github.com/Rishab-Kumar-R/nano-faas/worker/internal/scheduler"
	"github.com/redis/go-redis/v9"
)

const (
	jobQueue    = "job_queue"
	dlq         = "dead_letter_queue"
	maxRetries  = 3
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	workerCount := 5
	if v := os.Getenv("WORKER_COUNT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			workerCount = n
		}
	}

	execTimeout := 30 * time.Second
	if v := os.Getenv("EXEC_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			execTimeout = time.Duration(n) * time.Second
		}
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	ctx := context.Background()

	engine, err := runtime.NewManager()
	if err != nil {
		panic(fmt.Sprintf("failed to connect to Podman: %v", err))
	}

	sched := scheduler.New(rdb)
	sched.Start(ctx)
	defer sched.Stop()

	fmt.Printf("Connected to Podman. Starting %d workers (timeout: %s)...\n", workerCount, execTimeout)

	var wg sync.WaitGroup
	for i := range workerCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runWorker(ctx, rdb, engine, execTimeout, id)
		}(i)
	}
	wg.Wait()
}

func runWorker(ctx context.Context, rdb *redis.Client, engine *runtime.Manager, timeout time.Duration, id int) {
	for {
		result, err := rdb.BLPop(ctx, 0, jobQueue).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			fmt.Printf("[worker %d] queue error: %v\n", id, err)
			continue
		}

		var job shared.JobPayload
		if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
			fmt.Printf("[worker %d] failed to parse job: %v\n", id, err)
			continue
		}

		fmt.Printf("[worker %d] running job %s (function %s, retry %d)\n", id, job.ExecutionID, job.FunctionID, job.RetryCount)
		processJob(ctx, rdb, engine, timeout, job)
	}
}

func processJob(ctx context.Context, rdb *redis.Client, engine *runtime.Manager, timeout time.Duration, job shared.JobPayload) {
	setStatus(ctx, rdb, job.ExecutionID, "RUNNING", "")

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var outputLines []string
	publish := func(msg, level string) {
		if level == "stdout" {
			outputLines = append(outputLines, msg)
		}
		entry := shared.LogEntry{
			Message:   msg,
			Level:     level,
			Timestamp: time.Now().Format(time.RFC3339),
		}
		data, _ := json.Marshal(entry)
		rdb.Publish(ctx, "logs:"+job.ExecutionID, data)
	}

	err := engine.RunCode(execCtx, job.Language, job.Code, job.Input, publish)

	done, _ := json.Marshal(shared.LogEntry{Done: true})
	rdb.Publish(ctx, "logs:"+job.ExecutionID, done)

	output := strings.Join(outputLines, "\n")
	result := shared.ResultPayload{Output: output}

	if err != nil {
		fmt.Printf("job %s failed (retry %d/%d): %v\n", job.ExecutionID, job.RetryCount, maxRetries, err)
		result.Error = err.Error()

		if job.RetryCount < maxRetries {
			job.RetryCount++
			data, _ := json.Marshal(job)
			rdb.RPush(ctx, jobQueue, data)
			setStatus(ctx, rdb, job.ExecutionID, "QUEUED", "")
		} else {
			entry := shared.DeadLetterEntry{
				ExecutionID: job.ExecutionID,
				FunctionID:  job.FunctionID,
				Language:    job.Language,
				Error:       err.Error(),
				Retries:     job.RetryCount,
			}
			data, _ := json.Marshal(entry)
			rdb.LPush(ctx, dlq, data)
			setStatus(ctx, rdb, job.ExecutionID, "FAILED", err.Error())
		}
	} else {
		setStatus(ctx, rdb, job.ExecutionID, "COMPLETED", output)
	}

	resultData, _ := json.Marshal(result)
	rdb.Publish(ctx, "result:"+job.ExecutionID, resultData)
}

func setStatus(ctx context.Context, rdb *redis.Client, execID, status, result string) {
	args := []any{"status", status}
	if result != "" {
		args = append(args, "result", result)
	}
	rdb.HSet(ctx, "execution:"+execID, args...)
}
