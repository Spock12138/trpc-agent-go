# Phase 1 v2 计划：Deterministic PromptIter Single-Round 主闭环

## Summary

在 `feat/eval-optimization-loop-workspace-v2` 上新增 `examples/evaluation/promptiter_regression_loop/**`，实现无 API Key 的真实 PromptIter 最小闭环：baseline train/validation evaluation -> 1 个 PromptIter round -> fake optimizer 生成 tool description patch -> candidate validation -> Phase 1 report。

Phase 1 v2 明确是 single-round 演示，目标是证明 patch 会经真实链路影响 candidate inference；多轮 refinement、delta、final gate、failure attribution、trace smoke 留到 Phase 2+。本阶段严格只改 example 目录，不修改 `evaluation/**`、`evaluation/workflow/promptiter/**`、`runner/**`、`agent/**`、`internal/**`、`go.mod`、`go.sum`。

## Key Changes

- 新建 `main.go`、`pipeline.go`、`fake.go`、`report.go`、`pipeline_test.go`、`README.md`、`config/`、`data/`、`output/`；不提交 `DESIGN.md`。
- CLI 只支持 `-mode fake`，默认 fake；其它 mode 返回明确错误。支持 `-data-dir`、`-output-dir`、`-prompt-path`、`-config-path`。
- `config/baseline_prompt.txt` 内容固定为 `You are a helpful assistant.`，不包含航班、记录查询、工具选择等领域暗示，确保 baseline fallback 完全由初始 tool description 驱动。
- 真实链路必须经过 `llmagent -> runner.NewRunner -> evaluation.AgentEvaluator -> promptiter engine`；fake 只替代 `model.Model`、`backwarder.Backwarder`、`aggregator.Aggregator`、`optimizer.Optimizer`。
- `RunRequest.InitialProfile = nil`、`Judge = nil`、`MaxRounds = 1`、`TargetSurfaceIDs = ["candidate#tool.lookup_record"]`；运行前调用 `engine.Describe(ctx)` 校验 target surface。
- 代码注释标明 `baseline.train = result.Rounds[0].Train` 依赖当前 engine 语义：第 1 轮 `InputProfile` 来自 normalized initial profile，而 Phase 1 设置 `InitialProfile = nil`。
- report 字段与完整计划对齐：`candidate.train = null`，并保留 `phase1Pending = ["final_gate","validation_delta","failure_attribution","trace_smoke","candidate_train_regression","design_doc"]`。

## Deterministic Behavior

- 初始 tool description 为 `Look up a traveler loyalty-profile record.`，不包含 flight/status/delay/gate 语义；lookup 类航班问题 baseline 输出固定 fallback。
- fake model 只读取 `model.Request.Messages` 中的用户文本、tool description、tool result；严禁读取 evalset expected invocation、expected final response、case ID 或测试内部状态来反推行为。
- user intent 只从用户文本关键词提取：`status/cancelled/operating` -> status，`delay/delayed` -> delay，`gate` -> gate，`departure/depart` -> departure；无 record ID 或无可识别 intent 时不调用工具。
- fake optimizer 第 1 轮返回合法 tool patch：保留 tool ID，仅更新 description 为 `Use lookup_record to query flight status, delay, departure time, and gate information.`。
- fake aggregator 使用当前 upstream API：返回 `promptiter.AggregatedSurfaceGradient{SurfaceID, NodeID, Type, Gradients: []promptiter.SurfaceGradient{...}}`。
- fake backwarder 对 nil request / 空 allowed surfaces / 非目标 surface 返回空 result；对目标 surface 返回 `LossSeverityP1` gradient，并回填 `EvalSetID`、`EvalCaseID`、`StepID`。
- `metrics.json` 只包含 deterministic metrics：`tool_trajectory_avg_score` exact name/arguments/result，`final_response_avg_score` with `criterion.finalResponse.text.matchStrategy = "exact"`。

## Test Plan

- `go test ./examples/evaluation/promptiter_regression_loop`
- 端到端：无 API Key、nil judge 跑完整 single-round pipeline，至少 1 个 accepted round，candidate validation 分数高于 baseline validation，并生成 JSON/MD report。
- Prompt 读取：临时 prompt 文件 hash 改变，fake model 观察到 instruction，且默认 prompt 不含航班领域暗示。
- Patch 生效：fake model 先看到 loyalty description，再看到 flight/status/delay/gate description。
- Intent 约束：fake model 测试直接构造 request，证明 intent 只从用户消息关键词提取，不依赖 evalset expected 字段。
- Worker API：覆盖 backwarder 空输入/非目标 surface/失败 loss，aggregator 当前 `Gradients []SurfaceGradient` 形状，optimizer patch 经完整 engine run 成功应用。
- Report schema：断言 `baseline.train`、`baseline.validation`、`candidate.validation`、`candidate.train:null`、`rounds[*].train`、`rounds[*].validation`、`phase1Pending` 存在，且无 `trainScore` / `validationScore`。
- 收尾执行 `git diff --name-only`，本阶段实现变更必须只落在 `examples/evaluation/promptiter_regression_loop/**`。

## Assumptions

- Phase 1 v2 是阶段性 PR，不声明关闭完整 issue。
- 旧分支 `eval-optimization-loop` 只能作为 donor/reference；可以选择性复用 example 层代码和数据，不能引入旧的 core 改动依赖。
- 不使用 `ProfileEvaluator`、`EvaluateWithProfile`、Actual/Expected invocation 透传或任何新增 core API。
- README 必须明确：Phase 1 是 single-round deterministic demo，多轮优化和发布决策在 Phase 2+ 补齐。
