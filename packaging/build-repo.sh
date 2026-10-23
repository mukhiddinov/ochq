#!/usr/bin/env bash
# Builds signed apt and dnf repositories from .deb/.rpm files.
#
#   packaging/build-repo.sh <packages-dir> <output-dir> <gpg-key-id> <base-url>
#
# Output (served as static files, e.g. by GitHub Pages at pkg.ochq.uz):
#   gpg.key                         public key users import
#   install.sh                      one-line installer
#   apt/dists/stable/InRelease      signed index of indexes
#   apt/pool/main/*.deb
#   rpm/ochq.repo                   drop-in for /etc/yum.repos.d/
#   rpm/*.rpm, rpm/repodata/repomd.xml(.asc)
#
# The repository is rebuilt from scratch on every release, so nothing but
# the packages themselves needs to be stored anywhere.
#
# Needs: gpg, apt-ftparchive (apt-utils), createrepo_c.
set -euo pipefail

PKGS=$(realpath "$1")
OUT=$2
KEY=$3
BASE_URL=${4%/}
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# Empty the output dir rather than deleting it (it may be a mount point).
mkdir -p "$OUT"
find "$OUT" -mindepth 1 -delete
mkdir -p "$OUT/apt/pool/main" "$OUT/rpm"
OUT=$(realpath "$OUT")

gpg --batch --yes --armor --export "$KEY" > "$OUT/gpg.key"

# ---- apt -------------------------------------------------------------
cp "$PKGS"/*.deb "$OUT/apt/pool/main/"
(
    cd "$OUT/apt"
    for arch in amd64 arm64; do
        dir=dists/stable/main/binary-$arch
        mkdir -p "$dir"
        apt-ftparchive --arch "$arch" packages pool > "$dir/Packages"
        gzip -9kn "$dir/Packages"
    done
    apt-ftparchive \
        -o APT::FTPArchive::Release::Origin=OCHQ \
        -o APT::FTPArchive::Release::Label=OCHQ \
        -o APT::FTPArchive::Release::Suite=stable \
        -o APT::FTPArchive::Release::Codename=stable \
        -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
        -o APT::FTPArchive::Release::Components=main \
        release dists/stable > dists/stable/Release
    # InRelease (inline signature) is what modern apt reads; Release.gpg is
    # the detached form older clients look for.
    gpg --batch --yes --local-user "$KEY" --clearsign -o dists/stable/InRelease dists/stable/Release
    gpg --batch --yes --local-user "$KEY" --armor --detach-sign -o dists/stable/Release.gpg dists/stable/Release
)

# ---- rpm -------------------------------------------------------------
cp "$PKGS"/*.rpm "$OUT/rpm/"
createrepo_c --quiet "$OUT/rpm"
# repomd.xml lists checksums of all metadata, which lists checksums of all
# packages: signing it covers the whole repository, like InRelease for apt.
gpg --batch --yes --local-user "$KEY" --armor --detach-sign -o "$OUT/rpm/repodata/repomd.xml.asc" "$OUT/rpm/repodata/repomd.xml"
cat > "$OUT/rpm/ochq.repo" <<EOF
[ochq]
name=OCHQ
baseurl=$BASE_URL/rpm
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=$BASE_URL/gpg.key
EOF

# ---- installer ---------------------------------------------------------
sed "s|@BASE_URL@|$BASE_URL|g" "$HERE/install.sh" > "$OUT/install.sh"

echo "repository built in $OUT"
