module github.com/Rishab-Kumar-R/nano-faas/gateway

go 1.25.5

require (
	github.com/99designs/gqlgen v0.17.85
	github.com/Rishab-Kumar-R/nano-faas/shared v0.0.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.0
	github.com/redis/go-redis/v9 v9.17.2
	github.com/vektah/gqlparser/v2 v2.5.31
)

require (
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/alicebob/miniredis/v2 v2.38.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
)

replace github.com/Rishab-Kumar-R/nano-faas/shared => ../shared
