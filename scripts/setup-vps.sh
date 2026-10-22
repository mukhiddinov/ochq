#!/usr/bin/env bash
# One-time VPS bootstrap for ochqd (Debian/Ubuntu). Safe to re-run.
#
#   git clone https://github.com/mukhiddinov/ochq.git
#   sudo bash ochq/scripts/setup-vps.sh "ssh-ed25519 AAAA... ochq-deploy"
#
# Creates:
#   ochq    system user that runs the service (no login, no home)
#   deploy  user GitHub Actions logs in as; its key can only run ochq-deploy
set -euo pipefail

DEPLOY_PUBKEY="${1:-}"
if [[ $EUID -ne 0 ]]; then
    echo "run as root (sudo)" >&2
    exit 1
fi
if [[ ! "$DEPLOY_PUBKEY" =~ ^ssh-(ed25519|rsa)\  ]]; then
    echo "usage: setup-vps.sh '<deploy public key>'" >&2
    exit 2
fi
for cmd in systemctl sudo runuser visudo; do
    command -v "$cmd" >/dev/null || { echo "missing $cmd (apt install sudo?)" >&2; exit 1; }
done

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../deploy" && pwd)"
step() { echo "==> $*"; }

step "users"
id ochq &>/dev/null ||
    useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin ochq
id deploy &>/dev/null ||
    useradd --create-home --shell /bin/bash deploy
passwd -l deploy >/dev/null # key-only login

step "directories"
install -d -o root -g root -m 0755 /opt/ochq /opt/ochq/releases
install -d -o root -g ochq -m 0750 /etc/ochq
if [[ ! -f /etc/ochq/ochqd.env ]]; then
    install -o root -g ochq -m 0640 "$SRC/ochqd.env.example" /etc/ochq/ochqd.env
    echo "    created /etc/ochq/ochqd.env — edit OCHQD_DOMAIN before the first deploy"
fi

step "systemd unit and deploy command"
install -o root -g root -m 0644 "$SRC/ochqd.service" /etc/systemd/system/ochqd.service
install -o root -g root -m 0755 "$SRC/ochq-deploy" /usr/local/bin/ochq-deploy
systemctl daemon-reload
systemctl enable ochqd >/dev/null # started by the first deploy

step "sudo rule"
SUDOERS=/etc/sudoers.d/ochq-deploy
echo 'deploy ALL=(root) NOPASSWD: /usr/local/bin/ochq-deploy' > "$SUDOERS.tmp"
chmod 0440 "$SUDOERS.tmp"
visudo -cqf "$SUDOERS.tmp"
mv "$SUDOERS.tmp" "$SUDOERS"

step "restricted SSH key for deploy"
# restrict: no shell/pty, no port or agent forwarding.
# command=: whatever the client asks to run, only ochq-deploy runs, and the
# requested command line is passed to it as the version argument.
install -d -o deploy -g deploy -m 0700 /home/deploy/.ssh
printf 'restrict,command="sudo /usr/local/bin/ochq-deploy \\"$SSH_ORIGINAL_COMMAND\\"" %s\n' \
    "$DEPLOY_PUBKEY" > /home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 0600 /home/deploy/.ssh/authorized_keys

step "done"
echo "    host key fingerprints (compare with what ssh-keyscan shows on your laptop):"
for k in /etc/ssh/ssh_host_*_key.pub; do ssh-keygen -lf "$k"; done
