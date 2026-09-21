#!/bin/sh
# Installation changes host configuration. Run only after operator approval.
set -eu
test "$(id -u)" = 0
test "${1:-}" = '--confirm-host-install' || {
  echo 'Host installation requires --confirm-host-install. Production remains disabled.' >&2
  exit 1
}
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
install -d -m 700 /etc/sub2api-deployer /var/lib/sub2api-deployer
install -d -m 755 /opt/sub2api-deployer
install -m 755 controller.py /opt/sub2api-deployer/controller.py
if [ ! -e /etc/sub2api-deployer/config.json ]; then
  install -m 600 config.example.json /etc/sub2api-deployer/config.json
fi
install -m 644 sub2api-deployer.service /etc/systemd/system/sub2api-deployer.service
systemctl daemon-reload
systemctl enable --now sub2api-deployer.service
echo 'Controller installed. Configure dedicated credentials and explicitly enable before preparing a release.'
