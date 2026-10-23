#!/bin/sh
# OCHQ installer: curl -fsSL @BASE_URL@/install.sh | sh
#
# On Debian/Ubuntu and Fedora/RHEL it adds the signed OCHQ package
# repository, so `apt upgrade` / `dnf upgrade` keep ochq up to date.
# Elsewhere (and on macOS) it installs the binary from GitHub Releases.
set -eu

BASE_URL="@BASE_URL@"
REPO="mukhiddinov/ochq"

say() { printf '\033[1m==>\033[0m %s\n' "$*"; }
die() { printf 'ochq install: %s\n' "$*" >&2; exit 1; }

SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    command -v sudo >/dev/null 2>&1 || die "run as root or install sudo"
    SUDO="sudo"
fi

fetch() {
    if command -v curl >/dev/null 2>&1; then curl -fsSL "$1"
    elif command -v wget >/dev/null 2>&1; then wget -qO- "$1"
    else die "need curl or wget"; fi
}

install_apt() {
    say "Adding the OCHQ apt repository"
    # apt >= 2.4 reads armored keys from *.asc directly, so no gpg needed.
    $SUDO install -d -m 0755 /etc/apt/keyrings
    fetch "$BASE_URL/gpg.key" | $SUDO tee /etc/apt/keyrings/ochq.asc >/dev/null
    echo "deb [signed-by=/etc/apt/keyrings/ochq.asc] $BASE_URL/apt stable main" |
        $SUDO tee /etc/apt/sources.list.d/ochq.list >/dev/null
    $SUDO apt-get update -o Dir::Etc::sourcelist=sources.list.d/ochq.list \
        -o Dir::Etc::sourceparts=- -o APT::Get::List-Cleanup=0 -qq
    $SUDO apt-get install -y ochq
}

install_dnf() {
    say "Adding the OCHQ dnf repository"
    fetch "$BASE_URL/rpm/ochq.repo" | $SUDO tee /etc/yum.repos.d/ochq.repo >/dev/null
    $SUDO dnf install -y ochq
}

install_binary() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case $(uname -m) in
        x86_64 | amd64) arch=amd64 ;;
        aarch64 | arm64) arch=arm64 ;;
        *) die "unsupported architecture $(uname -m)" ;;
    esac
    version=$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p' | head -n1)
    [ -n "$version" ] || die "could not find the latest release"

    name="ochq_${version}_${os}_${arch}"
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    say "Downloading $name"
    fetch "https://github.com/$REPO/releases/download/v$version/$name.tar.gz" > "$tmp/ochq.tar.gz"
    fetch "https://github.com/$REPO/releases/download/v$version/checksums.txt" > "$tmp/checksums.txt"

    want=$(grep " $name.tar.gz\$" "$tmp/checksums.txt" | cut -d' ' -f1)
    if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/ochq.tar.gz" | cut -d' ' -f1)
    else got=$(shasum -a 256 "$tmp/ochq.tar.gz" | cut -d' ' -f1); fi
    [ -n "$want" ] && [ "$want" = "$got" ] || die "checksum mismatch, refusing to install"

    tar -xzf "$tmp/ochq.tar.gz" -C "$tmp" ochq
    $SUDO install -m 0755 "$tmp/ochq" /usr/local/bin/ochq
}

if command -v apt-get >/dev/null 2>&1; then
    install_apt
elif command -v dnf >/dev/null 2>&1; then
    install_dnf
else
    install_binary
fi

say "Installed: $(ochq -version)"
