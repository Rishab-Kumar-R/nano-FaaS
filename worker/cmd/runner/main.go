package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	fmt.Println("Worker started. Waiting for jobs...")

	for {
		result, err := rdb.BLPop(ctx, 0*time.Second, JobQueue).Result()
		if err != nil {
			fmt.Println("Error fetching job:", err)
			continue
		}

		processJob(result[1])
	}
}

func processJob(jsonPayload string) {
	var job JobPayload
	json.Unmarshal([]byte(jsonPayload), &job)

	fmt.Printf("\n>> Processing Job: %s\n", job.ID)
	fmt.Printf(">> [Lang]: %s\n", job.Language)
	fmt.Printf(">> [Code]: %s\n", job.Code)

	time.Sleep(2 * time.Second)

	fmt.Println("Job Completed")
}
