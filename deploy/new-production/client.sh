#!/usr/bin/env bash
# Shared SSH protocol for the host-owned production deployment gateway.
set -Eeuo pipefail
[[ "$DEPLOY_HOST" =~ ^[a-zA-Z0-9.-]+$ ]]
[[ "$DEPLOY_USER" =~ ^[a-z_][a-z0-9_-]*$ ]]
[[ "$DEPLOY_PORT" =~ ^[0-9]+$ ]]
test -n "$DEPLOY_KEY" && test -n "$KNOWN_HOSTS"
umask 077
ssh_dir=$(mktemp -d)
trap 'rm -rf "$ssh_dir"' EXIT
printf '%s\n' "$DEPLOY_KEY" > "$ssh_dir/key"
printf '%s\n' "$KNOWN_HOSTS" > "$ssh_dir/known_hosts"
SSH=(ssh -T -i "$ssh_dir/key" -p "$DEPLOY_PORT" -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=20 -o ServerAliveInterval=20 -o ServerAliveCountMax=6 -o UserKnownHostsFile="$ssh_dir/known_hosts" "$DEPLOY_USER@$DEPLOY_HOST")
if [[ "${1:-}" == verify ]]; then
  "${SSH[@]}" verify | tee "$ssh_dir/receipt.json"
  python3 - "$ssh_dir/receipt.json" <<'PY'
import json,sys
r=json.load(open(sys.argv[1])); assert r['status']=='succeeded' and r['host']=='206.82.2.46'
PY
  cat "$ssh_dir/receipt.json" >> "$GITHUB_STEP_SUMMARY"
  exit 0
fi
revision=$1
archive=$2
[[ "$revision" =~ ^([0-9a-f]{40}|v(0|[1-9][0-9]*)\.[0-9]+\.[0-9]+)$ ]]
digest=$(sha256sum "$archive" | cut -d' ' -f1)
"${SSH[@]}" "deploy $revision $digest" < "$archive" > "$ssh_dir/receipt.json"
job=$(python3 -c 'import json,re,sys; j=json.load(open(sys.argv[1])); assert re.fullmatch("[0-9a-f]{32}", j.get("id", "")); print(j["id"])' "$ssh_dir/receipt.json")
echo "Submitted durable deployment job $job"
for attempt in $(seq 1 660); do
  # A transient SSH interruption does not cancel a detached production job.
  if ! "${SSH[@]}" "status $job" > "$ssh_dir/status.json"; then
    sleep 15
    continue
  fi
  cat "$ssh_dir/status.json"
  status=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["status"])' "$ssh_dir/status.json")
  if [[ "$status" == succeeded ]]; then
    cat "$ssh_dir/status.json" >> "$GITHUB_STEP_SUMMARY"
    exit 0
  fi
  case "$status" in failed|failed_rolled_back|failed_needs_inspection|rejected) exit 1 ;; esac
  sleep 15
done
echo "Timed out waiting for durable job $job; inspect its host status." >&2
exit 1
