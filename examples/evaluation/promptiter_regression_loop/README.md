# Phase 1 v2 PromptIter Regression Loop

This example is a single-round deterministic demo of the PromptIter optimization loop. It runs without API keys and proves that a tool description patch can travel through the real `llmagent -> runner.NewRunner -> evaluation.AgentEvaluator -> promptiter engine` path and change candidate inference.

Phase 1 v2 intentionally does not implement multi-round refinement, validation delta reporting, final gate decisions, failure attribution, trace smoke checks, candidate train regression, or a design doc. Those remain Phase 2+ work.

## Run

From `examples/evaluation`:

```bash
go test ./promptiter_regression_loop
go run ./promptiter_regression_loop -mode fake
```

From this directory:

```bash
go test .
go run . -mode fake
```

Only `-mode fake` is supported. Any other mode returns an explicit error. The demo also accepts:

```bash
-data-dir ./data
-output-dir ./output
-prompt-path ./config/baseline_prompt.txt
-config-path ./config/promptiter.json
```

The baseline instruction is deliberately neutral:

```text
You are a helpful assistant.
```

The initial `lookup_record` tool description only mentions a traveler loyalty profile. The fake model reads only user messages, tool descriptions, and tool results. It does not read evalset expected invocations, expected final responses, case IDs, or test state.
