# Phase 4: Trace Smoke 与文档收口

## Summary
Phase 4 补齐当前 `phase1Pending` 剩余两项：`trace_smoke` 和 `design_doc`。实现保持示例边界：不修改 PromptIter / evaluation 公共接口，不引入真实 API key，不改变 fake 主闭环；新增 `trace-smoke` 只验证 trace-mode evalset 能经由真实 `AgentEvaluator` 与 PromptIter engine 的 evaluation adapter 被评估、汇总和归因，并明确跳过优化。

## Key Changes
- 新增 `-mode trace-smoke`，保留 `-mode fake` 默认行为不变。入口改为模式分发：fake 继续跑完整 PromptIter 优化闭环；trace-smoke 只跑 trace eval，不调用 optimizer round。
- 新增 `data/promptiter-regression-loop-app/trace_smoke.evalset.json`，case 使用 `evalMode: "trace"`，包含 `actualConversation`、expected `conversation`、tool invocation、final response，以及可被 engine 校验的 `executionTrace`。
- trace-smoke 通过现有 `llmagent -> runner -> evaluation.AgentEvaluator -> promptiter engine EvaluateWithProfile` 路径获取 `EvaluationResult`，复用现有 summary 与 failure attribution 逻辑；不走 `engine.Run`，不应用 patch，不生成 candidate profile。
- `OptimizationReport` 增加 `traceSmoke` 字段，结构固定为：`enabled`、`evalSetId`、`optimizationSkipped`、`optimizationSkippedReason`、`evaluation`、`attribution`。reason 固定为 `trace mode replays actual output and cannot validate candidate inference`。
- fake 报告继续输出完整 baseline/candidate/rounds/delta/gate；trace-smoke 报告输出同名 `optimization_report.json/.md`，其中 traceSmoke 明确为 enabled/skipped，优化相关字段保持空或零值，不伪造 candidate/gate 结论。
- `phase1Pending` 在 Phase 4 完成后移除或为空；README 更新为最终示例说明，新增 `DESIGN.md` 300-500 字，覆盖 PromptIter 接入、failure attribution、PromptIter acceptance 与 final gate 分工、防过拟合策略、trace smoke 边界、审计报告可追溯性。
- 刷新示例输出：先保留 `go run . -mode fake` 的当前 overfit-reject 报告作为主 golden；如仓库只接受一组 output，则默认提交 fake 报告，并在 README 说明 trace-smoke 可用 `-output-dir` 单独生成。

## Test Plan
- 更新现有 unsupported mode 测试为 `trace-smoke` 成功路径，断言 runner inference 不作为优化证据、`traceSmoke.optimizationSkipped=true`、skip reason 精确匹配。
- 新增 trace-smoke integration test：使用 `t.TempDir()` 输出，断言 evaluation 可运行、trace invocation 被透传到 attribution、失败 case 能产生分类与 metric reason。
- 更新 fake pipeline test：断言 `phase1Pending` 不再包含 `trace_smoke` / `design_doc`，fake 主路径分数、round acceptance、final gate reject、critical regression 仍保持 Phase 3 行为。
- 新增/更新 report schema test：JSON 顶层包含 `traceSmoke`；fake mode 中 `traceSmoke.enabled=false` 或为空策略固定一致；trace-smoke mode 中优化字段不被误标为可发布。
- 验证命令：`go test ./workflow/promptiter/engine`、`go test .`，并手动刷新 `go run . -mode fake`；可选验证 `go run . -mode trace-smoke -output-dir <temp-or-output-trace>`，总耗时要求小于 3 分钟。

## Assumptions
- Trace smoke 是兼容性 smoke test，不参与 PromptIter 优化主路径，也不证明 prompt patch 会影响下一轮 inference。
- 不改 `Engine` interface，不改核心 evaluation / runner / agent 公共 API。
- fake model 与 fake PromptIter workers 仍只用于无 API key 的 deterministic 验收。
- 若只能提交一份默认 output，默认提交 fake 主闭环报告；trace-smoke 输出由测试和 README 命令覆盖。
