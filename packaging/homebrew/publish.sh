#!/usr/bin/env bash
#
# Publish margin's Homebrew formula to the org tap `neuroplastio/homebrew-tap`
# (installed with `brew install neuroplastio/tap/margin`).
#
#   packaging/homebrew/publish.sh <release> [--push | --render <file>]
#
# <release> is a margin release tag (26.10.07). The script downloads that
# release's eight artifacts — the launcher and the seed for linux/darwin ×
# amd64/arm64 — renders `Formula/margin.rb` from `packaging/homebrew/margin.rb`
# with their sha256s, and (with `--push`) commits and pushes it to the tap.
#
# `--render <file>` writes the rendered formula to <file> and stops, for
# check.sh to audit and install it before anything is pushed.
#
# Needs curl, sha256sum (or shasum) and git. On --push it uses
# HOMEBREW_TAP_DEPLOY_KEY when set (the CI path): the private half of the tap's
# deploy key for margin, which may write to the tap and nothing else. Without
# it, your own git credentials.
set -euo pipefail

if [ $# -lt 1 ] || [ $# -gt 3 ] || { [ $# -ge 2 ] && [ "$2" != --push ] && [ "$2" != --render ]; } ||
	{ [ "${2:-}" = --render ] && [ $# -ne 3 ]; } || { [ "${2:-}" = --push ] && [ $# -ne 2 ]; }; then
	echo "usage: $0 <release> [--push | --render <file>]" >&2
	exit 2
fi
release=$1 push=${2:-} out=${3:-}
[ "$push" = --render ] && push=''

here=$(cd "$(dirname "$0")" && pwd)
tap=neuroplastio/homebrew-tap
urlbase="https://github.com/neuroplastio/margin/releases/download/$release"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# The commit the release names — the seed's build directory in the home. Prefer
# the tag in this checkout; fall back to the GitHub API.
commit="$(git -C "$here" rev-parse --verify -q "${release}^{commit}" 2>/dev/null || true)"
if [ -z "$commit" ]; then
	commit="$(gh api "repos/neuroplastio/margin/commits/$release" -q .sha 2>/dev/null || true)"
fi
[ -n "$commit" ] || { echo "publish: cannot resolve $release to a commit" >&2; exit 1; }

sum() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

assets="margin-launcher_darwin_amd64 margin-launcher_darwin_arm64 \
	margin-launcher_linux_amd64 margin-launcher_linux_arm64 \
	margin_darwin_amd64 margin_darwin_arm64 \
	margin_linux_amd64 margin_linux_arm64"
for f in $assets; do
	curl -fsSL "$urlbase/$f" -o "$work/$f"
done

# The hotty version the release speaks, as its margin for this machine says it
# beside its name (gov P-5 rule 6), for desc: " (hotty 0.5)". A release from
# before margin said one has none.
self="$work/margin_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed -e s/x86_64/amd64/ -e s/aarch64/arm64/)"
chmod +x "$self"
label=$(HOME="$work" "$self" --version)
hotty=''
case "$label" in *' · hotty '*) hotty=" (hotty ${label##* · hotty })" ;; esac

platform() { case "$1" in *darwin_amd64) echo DARWIN_AMD64 ;; *darwin_arm64) echo DARWIN_ARM64 ;;
	*linux_amd64) echo LINUX_AMD64 ;; *linux_arm64) echo LINUX_ARM64 ;; esac; }

render() {
	local args=(-e "s/@VERSION@/$release/g" -e "s/@COMMIT@/$commit/" -e "s/@HOTTY@/$hotty/") f
	for f in margin-launcher_darwin_amd64 margin-launcher_darwin_arm64 \
		margin-launcher_linux_amd64 margin-launcher_linux_arm64; do
		args+=(-e "s/@LAUNCHER_$(platform "$f")@/$(sum "$work/$f")/")
	done
	for f in margin_darwin_amd64 margin_darwin_arm64 margin_linux_amd64 margin_linux_arm64; do
		args+=(-e "s/@SEED_$(platform "$f")@/$(sum "$work/$f")/")
	done
	sed "${args[@]}" "$here/margin.rb"
}

if [ -n "$out" ]; then
	render > "$out"
	exit 0
fi

if [ -n "$push" ] && [ -n "${HOMEBREW_TAP_DEPLOY_KEY:-}" ]; then
	printf '%s\n' "$HOMEBREW_TAP_DEPLOY_KEY" > "$work/key"
	chmod 600 "$work/key"
	# github.com's ed25519 host key, pinned.
	echo "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl" > "$work/known_hosts"
	export GIT_SSH_COMMAND="ssh -i $work/key -o IdentitiesOnly=yes -o UserKnownHostsFile=$work/known_hosts"
	remote="git@github.com:$tap.git"
else
	remote="https://github.com/$tap.git"
fi
git clone -q "$remote" "$work/tap"
mkdir -p "$work/tap/Formula"
render > "$work/tap/Formula/margin.rb"

cd "$work/tap"
git add Formula/margin.rb
if git diff --cached --quiet; then
	echo "margin $release: the formula is unchanged; nothing to publish"
	exit 0
fi
git --no-pager diff --cached --stat
if [ -z "$push" ]; then
	echo "(not pushed: run with --push to publish)"
	exit 0
fi
git -c user.name="Margin Agent" -c user.email="margin@neuroplast.io" \
	commit -q -m "margin $release"
git push -q origin HEAD
echo "published https://github.com/$tap"
