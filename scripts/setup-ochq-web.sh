#!/usr/bin/env bash
# One-time VPS bootstrap for ochq-web, the ochq.uz landing page. Safe to re-run.
#
#   git -C ochq pull
#   sudo bash ochq/scripts/setup-ochq-web.sh "ssh-ed25519 AAAA... ochq-web-deploy"
#
# Creates:
#   ochq_web          system user that runs the service (no login, no home)
#   ochq_web_deploy   user GitHub Actions logs in as; its key can only run ochq-web-deploy
#
# Separate from ochqd's users and deploy key, so the two never touch each
# other. Run setup-vps.sh first (it creates /etc/ochq).
set -euo pipefail

DEPLOY_PUBKEY="${1:-}"
if [[ $EUID -ne 0 ]]; then
    echo "run as root (sudo)" >&2
    exit 1
fi
# Validate with ssh-keygen: a placeholder or truncated paste must not end up
# in authorized_keys, where it would silently lock CI out.
if ! ssh-keygen -l -f - <<<"$DEPLOY_PUBKEY" >/dev/null 2>&1; then
    echo "usage: setup-ochq-web.sh '<deploy public key>'" >&2
    echo "  not a valid SSH public key: ${DEPLOY_PUBKEY:0:40}..." >&2
    exit 2
fi
for cmd in systemctl sudo runuser visudo ss getent docker curl openssl; do
    command -v "$cmd" >/dev/null || { echo "missing $cmd" >&2; exit 1; }
done
[[ -d /etc/ochq ]] || { echo "/etc/ochq is missing: run setup-vps.sh first" >&2; exit 1; }

DEPLOY_USER=ochq_web_deploy
PORT=8090
WANT_GATEWAY=172.30.0.1
TRAEFIK_IP=172.30.0.2

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../deploy" && pwd)"
step() { echo "==> $*"; }

step "edge network"
# ochq-web listens on the edge network's gateway: Traefik can reach it,
# the internet cannot.
gateway=$(docker network inspect edge -f '{{(index .IPAM.Config 0).Gateway}}')
if [[ "$gateway" != "$WANT_GATEWAY" ]]; then
    echo "    the edge network's gateway is $gateway, not $WANT_GATEWAY:" >&2
    echo "    set OCHQ_WEB_ADDR=$gateway:$PORT in /etc/ochq/ochq-web.env and" >&2
    echo "    the vps-edge ochq-web service URL to http://$gateway:$PORT" >&2
    exit 1
fi

step "users"
id ochq_web &>/dev/null ||
    useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin ochq_web
id "$DEPLOY_USER" &>/dev/null ||
    useradd --create-home --shell /bin/bash "$DEPLOY_USER"
passwd -l "$DEPLOY_USER" >/dev/null # key-only login

step "directories and settings"
install -d -o root -g root -m 0755 /opt/ochq-web /opt/ochq-web/releases
if [[ ! -f /etc/ochq/ochq-web.env ]]; then
    # Never overwritten: the salt must stay the same forever.
    sed "s/^OCHQ_WEB_SALT=$/OCHQ_WEB_SALT=$(openssl rand -hex 32)/" "$SRC/ochq-web.env.example" > /etc/ochq/ochq-web.env
    chown root:root /etc/ochq/ochq-web.env
    chmod 0600 /etc/ochq/ochq-web.env
    echo "    created /etc/ochq/ochq-web.env with a new salt"
fi

step "systemd unit and deploy command"
install -o root -g root -m 0644 "$SRC/ochq-web.service" /etc/systemd/system/ochq-web.service
install -o root -g root -m 0755 "$SRC/ochq-web-deploy" /usr/local/bin/ochq-web-deploy
# For moderation from a root shell: ochq-web list / hide / unhide.
ln -sfn /opt/ochq-web/current/ochq-web /usr/local/bin/ochq-web
systemctl daemon-reload
systemctl enable ochq-web >/dev/null # started by the first deploy

step "sudo rule"
SUDOERS=/etc/sudoers.d/ochq-web-deploy
echo "$DEPLOY_USER ALL=(root) NOPASSWD: /usr/local/bin/ochq-web-deploy" > "$SUDOERS.tmp"
chmod 0440 "$SUDOERS.tmp"
visudo -cqf "$SUDOERS.tmp"
mv "$SUDOERS.tmp" "$SUDOERS"

step "restricted SSH key for $DEPLOY_USER"
# restrict: no shell/pty, no port or agent forwarding.
# command=: whatever the client asks to run, only ochq-web-deploy runs, and
# the requested command line is passed to it as the version argument.
DEPLOY_HOME=$(getent passwd "$DEPLOY_USER" | cut -d: -f6)
install -d -o "$DEPLOY_USER" -g "$DEPLOY_USER" -m 0700 "$DEPLOY_HOME/.ssh"
printf 'restrict,command="sudo /usr/local/bin/ochq-web-deploy \\"$SSH_ORIGINAL_COMMAND\\"" %s\n' \
    "$DEPLOY_PUBKEY" > "$DEPLOY_HOME/.ssh/authorized_keys"
chown "$DEPLOY_USER:$DEPLOY_USER" "$DEPLOY_HOME/.ssh/authorized_keys"
chmod 0600 "$DEPLOY_HOME/.ssh/authorized_keys"

step "port check"
if ss -Hltn "sport = :$PORT" | grep -q . && ! ss -Hltnp "sport = :$PORT" | grep -q ochq-web; then
    echo "    WARNING: port $PORT is already in use:"
    ss -Hltnp "sport = :$PORT" | sed 's/^/      /'
    echo "    change OCHQ_WEB_ADDR in /etc/ochq/ochq-web.env and the vps-edge route"
fi

step "done"
echo "    If ufw is active, let Traefik (and only Traefik) reach ochq-web:"
echo "      ufw allow from $TRAEFIK_IP to $WANT_GATEWAY port $PORT proto tcp"
echo "    Moderate comments with:  ochq-web list | ochq-web hide <id> | ochq-web unhide <id>"
