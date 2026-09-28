# Conventions (operator)

These conventions apply to anyone editing `operator/`, not only agents. They are listed explicitly here to assist agents where they struggle.

## Scope of a Change

- Make the smallest change that solves the problem. Do not rename symbols, refactor nearby code, or reformat lines the change does not touch.
- Keep one concern per change. API changes, Helm chart changes, sample updates, and documentation usually belong in separate changes.
- A cleanup or a simplification must produce a net line reduction, unless a few added lines substantially improve readability.
- Do not run a git command that changes state, such as `checkout`, `commit`, `rebase`, `reset`, or `stash`, unless the request asks for it.

## Changelog

- Add an entry to [CHANGELOG.md](./CHANGELOG.md) under the `Unreleased` heading for every change a user can observe: a new or a renamed spec field, a changed default, a change to a rendered object, a deprecation, or a bug fix.
- Add no entry for a test-only change, a refactor, a comment, or a documentation change.
- Use only the `Added`, `Changed`, `Deprecated`, and `Removed` subsections. Add a subsection only when an entry needs it. Create no other subsection.
- Keep each entry to one short line, and name the affected field or object in backticks. Use a sub-bullet only for a caveat the user must act on, such as a manual step or an upgrade order. Match the entries already in the file.
- A changelog entry is not documentation and should not cover every aspect of a change. It records what changed, not how the feature works.
- Put longer documentation in a file under `../docs/` and link to that file from the entry, as the `spec.auth.profileRef` entries do.

## File Style

- Every `.go` file needs a `Copyright ... NVIDIA CORPORATION. All rights reserved.` line within its first 10 lines, or `// no-copyright`. Copy the header from a neighboring file.
- An `import` block may hold at most one blank line.

## Comments

Write a comment only to state intent or a constraint the code cannot show. Never narrate what the code does.

Doc comments (package, type, function, method) describe only that entity's own responsibility:

- Never describe internals, precedence/fallback order, or what the code inside them does — that detail changes and the comment goes stale.
- Never make claims about callers, deployment, external tooling, or where a value is consumed later.
- Never justify a design or a change, and never reference obsolete or unimplemented behavior.

Comment specific confusing code inline, immediately above the line it explains, not in the doc comment. Keep each comment to the shortest form that still carries the constraint, and match the surrounding file's comment density and style.

```go
// BAD: describes internals, fallbacks, and callers
// Resolve determines the domain and identity, logging each value and its source. Neither failure is
// fatal: the domain falls back to the default, and callers that never need the identity keep running.

// GOOD: states its own responsibility and contract
// Resolve determines the cluster domain and the operator's own identity. It reports failures through
// the logger rather than to the caller.
```

Write comments in [ASD-STE100 Simplified Technical English](https://www.asd-ste100.org/):

- Use the active voice. Use simple present, simple past, or simple future tense.
- Write one instruction per sentence. Keep instruction sentences to 20 words or fewer and descriptive sentences to 25 words or fewer.
- Use each approved word in one part of speech and one meaning only. Use the same word for the same thing every time.
- Do not use noun clusters of more than three words.
- Do not omit articles or other words that make the sentence clear.

## Naming

- Name a thing by what it is. `daemon` is common but ambiguous in this project: be more precise and write `pod`, `container`, `process`, `AIS node`, or `StatefulSet`.
- A function name must match what the function does. `refreshToken` is the wrong name for a function that does not call the token endpoint.

## API Types and Validation

- Put a single-field constraint in a kubebuilder marker on the type. This includes an `XValidation` CEL rule for a constraint between fields of the same struct.
- Put a cross-resource rule, an update-only rule, or a rule that needs an API call in the webhook. Only the webhook sees the previous object.
- The operator owns every default. A Helm chart, a sample, or another caller passes through only the value the user sets.
- Write the field documentation in the Go doc comment on the field. `make manifests` copies it into the CRD, which is the primary reference for users.
- A webhook must accept a write that does not change the spec. The operator patches finalizers, annotations, and status on objects whose spec no longer validates.

## Deprecation and Compatibility

- Keep an existing spec field readable. Mark it `// Deprecated: use <new field>.` and keep the value working for a cluster that already sets it.
- Reject a deprecated field on create and on a spec update only in a major release. State that rejection in the deprecation comment.
- A read of a deprecated field triggers `staticcheck` SA1019. Add `//nolint:staticcheck` with a short reason at the read. When a file exists only to read deprecated fields, put one directive at the top of the file instead.
- A new option needs no version note. Give a version only when the option replaces a deprecated or a removed one.
- A change that only adds a field must not change the rendered output for a spec that omits the field.

## Reconcile Loop Efficiency

- The loop runs on every event for a watched object and again after each `RequeueAfter`. Weigh the cost of work on a path that every pass reaches.
- Work that leaves the process costs the most: an AIS API call, a token exchange, an authorization review, a certificate parse, or a file read. Cache that result.
- Key a cache on a value that changes when its input changes, so the entry invalidates itself.
- Do not assume a client read is free. Check which types the manager caches, and keep each informer no wider than the controller needs.

## Logging

- Prefer `V(1)`. Log at `V(0)` only a state transition or a message the operator must show, such as a security warning.

## Tests

- Put a test beside the file that holds the function it covers. When a refactor moves the function, move the test.
- Use `<file>_internal_test.go` with the package under test only when the test needs an unexported symbol. Otherwise use `package <name>_test`.
- Unexport a symbol that only a test uses from outside its package, and move that test to an internal test.
- A test asserts the contract the current code implements. Do not keep a test that locks in previous behavior.
- Prefer one table-driven test with subtests over several tests that run the same action and assert different fields.
- Assert against the same exported constant the code uses, not a copy of its value.
- Do not assert log output.
- After each change, remove the tests the change made duplicate or unreachable, without losing coverage.
