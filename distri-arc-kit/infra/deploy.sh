#!/usr/bin/env bash
# Deploy from GitHub on a server with nginx + systemd (docs/DEPLOY.md §C). Run as root:
#   /opt/distri-arc/repo/distri-arc-kit/infra/deploy.sh [branch]
# Pulls the branch (deploy key of the arc user), builds bin/arc and web/dist as arc, installs them into
# /opt/distri-arc/{bin,web,infra}, migrates, restarts api + worker, and checks /api/health. A failed build or
# migration leaves the running version untouched.
set -euo pipefail

BRANCH="${1:-${DEPLOY_BRANCH:-distri-arc-orbit}}"
REPO_URL="${REPO_URL:-git@github.com:itgsigroup/crm_distri.git}"
ROOT=/opt/distri-arc
SRC="$ROOT/repo"
KIT="$SRC/distri-arc-kit"
ENV_FILE=/etc/distri-arc/env
as_arc() { sudo -u arc -H bash -lc "$*"; }

[[ $EUID -eq 0 ]] || { echo "deploy: run as root" >&2; exit 1; }
[[ -f $ENV_FILE ]] || { echo "deploy: $ENV_FILE missing (see docs/DEPLOY.md §C)" >&2; exit 1; }

if [[ ! -d $SRC/.git ]]; then
  as_arc "git clone --branch $BRANCH $REPO_URL $SRC"
else
  as_arc "cd $SRC && git fetch --prune origin && git checkout -q $BRANCH && git reset -q --hard origin/$BRANCH"
fi
REV="$(as_arc "cd $SRC && git rev-parse --short HEAD")"
echo "deploy: $BRANCH @ $REV"

# build (as arc; Go fetches the toolchain from go.mod when needed)
as_arc "cd $KIT && export PATH=/usr/local/go/bin:/usr/local/bin:\$PATH GOTOOLCHAIN=auto && go build -trimpath -ldflags '-s -w -X distri-arc/internal/config.Version=$REV' -o bin/arc.new ./cmd/arc"
as_arc "cd $KIT/web && npm ci --no-audit --no-fund --silent && npm run build --silent"

# install
install -d -o arc -g arc "$ROOT/bin" "$ROOT/web" "$ROOT/infra"
install -o arc -g arc -m 755 "$KIT/bin/arc.new" "$ROOT/bin/arc.new"
rsync -a --delete "$KIT/web/dist/" "$ROOT/web.new/"
rsync -a --delete "$KIT/infra/" "$ROOT/infra/"
chown -R arc:arc "$ROOT/web.new" "$ROOT/infra"

# migrate with the new binary before switching (goose is transactional per migration)
sudo -u arc bash -c "set -a; . $ENV_FILE; set +a; $ROOT/bin/arc.new ctl migrate"

mv -f "$ROOT/bin/arc.new" "$ROOT/bin/arc"
rm -rf "$ROOT/web.old" && [[ -d $ROOT/web ]] && mv "$ROOT/web" "$ROOT/web.old"
mv "$ROOT/web.new" "$ROOT/web"

for f in distri-arc-api.service distri-arc-worker.service distri-arc-backup.service distri-arc-backup.timer; do
  install -m 644 "$KIT/infra/systemd/$f" "/etc/systemd/system/$f"
done
systemctl daemon-reload
systemctl enable --now distri-arc-backup.timer >/dev/null
systemctl enable distri-arc-api distri-arc-worker >/dev/null
systemctl restart distri-arc-api distri-arc-worker

ADDR="$(grep -E '^API_ADDR=' $ENV_FILE | cut -d= -f2)"
for i in $(seq 1 20); do
  if curl -fsS "http://${ADDR:-127.0.0.1:8080}/api/health" >/dev/null 2>&1; then
    echo "deploy: OK · $REV · $(curl -fsS "http://${ADDR:-127.0.0.1:8080}/api/health")"
    exit 0
  fi
  sleep 1
done
echo "deploy: api not healthy — journalctl -u distri-arc-api -n 50" >&2
exit 1
