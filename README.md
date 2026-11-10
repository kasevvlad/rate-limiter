# rate-limiter

[![tests](https://github.com/kasevvlad/rate-limiter/actions/workflows/tests.yml/badge.svg)](https://github.com/kasevvlad/rate-limiter/actions/workflows/tests.yml)

Rate limiting for Go HTTP services. A learning project where I implemented two classic algorithms myself instead of taking a library: to understand how they work and where each one breaks.

- **Token bucket** in memory: fast, smooth, for a single instance
- **Sliding window log** in Redis: exact, shared between many instances
- `net/http` middleware with `X-RateLimit-*` and `Retry-After` headers
- A small HTTP service on top of it, so other apps can ask "can this key make a request?"

How the algorithms work and why there are two of them is described (in Russian) in [docs/how-it-works.md](docs/how-it-works.md), known limitations in [docs/limitations.md](docs/limitations.md).

## Using as a library

```go
import ratelimit "github.com/kasevvlad/rate-limiter"

limiter := ratelimit.NewTokenBucket(100, time.Minute)

limited := ratelimit.Middleware(limiter, ratelimit.ByIP)
http.Handle("/api/", limited(apiHandler))
```

For several instances behind a load balancer use the Redis one:

```go
limiter := ratelimit.NewSlidingWindow(redisClient, 100, time.Minute)
```

Any function `func(*http.Request) string` can be the key: IP, user id, API key.

When the limit is exceeded the client gets:

```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
Retry-After: 12
```

## Running the service

```bash
docker compose up -d --build
```

```bash
curl -X POST localhost:8081/check -d '{"key": "user:1"}'
# {"allowed":true,"limit":5,"remaining":4,"retry_after_ms":0}

curl -i localhost:8081/hello   # a demo endpoint limited by IP
```

| Variable | Default | |
|---|---|---|
| `ADDR` | `:8080` | |
| `STORE` | `memory` | `memory` (token bucket) or `redis` (sliding window) |
| `LIMIT` | `10` | Requests per window |
| `WINDOW` | `1m` | Go duration: `10s`, `1m`, `1h` |
| `REDIS_ADDR` | `localhost:6379` | |

## Tests

```bash
go test -race ./...
```

Redis tests run against [miniredis](https://github.com/alicebob/miniredis), so Docker is not needed. Time is faked in tests, nothing sleeps.
