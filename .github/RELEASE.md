# Public releases

Sub2API releases are public from v1.2.19 onward. The existing repository and
package names are retained so production manifests and rollback references remain
valid; `sub2api-private` is now a legacy name, not a visibility requirement.

- The repository, formal GitHub Releases, application image and documentation
  image must be public. Release workflows verify visibility and anonymous pulls.
- Push a stable `vMAJOR.MINOR.PATCH` tag, or dispatch `release.yml` for an existing
  unpublished tag. A published release and its image digest are immutable.
- If source tests and image builds passed but publication failed, dispatch
  `finalize-private-release.yml` with the original tag, exact application and docs
  digests, and source build run. It verifies provenance and reruns image checks
  and deployment rehearsals before publishing; it does not move the source tag.
- Production switching remains a separate operation. Use the host manager after
  release verification, backup and restore rehearsal. `--immediate` is available
  for an explicitly requested immediate cutover.
- Production credentials, database dumps, backups and operational logs are never
  release assets. Existing deployment paths keep their names for compatibility.
