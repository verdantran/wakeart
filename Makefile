BINARY := wakeart
PREFIX ?= /usr/local

.PHONY: build install test vet fmt bench bench-baseline bench-check golden clean run

build:
	go build -ldflags="-s -w" -o $(BINARY) ./cmd/wakeart

install:
	go install -ldflags="-s -w" ./cmd/wakeart

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# Golden files track layout, not palette tuning. Re-record after a
# deliberate change to fitting or scene art.
golden:
	go test ./internal/tui -update

bench:
	go test ./internal/tui -bench . -benchmem -run XXX

bench-baseline:
	go test ./internal/tui -bench . -run XXX -count 5 > testdata/bench.baseline
	@echo "baseline recorded"

# Fails when a benchmark regresses more than 20% against the recorded baseline.
bench-check:
	@go test ./internal/tui -bench . -run XXX -count 5 > /tmp/wakeart.bench
	@scripts/benchcheck.sh testdata/bench.baseline /tmp/wakeart.bench 20

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
