BIN := bin/margin

# A build has a name and an identity (D18). COMMIT is the identity: the full
# commit. MODIFIED marks a tree with uncommitted changes, whose commit names
# code the binary does not contain. A local build is named after its short
# commit (+dirty) and is on no channel; `make dist`, which the release workflow
# runs, names it YY.MM.DD-dev.<sha7> — the commit's date in UTC — and stamps
# the dev channel it is published to.
SHORT    := $(shell git rev-parse --short=7 HEAD 2>/dev/null)
COMMIT   ?= $(shell git rev-parse HEAD 2>/dev/null)
MODIFIED ?= $(if $(shell git status --porcelain 2>/dev/null),true,)
CHANNEL  ?= dev
DIST_VERSION = $(shell TZ=UTC git show -s --date=format-local:%y.%m.%d --format=%cd HEAD)-$(CHANNEL).$(SHORT)

VERSION_PKG := github.com/neuroplastio/margin/internal/version
LDFLAGS := -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).modified=$(MODIFIED)
DIST_LDFLAGS := -s -w -X $(VERSION_PKG).Commit=$(COMMIT) \
	-X $(VERSION_PKG).Version=$(DIST_VERSION) -X $(VERSION_PKG).Channel=$(CHANNEL)
DIST_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: check build dist test test-race vet fmt run doctor clean mermaid-wasm mermaid-metrics

# The canonical gate. Keep the tree green.
check: build test vet

build:
	go build ./...
	go build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/margin

# The release binaries: static, one per platform, dist/margin_<os>_<arch>, as
# the release workflow publishes them to the channel. Refuses a modified tree,
# whose build would claim a commit it does not contain.
dist:
	@test -z "$(MODIFIED)" || { echo "make dist: the tree has uncommitted changes" >&2; exit 1; }
	rm -rf dist
	@set -e; for p in $(DIST_PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "dist/margin_$${os}_$${arch}"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(DIST_LDFLAGS)' \
			-o dist/margin_$${os}_$${arch} ./cmd/margin; \
	done

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

# make run FILE=path/to/doc.md
FILE ?= README.md
run: build
	$(BIN) $(FILE)

clean:
	rm -rf bin dist

# The mermaid renderer internal/mermaidsvg embeds: merman (Rust) built as a
# WASI command and committed gzipped, so `go build` and `go install` need no
# Rust. Rebuild after changing anything under $(MERMAID_DIR); see its
# README.md. The toolchain and the wasm32-wasip1 target are pinned in
# $(MERMAID_DIR)/rust-toolchain.toml.
MERMAID_DIR := internal/mermaidsvg/wasm
CARGO ?= mise x rust -- cargo
mermaid-wasm:
	cd $(MERMAID_DIR) && $(CARGO) build --release --locked --target wasm32-wasip1
	gzip -9 -n -c $(MERMAID_DIR)/target/wasm32-wasip1/release/margin-mermaid.wasm \
		> internal/mermaidsvg/mermaid.wasm.gz
	@ls -l internal/mermaidsvg/mermaid.wasm.gz

# Regenerates the Inter advance widths the renderer measures labels with.
INTER_TTC ?= /usr/share/fonts/inter/Inter.ttc
mermaid-metrics:
	cd $(MERMAID_DIR) && $(CARGO) run --locked --example gen_metrics -- $(INTER_TTC) > src/inter_metrics.rs.new
	mv $(MERMAID_DIR)/src/inter_metrics.rs.new $(MERMAID_DIR)/src/inter_metrics.rs
