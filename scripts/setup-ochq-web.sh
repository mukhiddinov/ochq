#!/usr/bin/env bash
# One-time VPS bootstrap for ochq-web, the ochq.uz landing page. Safe to re-run.
#
#   git -C ochq pull
#   sudo bash ochq/scripts/setup-ochq-web.sh
#
# Creates the ochq_web system user that runs the service (no login, no
# home). CI deploys with ochqd's existing key: ochq-deploy hands
# "web <version>" uploads to ochq-web-deploy, so no new key or secret is
# needed. Run setup-vps.sh first (it creates ochq_deploy and /etc/ochq).
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
    echo "run as root (sudo)" >&2
    exit 1
fi
for cmd in systemctl runuser ss docker curl openssl; do
    command -v "$cmd" >/dev/null || { echo "missing $cmd" >&2; exit 1; }
done
[[ -d /etc/ochq && -x /usr/local/bin/ochq-deploy ]] || { echo "run setup-vps.sh first" >&2; exit 1; }

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

step "directories and settings"
install -d -o root -g root -m 0755 /opt/ochq-web /opt/ochq-web/releases
if [[ ! -f /etc/ochq/ochq-web.env ]]; then
    # Never overwritten: the salt must stay the same forever.
    sed "s/^OCHQ_WEB_SALT=$/OCHQ_WEB_SALT=$(openssl rand -hex 32)/" "$SRC/ochq-web.env.example" > /etc/ochq/ochq-web.env
    chown root:root /etc/ochq/ochq-web.env
    chmod 0600 /etc/ochq/ochq-web.env
    echo "    created /etc/ochq/ochq-web.env with a new salt"
fi

step "systemd unit and deploy commands"
install -o root -g root -m 0644 "$SRC/ochq-web.service" /etc/systemd/system/ochq-web.service
install -o root -g root -m 0755 "$SRC/ochq-web-deploy" /usr/local/bin/ochq-web-deploy
# The updated ochq-deploy knows "web <version>"; ochqd deploys are unchanged.
install -o root -g root -m 0755 "$SRC/ochq-deploy" /usr/local/bin/ochq-deploy
# For moderation: ochq-web list / hide / unhide. The binary is whatever CI
# uploaded last, so it runs as ochq_web, never as root: a leaked deploy
# key must not turn into root the next time someone moderates.
rm -f /usr/local/bin/ochq-web # was a symlink in the first version of this script
cat > /usr/local/bin/ochq-web <<'WRAPPER'
#!/bin/sh
exec runuser -u ochq_web -- /opt/ochq-web/current/ochq-web "$@"
WRAPPER
chmod 0755 /usr/local/bin/ochq-web
systemctl daemon-reload
systemctl enable ochq-web >/dev/null # started by the first deploy

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
