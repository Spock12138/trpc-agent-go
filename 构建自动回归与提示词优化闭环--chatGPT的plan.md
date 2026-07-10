# Evaluation + Optimization 自动回归与提示词优化闭环计划（完整验收版）

## Summary

本计划新增 `examples/evaluation/promptiter_regression_loop/`，复用现有 `llmagent -> runner -> evaluation -> evaluation/workflow/promptiter` 路径实现一个可复现的 Evaluation + Optimization pipeline。实现边界保持为示例级交付：不改公共 API、不改核心包、不引入真实 API Key 作为验收前置条件。

主验收路径使用 deterministic fake model 和 deterministic metrics。fake model 仍通过真实 `llmagent`、真实 `runner.NewRunner(...)`、真实 `evaluation.AgentEvaluator` 和真实 PromptIter engine 执行；PromptIter 产出的 patch 必须经由现有 `profilecompiler -> agent.WithSurfacePatchForNode -> llmagent` 路径应用到 candidate agent 的 tool declaration / instruction surface，而不是在示例里直接 import `internal/surfacepatch`。

trace mode 不进入优化闭环主路径。trace evalset 会复放既有 actual invocation / trace，跳过 candidate inference，因此无法证明 prompt patch 改动影响了下一轮实际输出。为满足 issue 的“支持 trace mode”要求，本计划提供独立 trace smoke：验证 trace mode 的 evaluation、report 输入适配、failure attribution 兼容可运行，并在报告中明确标记 `optimizationSkippedReason: "trace mode replays actual output and cannot validate candidate inference"`。

如果按阶段提交，Phase 1 只能作为阶段性 PR，不声明关闭 issue；完整验收版必须包含 delta、final gate、failure attribution、审计报告和相应测试，不能留下 `phase1-not-implemented` 作为最终字段。

## Target Layout

```text
examples/evaluation/promptiter_regression_loop/
  main.go
  pipeline.go
  fake.go
  analysis.go
  report.go
  pipeline_test.go
  config/
    baseline_prompt.txt
    promptiter.json
  data/
    promptiter-regression-loop-app/
      train.evalset.json
      validation.evalset.json
      metrics.json
      trace_smoke.evalset.json
  output/
    optimization_report.json
    optimization_report.md
  README.md
  DESIGN.md
```

职责划分：

- `main.go`：CLI flags、入口、错误处理。默认 `-mode fake`，可选 `-mode trace-smoke`。
- `pipeline.go`：组装 fake model、llmagent、runner、evaluator、PromptIter engine，执行 baseline、optimization、accepted-profile regression、report。
- `fake.go`：deterministic fake `model.Model`、fake lookup tool、fake backwarder / aggregator / optimizer。
- `analysis.go`：case delta、final gate、failure attribution、cost / latency summary。
- `report.go`：`optimization_report.json/.md` 结构和落盘。
- `pipeline_test.go`：覆盖 fake pipeline、trace smoke、delta、gate、attribution、report、failed metric reason。
- `README.md`：运行命令、输入输出、fake / trace mode 说明。
- `DESIGN.md`：300-500 字设计说明，解释失败归因、接受策略、防过拟合策略、PromptIter 接入方式和审计方式。

目录约定：

- `data/promptiter-regression-loop-app/` 贴合现有 local evalset manager 的默认约定：`data/<appName>/<evalSetID>.evalset.json`。
- `metrics.json` 为 train / validation 共享 deterministic metrics，不依赖 LLM judge。
- `output/` 提交一份稳定 fake run 的 golden 示例输出；测试必须写入 `t.TempDir()` 或显式临时 `-output-dir`，不覆盖仓库内 golden report。

## Config

`config/promptiter.json` 示例：

```json
{
  "targetSurfaceIDs": ["candidate#tool.lookup_record"],
  "maxRounds": 2,
  "acceptancePolicy": {
    "minScoreGain": 0.1
  },
  "stopPolicy": {
    "maxRoundsWithoutAcceptance": 1
  },
  "finalGate": {
    "minValidationGain": 0.05,
    "maxDurationMs": 180000,
    "criticalCaseIDs": ["TR789"],
    "rejectOnNewHardFail": true,
    "rejectOnCriticalRegression": true
  }
}
```

默认 target surface 为 `candidate#tool.lookup_record`。后续可补 instruction surface 配置，但首个完整验收版本以 tool description surface 为主，保证 patch 路径最短且可审计。

## Report Semantics

报告字段必须写死，避免 rejected patch 污染 candidate 语义：

- `baseline.validation`：来自 PromptIter `RunResult.BaselineValidation`。
- `baseline.train`：来自 round 1 的 `Train`。round 1 的 `InputProfile` 是 initial profile，因此该 train 结果就是 baseline train。
- `candidate.profile`：最终 `RunResult.AcceptedProfile`，不是最后一轮 `OutputProfile`。
- `candidate.validation`：最终 accepted profile 对应的 validation 结果。若最后一轮 rejected，应回退到最近一次 accepted round 的 validation；若没有任何 accepted round，则 candidate 等于 baseline。
- `candidate.train`：对最终 `AcceptedProfile` 额外跑一次 train regression 得到，不能直接使用任意 `Round.Train`，因为 `Round.Train` 评估的是该轮输入 profile。
- `delta`：只由 `baseline.validation` 和 `candidate.validation` 做 per-case 对比。
- `rounds[*].outputProfile`：可以记录每轮 optimizer 产物，但字段名必须表明它只是 round output，不代表最终 candidate。
- `gateDecision`：只检查最终 `AcceptedProfile` 是否建议发布，不重新定义 PromptIter 的逐轮 acceptance。

`optimization_report.json` 至少包含：

- `mode`、`seed`、`targetSurfaces`、`promptSource`、`promptHash`
- `baseline.train`、`baseline.validation`
- `candidate.train`、`candidate.validation`、`candidate.acceptedProfile`
- `rounds`：每轮 train、validation、patch、PromptIter acceptance、stop reason
- `delta.perCase` 和 `delta.summary`
- `gate.decision`、`gate.reasons`
- `attribution.perFailedCase`、`attribution.summary`
- `cost.totalUsd`、`modelCallCount`、`latencyMs`
- `traceSmoke`：trace smoke 是否运行、是否跳过 optimization、跳过原因

`optimization_report.md` 面向人类解释：优化是否值得接受、主要提升、是否有新增失败、critical case 是否退化、拒绝或接受理由。

## Phase 1: Deterministic 主闭环

目标：跑通无 API Key 的真实 PromptIter pipeline，证明 patch 会影响 candidate inference。

实现内容：

- 新增示例目录、CLI、README 草稿、`promptiter.json`、`baseline_prompt.txt` 和至少 6 条样例 case。
- 启动时读取 `config/baseline_prompt.txt`，作为 `llmagent.WithInstruction(...)` 输入；报告记录 prompt 文件路径、hash 和摘要。
- Candidate 使用现有 `llmagent.New("candidate", ...)`，配 deterministic fake `model.Model` 和 `lookup_record` tool。
- 初始 `lookup_record` tool description 故意偏向 loyalty profile，导致 flight/status/delay/gate 查询不调用工具或输出 fallback。
- fake optimizer 第 1 轮返回通用 flight lookup 描述，例如 `Use lookup_record to query flight status, delay, departure time, and gate information.`
- fake model 只观察 `model.Request.Tools["lookup_record"].Declaration().Description` 和用户输入，不读取测试内部状态：
  - 如果 description 表示可查 flight/status/delay/gate，则输出 `lookup_record` tool call。
  - 工具返回固定记录后，第二次 `GenerateContent` 输出 deterministic final response。
  - direct no-tool case 不受 tool description 影响，baseline 和 candidate 都通过。
- `metrics.json` 只使用 deterministic evaluator：
  - `tool_trajectory_avg_score`：精确匹配工具名、参数、工具结果。
  - `final_response_avg_score`：精确匹配 final response。
- `RunRequest.Judge = nil`，测试覆盖 nil judge 下 deterministic metrics 可正常运行。

阶段性 PR 说明：Phase 1 可独立合并以证明真实路径可跑通，但不得声称满足完整 issue；最终交付不得保留 gate、delta、attribution 的占位字段。

## Phase 2: Validation Delta + Final Gate

目标：补齐“是否值得采用优化产物”的决策层。

per-case validation delta 分类：

- `new_pass`：baseline fail，candidate pass。
- `new_fail`：baseline pass，candidate fail。
- `improved`：两边未形成 pass/fail 翻转，但分数上升。
- `regressed`：两边未形成 pass/fail 翻转，但分数下降。
- `unchanged_pass`：两边均 pass 且分数不变。
- `unchanged_fail`：两边均 fail 且分数不变。

两层决策分工：

- PromptIter acceptance：判断某一轮 patch 是否进入 `AcceptedProfile`。
- final gate：检查最终 `AcceptedProfile` 是否建议发布。它不接管逐轮 patch 选择，只补充总分以外的发布门禁。

默认 final gate 规则：

- validation 总分提升 `>= finalGate.minValidationGain`。
- 不允许新增 hard fail。
- `finalGate.criticalCaseIDs` 中的 case 不允许退化。
- fake / trace smoke 总耗时 `< finalGate.maxDurationMs`，默认 180000 ms。
- fake mode 成本必须为 0，并记录 deterministic model / worker call count。

报告补充 gate decision、拒绝 / 接受理由、per-case delta 和 summary。final gate 输入只允许使用 `baseline.validation`、`candidate.validation`、`delta`、`criticalCaseIDs`、cost / latency，不读取 optimizer 内部意图。

## Phase 3: Failure Attribution + 过拟合拒绝

目标：满足失败归因和“训练集提升但验证集退化必须拒绝”的核心验收。

failure attribution 分类：

- `final_response_mismatch`
- `tool_not_called`
- `wrong_tool_name`
- `tool_arguments_mismatch`
- `route_error`
- `format_error`
- `knowledge_insufficient`
- `metric_failure`

归因信号来源：

- `EvalMetricResult.MetricName`
- `EvalMetricResult.EvalStatus`
- `EvalMetricResult.Details.Reason`
- expected / actual invocation 的 final response
- expected / actual invocation 的 tool calls、tool args、tool results
- execution trace 中的 terminal step 和 applied surface IDs

失败 metric 必须带 reason。PromptIter engine 会拒绝 failed metric 空 reason，因此 deterministic evaluator 在所有 fail 分支都要填稳定 reason，例如：

- `final response mismatch: expected "...", got "..."`
- `tool trajectory mismatch: expected lookup_record(flight_id=TR123), got no tool call`

测试必须覆盖 failed metric reason 被保留、空 reason 会失败、归因能从 reason 和 invocation 中给出解释。

过拟合场景分数设计：

- validation 设计为 4 个 case，每个 case 2 个 exact metric，共 8 分。
- baseline 得分 2/8 = 0.25。
- round 1 通用 flight lookup patch 得分 4/8 = 0.50，满足 PromptIter `minScoreGain`。
- round 2 optimizer 根据 train 中的 gate-only case 产出过拟合描述，例如 `Use lookup_record for flight status. For TR456, output ONLY the gate code.`
- round 2 在 train 上继续提升，并在 validation 总分达到 6/8 = 0.75，仍满足 PromptIter acceptance。
- 但 round 2 让 held-out critical case `TR789` 从 2/2 退化为 0/2，触发 final gate 的 critical regression / new hard fail，最终拒绝发布。

关键点：过拟合拒绝必须由真实 validation delta 驱动，不能硬编码在报告里。报告应说明“训练信号和总分变好，但 critical validation case 退化，因此拒绝”。

## Phase 4: Trace Smoke + 文档收口

目标：解释并验证 trace mode 支持边界，补齐交付文档。

trace smoke 实现：

- 新增 `trace_smoke.evalset.json`，包含已有 actual invocation 和 execution trace。
- CLI 支持 `-mode trace-smoke`，或测试中直接调用 trace smoke runner。
- evaluation 应能读取 trace mode case 并输出 deterministic metric / attribution。
- trace smoke 报告必须包含：
  - `traceSmoke.enabled = true`
  - `traceSmoke.optimizationSkipped = true`
  - `traceSmoke.optimizationSkippedReason = "trace mode replays actual output and cannot validate candidate inference"`
- README 明确 trace mode 是评测兼容和 smoke test，不是 prompt 优化闭环主路径。

文档收口：

- `DESIGN.md` 300-500 字，覆盖失败归因、PromptIter 接入、PromptIter acceptance 与 final gate 分工、防过拟合策略、审计报告可追溯性。
- README 补齐 fake mode、trace smoke、运行命令、输出字段、阶段性限制。
- 提交稳定示例输出：
  - `output/optimization_report.json`
  - `output/optimization_report.md`

## Hidden Sample Strategy

隐藏样本假设仍在 deterministic fake domain 内，但不能依赖固定 case ID 分支。fake model 应做成数据驱动：

- 从用户输入抽取 record id、intent 和 requested field，例如 status、delay、departure time、gate。
- lookup tool 使用 record map 返回固定记录；新增同域 record 不需要改模型逻辑。
- model 根据 tool description 的能力词判断是否调用工具，而不是判断具体 `TR123/TR456`。
- gate-only / final-response-only 行为由 requested field 和 expected format 驱动，只有过拟合 round 的特化描述才影响特定训练样本格式。

如果隐藏样本完全换领域，示例级 fake domain 无法保证泛化；README 需要声明 fake mode 用于无 API Key 的闭环验收，real-model mode 可通过任意 `model.Model` 实现扩展，但不作为测试前置。

## Test Plan

Phase 1:

- fake mode integration test 跑完整 pipeline。
- 断言 PromptIter 至少执行 1 个 round，patch 被验证集接受，validation score 提升。
- 断言 fake model 在 baseline 看到 loyalty-profile tool declaration，在 candidate round 看到 PromptIter patch 后的 flight/status/delay/gate declaration。
- 断言 deterministic metrics 在 `RunRequest.Judge = nil` 时可运行。
- 断言 `llmagent` 生成的 tool call、tool result、final response 能被 evaluation 捕获为 expected `evalset.Invocation`。
- 测试 output-dir 使用 `t.TempDir()`，不覆盖 golden report。

Phase 2:

- delta 单测覆盖 6 类状态。
- gate 单测覆盖 accepted、validation gain 不足、new hard fail、critical regression、timeout。
- 报告单测断言 `candidate.*` 来源于最终 `AcceptedProfile`，不是最后 rejected `OutputProfile`。
- 报告单测断言 `baseline.train` 来自 round 1 train，`candidate.train` 来自 accepted profile 的额外 train regression。

Phase 3:

- attribution 单测覆盖 8 类分类。
- deterministic evaluator fail 分支必须包含 non-empty reason。
- integration test 断言过拟合候选先被 PromptIter accepted，再被 final gate 因 critical validation regression 拒绝。

Phase 4:

- trace smoke test 断言 trace mode evaluation 可运行，runner inference 不作为优化证据。
- trace smoke report 断言 optimization skipped reason 存在。
- `go run . -mode fake` 可生成最终示例报告。
- `go run . -mode trace-smoke` 可生成 trace smoke 结果，且耗时小于 3 分钟。

## Assumptions

- fake mode 是完整验收主路径；real-model mode 只作为扩展说明，不绑定 ChatGPT、Codex 或任何特定模型供应商。
- trace mode 是兼容 smoke，不参与 PromptIter 优化主路径。
- 不修改 `evaluation/workflow/promptiter`、`evaluation`、`runner`、`agent` 等核心包。
- 不新增第三方依赖，尽量避免修改 `examples/evaluation/go.mod` 和 `go.sum`。
- 若 maintainer 后续要求公共抽象，再从示例中提炼；本计划不先扩大公共 API。
