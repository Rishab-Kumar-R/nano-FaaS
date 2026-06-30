package graph

import "github.com/redis/go-redis/v9"

type Resolver struct {
	Redis *redis.Client
}
