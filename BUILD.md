# Build

## Prerequisites

- Go
- PortAudio development headers and library
- ffmpeg available on `PATH`

## Test

```bash
GOCACHE=/tmp/go-build go test ./...
```

For verbose test output:

```bash
GOCACHE=/tmp/go-build go test -v -count=1 ./...
```

## Vet

```bash
GOCACHE=/tmp/go-build go vet ./...
```

## Build Binary

```bash
GOCACHE=/tmp/go-build go build -buildvcs=false -o /tmp/corder ./cmd/corder
```

Run the built binary:

```bash
/tmp/corder
```
