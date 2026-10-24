#!/usr/bin/env bash
# Builds the COPR source RPM for one version of the repository at HEAD:
#   packaging/copr/srpm.sh 0.3.2 out/
# The source tarball includes vendor/ so COPR can build offline.
set -euo pipefail

version=${1:?usage: srpm.sh VERSION OUTDIR}
out=$(mkdir -p "${2:?usage: srpm.sh VERSION OUTDIR}" && cd "$2" && pwd)
root=$(git rev-parse --show-toplevel)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

src="$work/ochq-$version"
mkdir -p "$src" "$work/SOURCES" "$work/SPECS"
git -C "$root" archive HEAD | tar -x -C "$src"
(cd "$src" && go mod vendor)
tar -czf "$work/SOURCES/ochq-$version.tar.gz" -C "$work" "ochq-$version"

sed "s/@VERSION@/$version/" "$root/packaging/copr/ochq.spec" > "$work/SPECS/ochq.spec"
rpmbuild -bs --define "_topdir $work" --define "_srcrpmdir $out" "$work/SPECS/ochq.spec"
