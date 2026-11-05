#!/usr/bin/env bash
#
# Prove margin's Homebrew formula before it is pushed to the tap:
#
#   packaging/homebrew/check.sh <release>
#
# Renders the formula for <release> into a throwaway local tap, trusted for the
# run and untrusted after it, then runs
# `brew audit --strict`, `brew style`, a real `brew install` and the
# formula's `brew test`, and finally `margin --version` with an empty HOME, so
# the seed is what runs. Needs Homebrew; CI runs it on macOS and Linux.
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: $0 <release>" >&2; exit 2; }
release=$1
here=$(cd "$(dirname "$0")" && pwd)

tap=neuroplastio/margin-proof trusted=''
brew tap-new --no-git "$tap" >/dev/null
dir="$(brew --repository "$tap")"
trap 'brew uninstall --force margin >/dev/null 2>&1 || true
	[ -z "$trusted" ] || brew untrust "$tap" >/dev/null 2>&1 || true
	brew untap "$tap" >/dev/null 2>&1 || true' EXIT

# Homebrew 7 refuses to load a formula from a tap no one has trusted: `brew
# install` of a full name is allowed, but `brew audit` and `brew test` are not.
# The tap is the one just made, so trust it, where the brew has `brew trust`;
# an older brew has no trust to give and loads it as it is. `brew untap` keeps
# the trust entry, so the trap removes it.
if brew trust --help >/dev/null 2>&1; then
	brew trust "$tap"
	trusted=1
fi

mkdir -p "$dir/Formula"
"$here/publish.sh" "$release" --render "$dir/Formula/margin.rb"

brew style "$dir/Formula/margin.rb"
brew audit --strict --formula "$tap/margin"
brew install "$tap/margin"
brew test "$tap/margin"

home=$(mktemp -d)
HOME=$home "$(brew --prefix)/bin/margin" --version | tee "$home/version"
grep -q "^margin $release " "$home/version"
test -x "$home/.local/margin/bin/margin"
echo "margin $release: the formula audits, installs and runs"
