# PromptIter Regression Loop

This example runs a deterministic Evaluation + PromptIter loop without any API key.

The main path still goes through real `llmagent`, `runner`, `evaluation.AgentEvaluator`, and `evaluation/workflow/promptiter/engine`. Fake code only replaces the candidate `model.Model` and the PromptIter worker interfaces.

## Run

```bash
go run . -mode fake
```

Outputs:

- `output/optimization_report.json`
- `output/optimization_report.md`

Optional flags:

```bash
go run . \
  -mode fake \
  -data-dir ./data \
  -output-dir ./output \
  -prompt ./config/baseline_prompt.txt \
  -config ./config/promptiter.json
```

Trace smoke mode validates trace-mode evaluation compatibility without running optimization:

```bash
go run . -mode trace-smoke -output-dir ./output-trace
```

## What The Fake Loop Proves

- The baseline prompt is read from `config/baseline_prompt.txt`, hashed, and passed into `llmagent.WithInstruction`.
- Baseline evaluation uses a loyalty-profile tool declaration, so flight lookup cases fail deterministically.
- PromptIter receives deterministic failures, builds gradients through fake worker interfaces, and applies a real surface patch to `candidate#tool.lookup_record`.
- Candidate evaluation sees the patched tool declaration and the fake model changes inference behavior by calling `lookup_record`.
- `candidate.train` is produced by rerunning the train evalset against the final `AcceptedProfile`, not by reusing an arbitrary round train result.
- Validation delta reports per-case `new_pass`, `new_fail`, `improved`, `regressed`, `unchanged_pass`, and `unchanged_fail` classifications.
- The fake optimizer runs two accepted rounds: a partial delay patch, followed by an overfit patch that improves aggregate validation while regressing a critical no-tool case.
- Failure attribution reports failed candidate validation cases with mutually exclusive categories such as `wrong_tool_name`, `tool_not_called`, and `final_response_mismatch`.
- The final gate rejects publishing when validation improves but a new hard fail or critical-case regression appears.
- Deterministic metrics run with `RunRequest.Judge = nil`.

## Trace Smoke Boundary

Trace smoke uses `data/promptiter-regression-loop-app/trace_smoke.evalset.json` with `evalMode: "trace"`. The evaluation service replays recorded actual invocations and their execution traces, so runner inference is skipped. The report records `traceSmoke.optimizationSkipped=true` and the reason `trace mode replays actual output and cannot validate candidate inference`.

Trace smoke is useful for checking evalset compatibility, deterministic metrics, trace propagation, and failure attribution. It is not evidence that a prompt patch changes future candidate inference, so the fake mode remains the optimization acceptance path.
