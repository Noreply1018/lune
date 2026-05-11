# Draft Backlog

This file holds design drafts that were previously parked under released version specs but were not implemented in those releases.

## External CPA Advanced Mode

Source: previous `spec/v0.1.4.md`

Status: not implemented as a complete user-facing mode.

### Goal

Keep the all-in-one image as the default while providing an advanced path for users who intentionally want to run CPA outside the Lune container.

### Why

Embedded CPA gives Lune a better default product boundary, but some users may need external CPA for:

- Faster CPA upgrades independent of Lune releases.
- Custom CPA configuration.
- Existing CPA deployments.
- Debugging or operating CPA separately.

### Draft Direction

- Default remains embedded CPA.
- External CPA must be opt-in.
- External CPA should be configured with explicit Lune environment variables.
- Documentation should clearly mark this as advanced.
- The default quick start should not mention external CPA.
- Docker Compose and `.env.example` should expose a coherent external CPA path instead of requiring users to infer it from internal variables.
- Settings should distinguish `embedded` from `external` runtime mode instead of always displaying `embedded`.

### Open Questions

- What exact env var disables embedded CPA in documented production Compose?
- Should Lune still auto-create or auto-update the CPA service row when external CPA is configured?
- Should external CPA support require users to provide both API key and management key?
- How much of the old two-container Compose setup should remain documented?
- Should tests cover switching from embedded CPA to external CPA and back?

## Managed CPA Update

Source: previous `spec/v0.1.4.md`

Status: not implemented.

### Goal

Allow advanced users to update the embedded CPA binary independently from the Lune image, without changing the default image-pinned behavior.

The default user path should still upgrade CPA by upgrading the Lune image.

### Possible Modes

- `embedded_pinned`: default. CPA comes from the Lune image and changes only when Lune is upgraded.
- `external`: Lune connects to a user-managed CPA service.
- `managed`: Lune downloads and runs a CPA binary under the data directory, with explicit user action.

### Draft Direction

If `managed` mode is implemented, it should be manual by default:

- Settings shows current running CPA version, image-pinned CPA version, and latest available CPA version.
- User clicks an explicit update action.
- Lune downloads only from trusted release sources.
- Lune verifies checksum or signature before execution.
- Lune keeps the previous CPA binary for rollback.
- Lune restarts only the CPA child process after a successful update.
- Lune records update attempts, versions, and failures in system/activity logs.
- Lune clearly marks the runtime as diverged from the image-pinned CPA version.

### Non-Goal

- No silent automatic CPA binary replacement.

### Open Questions

- Should managed CPA updates live in a near-term release or remain an advanced-only future capability?
- What release metadata source and verification mechanism should be trusted for managed CPA downloads?
- Should rollback be automatic on health-check failure or only manually triggered?
- How should managed mode interact with container immutability and mounted data directories?

## Large Non-Stream Response Streaming

Source: previous `spec/v0.1.5.md`

Status: not implemented.

### Goal

Reduce memory pressure when upstream providers return large non-stream responses, especially for `/v1/responses` image and file workflows.

### Current Problem

Lune currently buffers non-stream upstream responses in memory before writing them to the client.

This is useful for retry behavior and usage parsing, but it can become expensive when the upstream response is large.

### Draft Direction

- Keep buffering small non-stream responses in memory.
- For larger non-stream responses, stream to the client instead of holding the full body.
- Preserve bounded metadata needed for Activity and usage tracking.
- Avoid breaking retry semantics for errors that happen before response headers are written.
- Distinguish this response-body streaming work from the existing request-body replay-to-disk feature.

### Open Questions

- What response size threshold should switch from memory buffer to streaming?
- Can usage be parsed from a bounded response tail or side channel?
- Should large response streaming be disabled for routes where retry fidelity matters more than memory?
- Should large response streaming be configurable globally, per route, or both?
