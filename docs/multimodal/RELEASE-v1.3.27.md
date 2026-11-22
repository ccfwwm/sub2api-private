# Sub2API v1.3.27

Merges upstream v0.2.24 at 5c2982a81b46e2142d0143d2f83c1ce9c4da1d24. This release includes upstream request validation, scheduling and streaming fixes. Existing clients that send duplicate fields will receive the upstream validation error.

The optional `SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS_FILE` setting accepts a JSON array of exact DNS names and requires URL allowlist validation. The process loads the initial policy before startup and polls for changes once per second. A rejected update retains the applied policy. A valid empty array denies all custom upstream hosts. The localhost-only endpoint `GET /internal/upstream-allowlist/status` returns the SHA-256 of the loaded file and its exact hosts. Mount the parent directory read-only so that atomic file replacement is visible.

Production uses the existing 317 SQL migration files. Historical channel verification retains its original version and dates.
