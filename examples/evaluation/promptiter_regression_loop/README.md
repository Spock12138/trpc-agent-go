# PromptIter Regression Loop

This Phase 3 example runs a deterministic Evaluation + PromptIter loop without any API key.

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

## What Phase 3 Proves

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

## Remaining Boundary

This example intentionally keeps trace smoke and the final design document for later phases. Those remaining items are recorded in `phase1Pending` in the report.
