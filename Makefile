BIN := bin/margin

.PHONY: check build test test-race vet fmt run doctor clean

# The canonical gate. Keep the tree green.
check: build test vet

build:
	go build ./...

$(BIN):
	go build -o $(BIN) ./cmd/margin

test:
	go test ./...

# The pty-backed tests spawn real nvim children; -race is where the interesting
# failures live, since the emulator is written from one goroutine and read from
# another.
test-race:
	go test -race ./...

vet:
	go vet ./...

# Prove the machine can actually run the composer tests. Without nvim they
# t.Skip() and the suite still reports ok, so a green `make check` on an
# unprovisioned box proves much less than it appears to.
doctor:
	./scripts/setup-env.sh --verify

fmt:
	gofmt -l -w .

run: $(BIN)
	$(BIN)

clean:
	rm -rf bin
