# Phase 1: 最小可运行 PromptIter 回归闭环

## Summary

新增 `examples/evaluation/promptiter_regression_loop/`，实现 fake mode 下的最小可运行 Evaluation + PromptIter 闭环。该阶段只证明真实 PromptIter engine 能基于失败评测生成 patch、重新验证并生成最小审计报告；不改 `agent`、`runner`、`evaluation`、`evaluation/workflow/promptiter` 的公共 API。

## Key Changes

- 新增示例 CLI：`go run . -mode fake`，默认读取 `config/promptiter.json`、`config/baseline_prompt.txt`、`data/.../{train,validation,metrics}.json`，输出 `output/optimization_report.json` 和 `.md`。
- 组装现有链路：`runner.NewRunner(candidate)` + `evaluation.New(...)` + `promptiterengine.New(...)`；deterministic metrics 不使用 LLM judge，`RunRequest.Judge` 保持 `nil`。
- fake candidate agent 实现 `agent.Agent` 与 `structure.Exporter`：
  - 固定 node ID 为 `candidate`；
  - `StructureID` 固定为 `promptiter-regression-loop-fake`;
  - `Snapshot` 至少包含 candidate node、`candidate#instruction`、`candidate#tool.lookup_record`。
- fake agent 通过 `internal/surfacepatch.PatchForNode(invocation.RunOptions.CustomAgentConfigs, "candidate")` 读取 PromptIter 编译出的 runtime patch；不新增公共 getter API。
- `RunRequest.InitialProfile = nil`，baseline 使用 fake agent 原始 surface：误导性的 loyalty-profile tool description。
- fake behavior：
  - baseline 下 flight 查询不调用工具，返回固定 fallback，评测失败；
  - patch 后若 tool declaration 描述包含 flight/status/delay/gate 语义，则 emit tool call、tool result、final response；
  - direct no-tool case 不受 patch 影响，始终通过。
- 事件流必须符合 evaluation 捕获逻辑：
  - tool call response：assistant message 带 `ToolCalls`；
  - tool result response：tool message 带 `ToolID`、`ToolName`、JSON content；
  - final response：assistant message，`Response.Done=true`。
- fake agent 调用 `agent.StartExecutionTraceStep`、`agent.SetExecutionTraceStepAppliedSurfaceIDs`、`agent.FinishExecutionTraceStep`；tracing 由 PromptIter 的 profile compile 自动打开。
- fake workers 直接实现 `backwarder.Backwarder`、`aggregator.Aggregator`、`optimizer.Optimizer`：
  - backwarder 对失败 case 产出指向 target surface 的固定合法 gradient；
  - aggregator 合并/去重同 surface gradients；
  - optimizer 第 1 轮返回通用 flight lookup tool description patch。
- Phase 1 报告只包含最小审计字段：mode、seed、target surfaces、baseline train/validation score、candidate validation score、rounds、patches、PromptIter acceptance、cost USD=0、model call count、latency ms；`gateDecision` 使用 `"phase1-not-implemented"`，`attributionSummary` 使用空集合或 `"phase1-not-implemented"`。

## Test Plan

- 集成测试跑完整 fake pipeline，断言 PromptIter 至少执行 1 round，round 1 patch 被接受，accepted profile 包含 `candidate#tool.lookup_record`。
- 单测 fake agent patch 读取：给 `CustomAgentConfigs` 注入 tool declaration patch 后，agent 会调用 `lookup_record`。
- 单测事件捕获：tool call、tool result、final response 能被 evaluation 转成 expected `evalset.Invocation`。
- 单测报告生成：JSON/MD 文件存在，关键字段和 phase-1 placeholder 值稳定。
- 手动验收：`go test ./examples/evaluation/promptiter_regression_loop` 与 `go run . -mode fake` 均无需 API Key，耗时远低于 3 分钟。

## Assumptions

- Phase 1 不实现 final gate、逐 case delta、复杂 attribution、过拟合拒绝；这些留给 Phase 2/3。
- 保留 `config/` 与 `data/` 分离，以匹配当前目标 layout：配置/prompt 放 `config/`，evalset/metrics 放 `data/`。
- 示例可以导入本 module 的 `internal/surfacepatch`，因此不需要修改核心包或新增公共 API。
