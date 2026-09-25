# Rung 1: Toxiproxy topology replica (PC-24)

A local, zero-cloud-credential replica of one dependency edge — the app tier's
dependency on its database — used to observe real degradation behaviour under a
Toxiproxy-injected fault. See `validate/rung1.go`'s own package doc comment for the
full scope decision (why this one edge, why "cut" specifically) and for exactly how
the observed fault signal was hand-verified against the real running stack before any
Go code was trusted to interpret it.

## Running it yourself

```bash
cd validate/rung1
docker-compose up -d --wait

# create the proxy (validate/rung1_test.go does this automatically; shown here for
# manual/interactive use)
curl -sf -X POST http://localhost:8474/proxies \
  -H "Content-Type: application/json" \
  -d '{"name":"database","listen":"0.0.0.0:15432","upstream":"database:5432","enabled":true}'

# inject the cut fault
curl -sf -X POST http://localhost:8474/proxies/database \
  -H "Content-Type: application/json" -d '{"enabled":false}'

# clear it again
curl -sf -X POST http://localhost:8474/proxies/database \
  -H "Content-Type: application/json" -d '{"enabled":true}'

docker-compose down -v
```

## Running the real test

```bash
cd validate
go test ./... -run TestRung1 -v
```

Skips cleanly (not a failure) if `docker-compose` isn't on `PATH` or the Docker daemon
isn't reachable — this rung needs zero cloud credentials, but it does need a real
local Docker.
