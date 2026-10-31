#!/bin/sh
# Install files only. Does not start a service, run a backup or change containers.
set -eu
test "$(id -u)" = 0
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
umask 077
release_id=$(cat manage.py controller.py media_migrate.py auto_deploy.py capacity_collect.py capacity_report.py docker-compose.capacity.yml | sha256sum | cut -c1-16)
destination="/root/sub2api-ops/releases/$release_id"
install -d -m 700 "$destination" /root/sub2api-ops/state/releases
install -m 600 manage.py controller.py media_migrate.py auto_deploy.py capacity_collect.py capacity_report.py "$destination/"
install -m 600 docker-compose.capacity.yml "$destination/"
install -m 600 SERVER-MANAGEMENT.md /root/sub2api-ops/README.md
printf '#!/bin/sh\nexec /usr/bin/python3 %s/manage.py "$@"\n' "$destination" > /root/sub2api-manage.tmp
chmod 700 /root/sub2api-manage.tmp
mv /root/sub2api-manage.tmp /root/sub2api-manage
printf '#!/bin/sh\nexec /usr/bin/python3 %s/media_migrate.py "$@"\n' "$destination" > /root/sub2api-media-migrate.tmp
chmod 700 /root/sub2api-media-migrate.tmp
mv /root/sub2api-media-migrate.tmp /root/sub2api-media-migrate
printf '#!/bin/sh\nexec /usr/bin/python3 %s/auto_deploy.py "$@"\n' "$destination" > /root/sub2api-auto-deploy.tmp
chmod 700 /root/sub2api-auto-deploy.tmp
mv /root/sub2api-auto-deploy.tmp /root/sub2api-auto-deploy
printf '#!/bin/sh\nexec /usr/bin/python3 %s/capacity_collect.py "$@"\n' "$destination" > /root/sub2api-capacity-collect.tmp
chmod 700 /root/sub2api-capacity-collect.tmp
mv /root/sub2api-capacity-collect.tmp /root/sub2api-capacity-collect
printf '#!/bin/sh\nexec /usr/bin/python3 %s/capacity_report.py "$@"\n' "$destination" > /root/sub2api-capacity-report.tmp
chmod 700 /root/sub2api-capacity-report.tmp
mv /root/sub2api-capacity-report.tmp /root/sub2api-capacity-report
printf 'Installed /root/sub2api-manage (code %s). No production operation executed.\n' "$release_id"
