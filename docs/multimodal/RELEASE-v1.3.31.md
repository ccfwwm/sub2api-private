# Sub2API v1.3.31

- Managed image, audio and video tasks retry on compatible accounts in the same group, ordered by account priority. Each task has up to three supplier submissions and keeps its original task ID and billing hold.
- Upstream service failures confirmed for an accepted job can trigger a retry when the task has no partial output and its customer charge is still held. Uncertain submissions continue through the existing recovery path.
- Three consecutive qualifying failures open the account circuit for 60 seconds or the provider's longer Retry-After. One recovery probe may run after the cooldown. A successful probe clears the circuit.
- The homepage preserves the original orbit angle while data and light move through the artwork. The update removes the line beneath the hub, the example route, and playback controls on the homepage and authentication pages.

Managed media failover uses the existing gateway.media_failover_mode setting. The database migration inventory matches v1.3.30. Channel evidence remains historical.
