package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Rishab-Kumar-R/nano-faas/shared"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

const jobQueue = "job_queue"

type Scheduler struct {
	rdb     *redis.Client
	c       *cron.Cron
	entries map[string]cron.EntryID
	mu      sync.Mutex
}

func New(rdb *redis.Client) *Scheduler {
	return &Scheduler{
		rdb:     rdb,
		c:       cron.New(),
		entries: make(map[string]cron.EntryID),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.loadSchedules(ctx)
	s.c.Start()
	go s.watchChanges(ctx)
}

func (s *Scheduler) Stop() {
	s.c.Stop()
}

func (s *Scheduler) loadSchedules(ctx context.Context) {
	ids, err := s.rdb.SMembers(ctx, "schedules").Result()
	if err != nil {
		fmt.Printf("scheduler: failed to load schedules: %v\n", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entryID := range s.entries {
		s.c.Remove(entryID)
	}
	s.entries = make(map[string]cron.EntryID)

	for _, id := range ids {
		vals, err := s.rdb.HGetAll(ctx, "schedule:"+id).Result()
		if err != nil || len(vals) == 0 {
			continue
		}
		s.register(ctx, vals["id"], vals["functionId"], vals["cron"])
	}
	fmt.Printf("scheduler: loaded %d schedule(s)\n", len(ids))
}

func (s *Scheduler) register(ctx context.Context, scheduleID, functionID, cronExpr string) {
	entryID, err := s.c.AddFunc(cronExpr, func() {
		s.enqueue(context.Background(), functionID)
	})
	if err != nil {
		fmt.Printf("scheduler: invalid cron %q for schedule %s: %v\n", cronExpr, scheduleID, err)
		return
	}
	s.entries[scheduleID] = entryID
}

func (s *Scheduler) enqueue(ctx context.Context, functionID string) {
	val, err := s.rdb.HGet(ctx, "function:"+functionID, "meta").Result()
	if err != nil {
		fmt.Printf("scheduler: function %s not found, skipping\n", functionID)
		return
	}

	var meta struct {
		Language string `json:"language"`
		Code     string `json:"code"`
	}
	if err := json.Unmarshal([]byte(val), &meta); err != nil {
		return
	}

	execID := uuid.NewString()
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, "execution:"+execID,
		"id", execID,
		"functionId", functionID,
		"status", "QUEUED",
	)
	pipe.Expire(ctx, "execution:"+execID, 24*time.Hour)
	pipe.LPush(ctx, "executions:"+functionID, execID)
	pipe.Expire(ctx, "executions:"+functionID, 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		fmt.Printf("scheduler: failed to create execution: %v\n", err)
		return
	}

	job := shared.JobPayload{
		ExecutionID: execID,
		FunctionID:  functionID,
		Language:    meta.Language,
		Code:        meta.Code,
	}
	data, _ := json.Marshal(job)
	s.rdb.RPush(ctx, jobQueue, data)
	fmt.Printf("scheduler: queued job %s for function %s\n", execID, functionID)
}

func (s *Scheduler) watchChanges(ctx context.Context) {
	pubsub := s.rdb.Subscribe(ctx, "schedules:changed")
	defer func() { _ = pubsub.Close() }()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			s.loadSchedules(ctx)
		}
	}
}
