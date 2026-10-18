#!/usr/bin/env bash
#
# Publish margin-bin to the AUR: a margin release's launcher, installed as
# margin, and its margin as the launcher's seed (D20).
#
#   packaging/aur/publish.sh <release> [--push]
#
# <release> is a margin release tag (26.10.03). The script downloads that
# release's four Linux files from its GitHub release — the launcher and margin
# for amd64 and arm64 — and the package moves when either file's bytes change
# (or the PKGBUILD does), and keeps its version otherwise:
#
#   • the AUR has no margin-bin yet, or other bytes: the package becomes
#     <release>-1 (or the next pkgrel, when <release> was replaced under its
#     name with other bytes);
#   • the same bytes: the package keeps its version, and is published again
#     with the next pkgrel only if packaging/aur/PKGBUILD changed;
#   • otherwise there is nothing to publish.
#
# Either way the package is built as a user's makepkg would build it, from the
# GitHub release, and its seed is run, before anything is pushed. Without
# --push it stops there and shows what it would publish. Needs makepkg, so an
# Arch system. --push writes with AUR_SSH_PRIVATE_KEY when set (the CI path: a
# key of the AUR account the package belongs to), and with your own SSH setup
# otherwise.
set -euo pipefail

if [ $# -lt 1 ] || [ $# -gt 2 ] || { [ $# -eq 2 ] && [ "$2" != --push ]; }; then
	echo "usage: $0 <release> [--push]" >&2
	exit 2
fi
release=$1 push=${2:-}
here=$(cd "$(dirname "$0")" && pwd)
pkg=margin-bin
urlbase="https://github.com/neuroplastio/margin/releases/download/$release"

case "$release" in
[0-9][0-9].[0-9][0-9].[0-9][0-9]) ;;
*) echo "publish: $release is not a margin release (YY.MM.DD)" >&2; exit 1 ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir "$work/release"
for f in margin-launcher_linux_amd64 margin_linux_amd64 margin-launcher_linux_arm64 margin_linux_arm64; do
	curl -fsSL "$urlbase/$f" -o "$work/release/$f"
done
sum() { sha256sum "$work/release/$1" | cut -d' ' -f1; }
l_amd64=$(sum margin-launcher_linux_amd64)
s_amd64=$(sum margin_linux_amd64)
l_arm64=$(sum margin-launcher_linux_arm64)
s_arm64=$(sum margin_linux_arm64)

if [ -n "$push" ]; then
	if [ -n "${AUR_SSH_PRIVATE_KEY:-}" ]; then
		printf '%s\n' "$AUR_SSH_PRIVATE_KEY" > "$work/key"
		chmod 600 "$work/key"
		# The AUR's host key; its fingerprint is on https://aur.archlinux.org.
		echo "aur.archlinux.org ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEuBKrPzbawxA/k2g6NcyV5jmqwJ2s+zpgZGZ7tpLIcN" > "$work/known_hosts"
		export GIT_SSH_COMMAND="ssh -i $work/key -o IdentitiesOnly=yes -o UserKnownHostsFile=$work/known_hosts"
	fi
	remote=ssh://aur@aur.archlinux.org/$pkg.git
else
	remote=https://aur.archlinux.org/$pkg.git
fi
# A package nobody has published yet clones as an empty repository, and has
# no .SRCINFO.
git -c init.defaultBranch=master -c advice.defaultBranchName=false \
	clone -q "$remote" "$work/aur" 2>&1 | grep -v 'cloned an empty repository' >&2 || true
test -d "$work/aur/.git"

srcinfo() {
	[ -f "$work/aur/.SRCINFO" ] || return 0
	sed -n "s/^\t$1 = //p" "$work/aur/.SRCINFO" | head -n 1
}
cur_ver=$(srcinfo pkgver)
cur_rel=$(srcinfo pkgrel)
# All four sums, in the PKGBUILD's order: each architecture's launcher, then
# its seed.
cur_sums=
if [ -f "$work/aur/.SRCINFO" ]; then
	cur_sums=$(sed -n -e 's/^\tsha256sums_x86_64 = //p' -e 's/^\tsha256sums_aarch64 = //p' "$work/aur/.SRCINFO" | xargs)
fi

render() {
	sed -e "s/@PKGVER@/$1/" -e "s/@PKGREL@/$2/" \
		-e "s/@SHA256_LAUNCHER_X86_64@/$l_amd64/" -e "s/@SHA256_SEED_X86_64@/$s_amd64/" \
		-e "s/@SHA256_LAUNCHER_AARCH64@/$l_arm64/" -e "s/@SHA256_SEED_AARCH64@/$s_arm64/" \
		"$here/PKGBUILD"
}

if [ -z "$cur_ver" ]; then
	ver=$release rel=1 why="margin-bin's first version"
elif [ "$l_amd64 $s_amd64 $l_arm64 $s_arm64" != "$cur_sums" ]; then
	if [ "$release" = "$cur_ver" ]; then
		ver=$cur_ver rel=$((cur_rel + 1)) why="the package's files for $release changed"
	else
		ver=$release rel=1 why="margin $release"
	fi
elif render "$cur_ver" "$cur_rel" | cmp -s - "$work/aur/PKGBUILD"; then
	echo "margin-bin $cur_ver-$cur_rel: margin $release has the same files; nothing to publish"
	exit 0
else
	ver=$cur_ver rel=$((cur_rel + 1)) why="the PKGBUILD changed; the files are still $cur_ver's"
fi

render "$ver" "$rel" > "$work/aur/PKGBUILD"
cd "$work/aur"
makepkg --printsrcinfo > .SRCINFO

# Build it as a user's makepkg would: fetched from the GitHub release, its
# sha256s checked, packaged. Everything makepkg writes stays in $work.
PKGDEST="$work/out" SRCDEST="$work/src" BUILDDIR="$work/build" \
	makepkg --force --nodeps --noconfirm >"$work/makepkg.log" 2>&1 ||
	{ cat "$work/makepkg.log" >&2; exit 1; }
built=$(ls "$work/out"/*.pkg.tar.*)
for f in usr/bin/margin usr/lib/margin/bin/margin; do
	bsdtar -tf "$built" | grep -qx "$f" ||
		{ echo "$built has no $f" >&2; exit 1; }
done
# The seed is the release: it runs here and names itself after it. Only on
# the architecture this runs on; the other's bytes are checked by their sha256.
mkdir "$work/pkg"
bsdtar -xf "$built" -C "$work/pkg" usr/lib/margin/bin/margin
said=$(HOME="$work" "$work/pkg/usr/lib/margin/bin/margin" --version)
case "$said" in
*"$ver"*) ;;
*) echo "the seed says it is \"$said\", not margin $ver" >&2; exit 1 ;;
esac

echo "margin-bin $ver-$rel: $why"
git add --intent-to-add PKGBUILD .SRCINFO
git --no-pager diff --stat
if [ -z "$push" ]; then
	git --no-pager diff
	echo "(not pushed: run with --push to publish)"
	exit 0
fi
git add PKGBUILD .SRCINFO
git -c user.name="Margin Agent" -c user.email="margin@neuroplast.io" \
	commit -q -m "$ver-$rel: $why"
git push -q origin HEAD:master
echo "published https://aur.archlinux.org/packages/$pkg"
