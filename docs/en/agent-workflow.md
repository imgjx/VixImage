# Self-Loop Agent Workflow

Give it a prompt; a multimodal LLM enters an `observe → decide → act` loop. Each turn emits one `{think, tools[]}`, the executor strictly parses and runs a batch of **image-related** skills, and a native Go render engine composes the final image. There is an explicit completion signal (`finish`) and the Agent can repeatedly view its own work (`view`) before revising.

## Core Features

- **One LLM request = one step**: each turn emits a short `think` plus a batch of `tools`; state accumulates across turns so the Agent can iterate (draft → view → adjust → finish).
- **Image-only skills**: `canvas`, `gen`, `stock`, `qrcode`, `qrdecode`, `add/set/del/clear`, `layers`, `copy`, `undo`, `view`, `finish`.
- **Bottom tools**: `view` and `finish` must be the last item in `tools`, at most one per turn.
- **Completion signal**: only `finish` delivers the final image.
- **System-enforced view**: when the model emits `finish`, the system intercepts and force-renders the image about to be delivered back into the next turn with a self-check prompt; the model must confirm with another `finish` or fix flaws first. A cap (`MaxForceReviews`) prevents loops.
- **Proactive creative composition**: the system prompt teaches the Agent to turn reference images into objects inside a scene (slanted screens, frames, shop windows, picture-in-picture); `gen{ref}` for image-to-image, `img` with `persp`/`shear` for perspective fit.
- **Hardcoded few-shot + cache savings**: the system prompt and learning data are compile-time constants forming a stable system prefix that hits upstream prompt caches; dynamic content goes into user messages. Toggle per Agent via `learning`.
- **Manageable intermediate assets**: `copy{src,as}` physically duplicates an asset; `undo{n}` rolls back the last n edits (layout only, keeps materials).
- **Context limit**: each LLM can set `max_tokens` (output cap) and `context_limit` (input character budget; trims older messages, keeps images and the latest request).

## Step Model & Exhaustion

- Each LLM request is **1 step**; each image generation costs the engine's configured steps (default 30); editing / viewing / finishing are free.
- Total budget is the Agent's `max_steps` (default 100); it need not be exhausted.
- Each model configures its own exhaustion behavior: `fail` (default) or `partial` (return current draft).
- The DB records `steps_used` (actual consumption) and `llm_calls` (LLM request count) separately.

## Fault Tolerance

- **Tolerant but never fabricating**: LLM throttling + exponential backoff on 429; on JSON parse / illegal tool errors, the error is fed back for a same-turn retry (capped, no step advance); out-of-range `set/del` indices are skipped with a warning.
- **Parse tolerance ≠ operation fallback**: notation differences (RGB arrays, tools as keys, wrapped params, trailing commas) are normalized by the parse layer; unknown tools / layers, bottom-rule violations, out-of-range refs, generation / render failures remain hard errors.
- **Turn pacing rule**: first turn builds only the canvas; each later turn has ≤2 actions; text merged into a single layer to prevent output truncation.

## Context Assembly Order

System prompt → compact context (remaining steps + image model list) → asset table → previous result (only gen/view outputs images) → user request. Only image generation and viewing produce output images; edit turns return a short "edited OK" status.

## Dashboard & Trace

- Paginated call overview (`page` / `size`); detail shows per-step thinking and operations.
- API parameter `trace: true` appends short per-step ThinkFlags (step + think only, no skills); off by default.
