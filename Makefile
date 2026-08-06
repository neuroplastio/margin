BIN := bin/margin

.PHONY: check build test vet fmt run clean

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

fmt:
	gofmt -l -w .

run: $(BIN)
	$(BIN)

clean:
	rm -rf bin
