# Mind Library — owner file management & derivatives

The runtime side of OpenPaw's file/workspace system: a set of owner-facing
endpoints for browsing and managing a synth's workspace, resumable uploads, and
background-generated preview derivatives (image thumbnails, video posters, face
crops). A companion (proprietary) web UI drives these; the engine only exposes
the endpoints and does the work.

## Design rules

Every surface obeys the same invariants:

- **Owner-private** — file, upload and derivative responses are
  `Cache-Control: private, no-store` (on success and error); they must never be
  cached by an intermediary.
- **Background-only rendering** — a preview request never decodes/resizes/encodes
  on the request path. It serves a ready cached file, or returns `202` and a
  single bounded background worker renders it. A derivative is rendered **once**
  per (source content, generator version) and then served as a file.
- **No resurrection** — a derivative of a deleted / trashed source never lands on
  disk and never lingers; the commit is guarded so a delete always wins the race,
  and a boot GC reaps orphans. Liveness checks fail closed on error.

## Endpoints (owner-twin)

- **Files** — `/api/owner/files` and `…/{read,upload,mkdir,move,meta,download,trash,trash/restore}`,
  plus `…/changes?since_revision=` (an append-only activity journal with a
  monotone catalog revision) and `…/processing?media_id=`.
- **Upload (resumable, chunked)** — `POST /api/owner/files/uploads` (init,
  idempotent per client token), `PUT …/{id}` (chunk, with a per-chunk SHA-256
  header), `POST …/{id}/complete`, `GET …/{id}` (status), `GET …/uploads`
  (discovery), `DELETE …/{id}` (cancel). Uploads are staged durably and published
  atomically; a completed (already-published) upload cannot be cancelled — a
  cancel then returns `409` with the durable result.
- **Preview** — `GET /api/owner/files/preview?media_id=&variant=thumb`:
  `200` (ready, JPEG, a strong byte-digest `ETag`), `304` (If-None-Match),
  `202 preview_pending` + `Retry-After` (a background render was dispatched), or
  `404 no_preview` (a type or source that can't be rendered).

## Under the hood

- **File catalog** — tracks each file's ownership and provenance, a monotone
  revision, smart-collection filters, and an activity journal backed by a durable
  outbox (events are staged with the mutation and drained exactly-once).
- **Derivative worker** — one bounded background goroutine draining a ledger of
  pending derivatives. Each render holds a claim token, so a source that changes
  or is deleted mid-render drops the stale result. Kind = thumbnail | poster;
  output is JPEG (≤512px) — the runtime is pure-Go, no native image codecs.
- **Face crops (People)** — the same background-only, claim-guarded model applied
  to a face gallery: a bounded render pool, media/cluster-scoped caching, and a
  generation-aware garbage collector. A recognized face is perception, not
  authentication.

## Package map

| Area | Package / file |
|---|---|
| File catalog + activity journal | `owner_file_catalog.go` |
| Resumable upload state machine | `owner_uploads_v2.go` |
| Move / rename tool | `tools/workspace_move.go` |
| Derivative worker + ledger | `owner_derivatives.go`, `storage/derivatives.go` |
| Face derivative cache | `face_avatar_cache.go` |
