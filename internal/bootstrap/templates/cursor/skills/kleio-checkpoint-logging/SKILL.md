---
name: kleio-checkpoint-logging
description: Record implementation-slice checkpoints via MCP `kleio_checkpoint` (relational POST /api/captures). Use when a meaningful slice of work is complete—not for backlog items or weak observations.
---

# Kleio checkpoint logging

## What a checkpoint is

A **checkpoint** is durable **implementation provenance** for a slice of work: what changed, how it was validated, and optional handoff. It is **not** a decision log and **not** a backlog work item.

## Which tool

- **`kleio_checkpoint`** — relational checkpoint (`POST /api/captures` with nested `checkpoint`). Use this.
- **`kleio_capture`** — smart/backlog path only. **Never** use `signal_type: checkpoint` there; it is rejected.

Structured fields (required): `slice_category`, `slice_status`, `validation_status`, and either `summary_what_changed` or a substantive `content` line (summary defaults to content). Prefer **short, structured** fields over long prose dumps.

## When to use

- Meaningful completion of a non-trivial slice, handoff, or boundary worth revisiting later.
- When the user explicitly asks to record a checkpoint.

## When not to use

- Trivial edits only.
- Intermediate steps inside the same slice (do not checkpoint every commit).
- Speculative or unimplemented work (use `kleio_capture` with `signal_type=observation` or skip).
- Actionable follow-up work (use `kleio_capture` / smart path).
- Settled engineering choices among options (use `kleio_decide`).

## Canonical contract

JSON field names and validation caps match the API and server (`CaptureCheckpointWrite` / `capture_metadata`). See the Kleio app docs: `docs/api/CAPTURE_INGESTION.md` and `docs/api/schemas/capture-create-checkpoint.request.schema.json` in the kleio-app repository.
