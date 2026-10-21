BIN := bin/margin

# A build has a name and an identity (D18). COMMIT is the identity: the full
# commit. MODIFIED marks a tree with uncommitted changes, whose commit names
# code the binary does not contain. A local build is named after its short
# commit (+dirty) and is on no channel; `make dist`, which the release workflow
# runs, names it YY.MM.DD-dev.<sha7> — the commit's date in UTC — and stamps
# the dev channel it is published to. `make dist RELEASE=YY.MM.DD`, for a
# release tag at HEAD, names it after the tag and stamps the stable channel
# (D21).
SHORT    := $(shell git rev-parse --short=7 HEAD 2>/dev/null)
COMMIT   ?= $(shell git rev-parse HEAD 2>/dev/null)
MODIFIED ?= $(if $(shell git status --porcelain 2>/dev/null),true,)
RELEASE  ?=
CHANNEL  ?= $(if $(RELEASE),stable,dev)
DIST_VERSION = $(or $(RELEASE),$(shell TZ=UTC git show -s --date=format-local:%y.%m.%d --format=%cd HEAD)-$(CHANNEL).$(SHORT))

VERSION_PKG := github.com/neuroplastio/margin/internal/version
LDFLAGS := -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).modified=$(MODIFIED)
DIST_LDFLAGS := -s -w -X $(VERSION_PKG).Commit=$(COMMIT) \
	-X $(VERSION_PKG).Version=$(DIST_VERSION) -X $(VERSION_PKG).Channel=$(CHANNEL)
DIST_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: check build dist release test test-race vet fmt run clean mermaid-wasm

# The canonical gate. Keep the tree green.
check: build test vet

build:
	go build ./...
	go build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/margin
	go build -ldflags '$(LDFLAGS)' -o $(BIN)-launcher ./cmd/margin-launcher

# The release binaries: static, one per platform, dist/margin_<os>_<arch>, as
# the release workflow publishes them to the channel, and beside each its thin
# launcher, dist/margin-launcher_<os>_<arch> (D20). Refuses a modified tree,
# whose build would claim a commit it does not contain, and a RELEASE that is
# not a tag at HEAD.
dist:
	@test -z "$(MODIFIED)" || { echo "make dist: the tree has uncommitted changes" >&2; exit 1; }
	@test -z "$(RELEASE)" || test "$$(git rev-parse -q --verify 'refs/tags/$(RELEASE)^{commit}')" = "$(COMMIT)" || \
		{ echo "make dist: RELEASE=$(RELEASE) is not a tag at HEAD" >&2; exit 1; }
	rm -rf dist
	@set -e; for p in $(DIST_PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		for b in margin margin-launcher; do \
			echo "dist/$${b}_$${os}_$${arch}"; \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(DIST_LDFLAGS)' \
				-o dist/$${b}_$${os}_$${arch} ./cmd/$$b; \
		done; \
	done

# Cut a release (D21): tag HEAD with today's date in UTC, YY.MM.DD, and push
# the tag. The tag is the whole act: the release workflow builds it, makes
# the GitHub release and publishes it to the stable channel. HEAD must be
# clean, on origin/main, and not a release already; a day has at most one
# release, so a tag that already exists is refused (README, Releasing:
# replacing one).
release:
	@test -z "$(MODIFIED)" || { echo "make release: the tree has uncommitted changes" >&2; exit 1; }
	@git fetch -q --tags origin
	@git merge-base --is-ancestor HEAD origin/main || { echo "make release: HEAD is not on origin/main" >&2; exit 1; }
	@other=$$(git tag --points-at HEAD --list '[0-9][0-9].[0-9][0-9].[0-9][0-9]'); \
	test -z "$$other" || { echo "make release: $(SHORT) is already released as $$other" >&2; exit 1; }
	@set -e; tag=$$(date -u +%y.%m.%d); \
	if git ls-remote --exit-code --tags origin "refs/tags/$$tag" >/dev/null; then \
		echo "make release: $$tag is already released; a day has one release" >&2; exit 1; \
	fi; \
	git tag -a "$$tag" -m "margin $$tag"; \
	git push origin "refs/tags/$$tag"; \
	echo "margin $$tag is $(SHORT); the release workflow takes it from here"

test:
	go test ./...

# The HOTTY tests run a real Bubble Tea program against a test host, whose
# output and input are on goroutines of their own: -race is where their
# failures live.
test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# make run FILE=path/to/doc.md
FILE ?= README.md
run: build
	$(BIN) $(FILE)

clean:
	rm -rf bin dist

# The mermaid renderer internal/mermaidsvg embeds: Ondine (Rust) at
# ONDINE_REV, built for wasm32-unknown-unknown and committed gzipped, so `go
# build` and `go install` need no Rust. Rebuild after moving ONDINE_REV; see
# $(MERMAID_DIR)/README.md. Ondine's own rust-toolchain.toml pins the
# toolchain and the target; its Cargo.lock is used as it is.
MERMAID_DIR := internal/mermaidsvg/wasm
ONDINE_DIR := $(MERMAID_DIR)/ondine
ONDINE_REV := 6a1586e5af980e67c104239593ae931837157b8b
CARGO ?= mise x rust -- cargo
mermaid-wasm:
	@test -d $(ONDINE_DIR)/.git || git init -q $(ONDINE_DIR)
	git -C $(ONDINE_DIR) fetch -q --depth 1 https://github.com/wevm/ondine $(ONDINE_REV)
	git -C $(ONDINE_DIR) checkout -q --detach $(ONDINE_REV)
	cd $(ONDINE_DIR) && $(CARGO) build --locked -p wasm --profile wasm --features all --target wasm32-unknown-unknown
	gzip -9 -n -c $(ONDINE_DIR)/target/wasm32-unknown-unknown/wasm/wasm.wasm \
		> internal/mermaidsvg/mermaid.wasm.gz
	@ls -l internal/mermaidsvg/mermaid.wasm.gz
