# Code Commenting Guidelines

## Purpose

Comments should preserve important context that code alone cannot reliably express.

Use comments to explain intent, constraints, invariants, trade-offs, and non-obvious behavior.

Comments must not replace clear naming, small functions, tests, validation, types, tooling, or architectural boundaries.

## Core Rule

Write comments for:

- why the code exists
- what must remain true
- what must not be changed
- what assumption the code depends on
- what external behavior affects the code
- why an obvious alternative was not used

Do not write comments that merely describe what the code does.

## When Comments Are Required

Add comments when code involves:

- domain or business invariants
- security-sensitive behavior
- external system quirks
- workarounds or compatibility constraints
- architectural boundaries that tooling cannot fully enforce
- concurrency, ordering, idempotency, or lifecycle assumptions
- performance or resource-sensitive decisions
- intentional non-obvious design choices
- public API, interface, plugin, or extension contracts

## Comments to Avoid

Avoid comments that:

- restate the code
- explain obvious control flow
- compensate for unclear naming
- are vague, such as “handle edge cases”
- are likely to become stale after small changes
- duplicate information already expressed by types, tests, schemas, or validation
- act as a substitute for proper design or enforcement

## TODO, FIXME, and HACK

Use `TODO`, `FIXME`, and `HACK` comments only when they are specific and actionable.

Required format:

```text
TODO(scope): specific action and reason
FIXME(scope): specific bug or risk
HACK(scope): why this workaround exists and when it can be removed
````

Do not write vague notes such as:

```text
TODO: improve this
FIXME: fix later
HACK: temporary
```

## Quality Check

Before adding a comment, verify:

1. The comment explains intent, constraint, invariant, risk, or non-obvious behavior.
2. The comment is close to the relevant code.
3. The comment is unlikely to become false after a small refactor.
4. A future maintainer or AI agent could make a wrong change without this comment.
5. The same information cannot be expressed better through naming, structure, tests, validation, types, schemas, lint rules, or build-time checks.

Only add the comment if it passes this check.

## Style

Comments should be:

* specific
* concise
* actionable
* located near the relevant code
* consistent with the codebase language and comment style

If the explanation is long, move the full rationale to architecture documentation and leave only a short pointer near the code.
