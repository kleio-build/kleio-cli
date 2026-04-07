# Agent instructions (Kleio)

This repository uses Kleio for durable engineering signals.

When the Kleio MCP server is enabled, follow these rules in addition to `.cursor/rules/kleio-mcp.mdc` (authoritative for tool usage and field details).

## Core rule

Do not let settled engineering intent disappear.

A task plan is not a Kleio record.

If you make or adopt a non-trivial decision, or identify durable follow-up work, you must log it in Kleio before moving on.

## Required workflow

For any non-trivial task, follow this sequence:

1. Understand the task
2. Make a plan
3. Log any settled decision with `kleio_decide` if a direction has been chosen
4. Log any durable follow-up with `kleio_capture`, or (for implementation-slice summaries) `kleio_checkpoint` when appropriate
5. Implement
6. Before finishing, verify that required Kleio records were created

Do not treat the plan itself as a substitute for a decision log.

## What counts as non-trivial

A change is non-trivial if it does any of the following:

- changes schema, model, or persistence behavior
- changes API shape or request/response semantics
- changes generation strategy, plugin usage, or SSOT structure
- changes cross-file control flow or architecture
- introduces or resolves a meaningful technical tradeoff
- identifies follow-up work that would still matter later
- creates a smell, risk, or unresolved concern worth revisiting

## Signal types (keep the distinction sharp)

| Kind | Tool | Meaning |
|------|------|--------|
| **Decision** | `kleio_decide` | What we **chose** and why. Uses relational path with structured `alternatives`, `rationale`, `confidence` fields. |
| **Checkpoint** | `kleio_checkpoint` | What was **implemented** in this work slice (provenance, validation, optional handoff). Uses `POST /api/captures` with a nested checkpoint—not the smart capture path. |
| **Work item** | `kleio_capture` | **Follow-up** work to schedule (smart/backlog synthesis). Only `signal_type=work_item` creates backlog items. |
| **Observation** | `kleio_capture` with `signal_type=observation` | Non-actionable signal (pattern, smell, hypothesis). Stored as a capture without backlog synthesis. |

Do not use `signal_type: checkpoint` or `signal_type: decision` with `kleio_capture`; the server and CLI reject them. Use `kleio_checkpoint` / `kleio_decide` instead.

## When to use each tool

### `kleio_decide`
Use when a direction is actually chosen.

Trigger when:
- a planning spike ends with a selected approach
- alternatives were compared and one was chosen
- a design thread concludes with "we're going with X"
- implementation is about to proceed on a meaningful architectural or structural choice

Minimum payload:
- `content`
- `alternatives`
- `rationale` (required)
- `confidence` (required: `low`, `medium`, or `high`)

Add `repo_name` and `file_path` when known.

### `kleio_capture`
Use for actionable follow-up work (smart capture / backlog path):
- bugs
- refactors
- feature gaps
- debt introduced or discovered during implementation

Only `signal_type=work_item` (default) creates or links backlog items. Use `signal_type=observation` for non-actionable signals that are stored without backlog synthesis.

This is **not** for relational checkpoints or decisions.

### `kleio_checkpoint`
Use when a **meaningful slice of implementation** is complete and worth recording as provenance: what changed, validation status, optional files/caveats/deferred.

**Do not** use for: trivial edits only; intermediate steps inside the same slice; purely speculative unimplemented work; items that are really backlog follow-ups (use `kleio_capture`) or decisions (use `kleio_decide`).

Prefer **structured** fields (`slice_category`, `slice_status`, `validation_status`, `summary_what_changed`, …) over dumping long prose into `content`.

### `kleio_session_summary`
Call at natural breakpoints to check your logging behavior for the current session. Returns tool call tallies, captures logged, and nudges if decisions or checkpoints are missing.

### `kleio_backlog_list` / `kleio_backlog_show`
Use before creating a new durable work item if there is a meaningful chance it already exists.

### `kleio_backlog_prioritize`
Only change urgency, importance, or status with clear user intent, explicit session intent, or obvious triage context. Urgency = how time-sensitive; importance = how much it affects project goals.

## Threshold

Default to logging when the information would help a future engineer understand:
- why a direction was chosen
- what follow-up work was discovered
- what risk or concern remains

If unsure between logging and skipping, prefer logging concisely.

## End-of-task check

Before finishing a non-trivial task, verify:

- Did I choose a meaningful direction? If yes, was it logged with `kleio_decide`?
- Did I discover durable follow-up work? If yes, was it logged with `kleio_capture`? Did I record an implementation checkpoint with `kleio_checkpoint` when a slice merited it?
- Did I check whether the work already existed if duplication was plausible?

If not, do it before returning control.
