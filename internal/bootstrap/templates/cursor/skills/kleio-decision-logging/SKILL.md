---
name: kleio-decision-logging
description: Log settled engineering decisions to Kleio via MCP using `kleio_decide` before implementation continues. Use when a non-trivial direction has been chosen.
---

# Kleio decision logging

## Core rule

A plan is not a decision log.

Use `kleio_decide` once a non-trivial engineering direction is chosen, before moving on with implementation.

## How it works

`kleio_decide` uses the **relational capture path** (`POST /api/captures` with a nested `decision` object). Decisions are stored as first-class relational data with structured, queryable columns — not JSON blobs in `structured_data`.

## Trigger conditions

Call `kleio_decide` when any of the following is true:

- a planning spike or discovery slice ends with a chosen direction
- alternatives were compared and one option was selected
- a design thread concludes with a real decision
- implementation is about to proceed on a meaningful architectural, schema, API, persistence, or generation choice

Do not wait for a perfect "closure moment" if the implementation is already proceeding based on a chosen direction.

## Do not skip for these categories

Log decisions involving:
- architecture or boundaries
- schema or persistence strategy
- API shape
- generation / plugin strategy
- auth or data flow strategy
- deployment or environment strategy
- meaningful tradeoffs that would be costly to unwind

## Skip only when

Skip only for clearly trivial, reversible, local tweaks unless the user asks to capture them.

## Required payload

Send:

- `content`: one sentence stating the chosen direction
- `alternatives`: serious options considered
- `rationale`: why this option fits the codebase and constraints (required)
- `confidence`: `low`, `medium`, or `high` (required)

Include `repo_name` and `file_path` when known.

## Reminder

If implementation would be confusing later without understanding why a choice was made, log it.
