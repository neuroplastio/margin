#!/bin/sh
# Install margin into a directory you own, with no package manager and no root
# (D22): margin-launcher at <bindir>/margin, and a complete margin as a seed in
# the launcher's home, ~/.local/margin. The launcher runs the seed, so the first
# run needs no network and margin keeps running when the channel is down;
# `margin update` then moves the build in the home, as it does under a package.
#
#   curl -fsSL https://raw.githubusercontent.com/neuroplastio/margin/main/install.sh | sh
#
# It trusts nothing but the release key pinned below, and uses only tools
# already on the machine: curl, ssh-keygen, sha256sum (or shasum). Read from
# the environment:
#   MARGIN_URL      where channels are served  (https://pkg.neuroplast.io)
#   MARGIN_CHANNEL  the channel                (stable)
#   MARGIN_COMMIT   a particular build         (the channel's newest)
#   MARGIN_BINDIR   where the launcher goes    (~/.local/bin)
#   MARGIN_FORCE    reseed the home even when it holds a build  (empty)
#   MARGIN_SIGNERS  another key than the pinned one  (the release key below)
set -eu

url="${MARGIN_URL:-https://pkg.neuroplast.io}"
project=margin
channel="${MARGIN_CHANNEL:-stable}"
base="$url/$project/$channel"
bindir="${MARGIN_BINDIR:-$HOME/.local/bin}"
home="$HOME/.local/$project"
# The public half of the key every margin is signed with, pinned here rather
# than fetched: a key from the server it vouches for would prove nothing. The
# same key is pinned in internal/update.
signers="${MARGIN_SIGNERS:-release@neuroplast.io namespaces=\"engram\" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDCsEZcn1tiubvKQzEVs5pJ4QVXoFmSDtO+k6SdetUdf}"

case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; *) echo "install: margin is not built for $(uname -s)" >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; aarch64 | arm64) arch=arm64 ;; *) echo "install: margin is not built for $(uname -m)" >&2; exit 1 ;; esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# field <key> — the value of key= on the line on stdin. The whole grammar is a
# split on spaces and a split on '='.
field() { tr ' ' '\n' | sed -n "s/^$1=//p" | head -n 1; }
sum() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

commit="${MARGIN_COMMIT:-}"
if [ -z "$commit" ]; then
	# The head is unsigned: it only says where to look.
	curl -fsS -o "$tmp/head" "$base/head" ||
		{ echo "install: cannot read the head of $project/$channel" >&2; exit 1; }
	commit="$(grep '^publish ' "$tmp/head" | field commit)"
	[ -n "$commit" ] || { echo "install: $project/$channel holds no build" >&2; exit 1; }
fi

curl -fsS -o "$tmp/manifest" "$base/builds/$commit/manifest"
curl -fsS -o "$tmp/manifest.sig" "$base/builds/$commit/manifest.sig"

printf '%s\n' "$signers" >"$tmp/allowed_signers"
identity="${signers%% *}"
ssh-keygen -Y verify -f "$tmp/allowed_signers" -I "$identity" -n engram \
	-s "$tmp/manifest.sig" <"$tmp/manifest" >/dev/null ||
	{ echo "install: the manifest of $commit is not signed by the pinned key" >&2; exit 1; }

# Signed for another project, channel or commit is as bad as not signed.
build="$(grep '^build ' "$tmp/manifest")"
[ "$(echo "$build" | field project)" = "$project" ] &&
	[ "$(echo "$build" | field channel)" = "$channel" ] &&
	[ "$(echo "$build" | field commit)" = "$commit" ] ||
	{ echo "install: the signed manifest is not for $project/$channel $commit" >&2; exit 1; }
version="$(echo "$build" | field version)"

# fetch <name> <dest> — one artifact, checked against the sha256 the signed
# manifest names for it.
fetch() {
	line="$(grep '^artifact ' "$tmp/manifest" | grep " name=$1 " | grep " os=$os " | grep " arch=$arch " | head -n 1)"
	[ -n "$line" ] || { echo "install: $version has no $1 for $os/$arch" >&2; exit 1; }
	path="$(echo "$line" | field path)"
	curl -fsS -o "$2" "$base/builds/$commit/$path"
	[ "$(sum "$2")" = "$(echo "$line" | field sha256)" ] ||
		{ echo "install: $path does not match the signed manifest" >&2; exit 1; }
}
# Both are fetched and checked before either is installed.
fetch margin-launcher "$tmp/launcher"
fetch margin "$tmp/margin"

mkdir -p "$bindir"
cp "$tmp/launcher" "$tmp/launcher.new"
chmod 755 "$tmp/launcher.new"
mv -f "$tmp/launcher.new" "$bindir/margin"

# The seed, laid out the way enlaunch lays out a build it fetched. A home that
# already holds a build — `margin update` put a newer one there — is kept
# unless MARGIN_FORCE is set: the launcher runs it either way.
seeded=""
if [ -n "${MARGIN_FORCE:-}" ] || [ ! -x "$home/bin/$project" ]; then
	mkdir -p "$home/builds/$commit"
	cp "$tmp/margin" "$home/builds/$commit/$project"
	chmod 755 "$home/builds/$commit/$project"
	ln -sfn "builds/$commit" "$home/bin"
	seeded=yes
fi

echo "installed margin $version ($commit), verified"
echo "  launcher  $bindir/margin"
if [ -n "$seeded" ]; then
	echo "  margin    $home/builds/$commit/margin"
else
	echo "  margin    kept the build already in $home (MARGIN_FORCE=1 to replace it)"
fi
case ":$PATH:" in
*":$bindir:"*) ;;
*) echo "  $bindir is not on your PATH:  export PATH=\"$bindir:\$PATH\"" ;;
esac
