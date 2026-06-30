package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/Rishab-Kumar-R/nano-faas/gateway/graph"
	"github.com/Rishab-Kumar-R/nano-faas/gateway/graph/model"
	"github.com/Rishab-Kumar-R/nano-faas/shared"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/vektah/gqlparser/v2/ast"
)

const (
	defaultPort   = "8080"
	jobQueue      = "job_queue"
	triggerTimeout = 35 * time.Second
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	srv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{Redis: rdb},
	}))

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(&transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv)
	http.HandleFunc("/fn/", httpTrigger(rdb))

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// httpTrigger handles POST /fn/{functionId} — runs the function synchronously
// and returns its stdout as the response body.
func httpTrigger(rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		functionID := strings.TrimPrefix(r.URL.Path, "/fn/")
		if functionID == "" {
			http.Error(w, "missing function ID", http.StatusBadRequest)
			return
		}

		body, _ := io.ReadAll(r.Body)
		input := string(body)

		ctx := r.Context()

		val, err := rdb.HGet(ctx, "function:"+functionID, "meta").Result()
		if err != nil {
			http.Error(w, "function not found", http.StatusNotFound)
			return
		}

		var fn model.Function
		if err := json.Unmarshal([]byte(val), &fn); err != nil {
			http.Error(w, "failed to parse function", http.StatusInternalServerError)
			return
		}

		execID := uuid.NewString()

		// Subscribe BEFORE pushing the job to avoid a result-missed race.
		triggerCtx, cancel := context.WithTimeout(ctx, triggerTimeout)
		defer cancel()

		pubsub := rdb.Subscribe(triggerCtx, "result:"+execID)
		defer func() { _ = pubsub.Close() }()

		pipe := rdb.Pipeline()
		pipe.HSet(ctx, "execution:"+execID,
			"id", execID,
			"functionId", functionID,
			"status", string(model.StatusQueued),
		)
		pipe.Expire(ctx, "execution:"+execID, 24*time.Hour)
		pipe.LPush(ctx, "executions:"+functionID, execID)
		pipe.Expire(ctx, "executions:"+functionID, 24*time.Hour)
		if _, err := pipe.Exec(ctx); err != nil {
			http.Error(w, "failed to create execution", http.StatusInternalServerError)
			return
		}

		job := shared.JobPayload{
			ExecutionID: execID,
			FunctionID:  functionID,
			Language:    fn.Language.String(),
			Code:        fn.Code,
			Input:       input,
		}
		jobData, _ := json.Marshal(job)
		if err := rdb.RPush(ctx, jobQueue, jobData).Err(); err != nil {
			http.Error(w, "failed to queue job", http.StatusInternalServerError)
			return
		}

		select {
		case msg, ok := <-pubsub.Channel():
			if !ok {
				http.Error(w, "result channel closed", http.StatusInternalServerError)
				return
			}
			var result shared.ResultPayload
			if err := json.Unmarshal([]byte(msg.Payload), &result); err != nil {
				http.Error(w, "failed to parse result", http.StatusInternalServerError)
				return
			}
			if result.Error != "" {
				http.Error(w, result.Error, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, result.Output)

		case <-triggerCtx.Done():
			http.Error(w, "execution timed out", http.StatusGatewayTimeout)
		}
	}
}
