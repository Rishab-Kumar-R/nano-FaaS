package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Rishab-Kumar-R/nano-faas/worker/internal/runtime"
	"github.com/redis/go-redis/v9"
)

var rdb = redis.NewClient(&redis.Options{
	Addr: "localhost:6379",
})

const JobQueue = "job_queue"

type JobPayload struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Code     string `json:"code"`
}

func main() {
	ctx := context.Background()
	engine, err := runtime.NewManager()
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to Podman: %v", err))
	}

	fmt.Println("Connected to Podman Socket")
	fmt.Println("Worker started. Waiting for jobs...")

	for {
		result, err := rdb.BLPop(ctx, 0*time.Second, JobQueue).Result()
		if err != nil {
			continue
		}

		var job JobPayload
		json.Unmarshal([]byte(result[1]), &job)

		fmt.Printf("Running Job: %s\n", job.ID)

		output, err := engine.RunCode(ctx, job.ID, job.Language, job.Code)
		if err != nil {
			fmt.Printf("Execution Failed: %v\n", err)
		} else {
			fmt.Printf("Result: %s\n", output)
		}
	}
}
