# Evaluation + Optimization 分阶段实现计划（修订版）

## Summary

继续采用 **示例级 PR、小步提交、阶段审阅** 的节奏。实现形态保持为新增 `examples/evaluation/promptiter_regression_loop/`，复用现有 `evaluation`、`runner`、`agent/llmagent` 与 `evaluation/workflow/promptiter`，不修改公共 API，不改核心包。

本文档文件名中的 `chatGPT` 仅表示计划来源 / 上下文标记，不代表实现绑定某个具体模型、供应商或 API。实现只依赖现有 `model.Model` / `llmagent.WithModel(...)` 抽象；交付主路径是 deterministic fake mode，保证没有 API Key 也能完整跑通。可选 real-model mode 可通过 OpenAI-compatible API 或其他 `model.Model` 实现接入，但不作为测试或验收前置条件。

关键修订：

- 主路径不使用 trace mode。trace mode 会跳过 candidate inference，PromptIter 改 surface 后不会影响下一轮 actual output，闭环会断裂。
- 不自定义 fake `runner.Runner`。candidate 使用现有 `llmagent.New(...)`，再用 `runner.NewRunner(appName, candidateAgent)` 接入 `evaluation.AgentEvaluator`。
- 不自定义 fake `agent.Agent`，也不在示例中直接 import `internal/surfacepatch`。PromptIter patch 由现有 `profilecompiler -> agent.WithSurfacePatchForNode -> llmagent` 路径真实应用。
- deterministic fake model 实现 `model.Model`，通过 `model.Request.Tools["lookup_record"].Declaration().Description` 观察 PromptIter patch 后的 tool declaration，并输出确定性 tool call / final response。
- fake PromptIter workers 直接实现现有 `backwarder.Backwarder`、`aggregator.Aggregator`、`optimizer.Optimizer` 接口。
- fake mode 的 `metrics.json` 只使用确定性 evaluator：`tool_trajectory_avg_score` 与 `final_response_avg_score`（exact final response criterion），不依赖 LLM judge。
- final gate 明确检查 PromptIter 最终 `AcceptedProfile`。过拟合验收采用“验证集总分满足 PromptIter 内置 acceptance、但 critical case / new hard fail 触发外置 final gate 拒绝”的设计，避免 final gate 被 engine acceptance 架空。
- 第一版不修改 parent README，只在新增示例目录内提供 README / DESIGN；等 maintainer 认可后再考虑把入口补到上级索引。

## Target Layout

最终目标布局：

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
  output/
    optimization_report.json
    optimization_report.md
  README.md
  DESIGN.md
```

职责划分：

- `main.go`：CLI flags、入口、错误处理。
- `pipeline.go`：组装 fake model、llmagent、runner、evaluator、PromptIter engine，执行 baseline、optimization、validation 与报告落盘。
- `fake.go`：deterministic fake `model.Model`、fake lookup tool，以及 fake backwarder / aggregator / optimizer。
- `report.go`：`optimization_report.json/.md` 的结构与生成，隔离输出格式细节。
- `analysis.go`：Phase 2 起承载逐 case delta、final gate、失败归因、cost / latency summary。Phase 1 可只保留最小占位逻辑，避免把第一步做成新框架。
- `pipeline_test.go`：覆盖完整 fake pipeline、patch 生效、nil judge deterministic evaluation、报告生成；Phase 2/3 再补 delta、gate、归因。

布局说明：

- `data/promptiter-regression-loop-app/` 贴合现有 local evalset manager 的默认目录约定：`data/<appName>/<evalSetID>.evalset.json`。
- `train.evalset.json` 与 `validation.evalset.json` 使用短 evalSetID，减少示例噪声。
- `metrics.json` 作为 train / validation 共享 metric 文件，避免复制两份相同 metrics；实现中用一个很小的 shared metric locator 读取它。
- `config/` 只放 pipeline 配置和 prompt 源文件，不和 evalset 数据混在一起。
- `output/` 中提交的是一次稳定 fake run 的 golden 示例输出。CLI 默认写 `./output`；测试必须使用 `t.TempDir()` 或显式临时 `-output-dir`，避免覆盖仓库中的 golden report。

`config/promptiter.json` 的最小结构：

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
    "criticalCaseIDs": ["TR789"]
  }
}
```

## Phase 1: 最小可运行闭环

目标：先跑通 fake 模式下的完整 PromptIter pipeline，可审阅、可测试、无 API Key。

- 新增示例目录、CLI、README 草稿、`promptiter.json`、baseline prompt 和 6 条样例 case。
- 启动时读取 `config/baseline_prompt.txt`，作为 `llmagent.WithInstruction(...)` 的输入；报告记录 prompt 文件路径、内容 hash 和 baseline instruction surface 摘要，避免 prompt 源文件变成未使用输入。
- 默认 target surface 为 `candidate#tool.lookup_record`。
- Candidate 采用现有 `llmagent`：
  - `llmagent.New("candidate", ...)` 使用 deterministic fake `model.Model` 和 `lookup_record` tool。
  - 初始 `lookup_record` tool description 是误导性的 loyalty-profile 描述，flight 查询不调用工具。
  - PromptIter optimizer 产生 tool declaration patch 后，`llmagent` 会通过现有 surface runtime 把 patched declaration 应用到 `model.Request.Tools`。
  - fake model 检查 `request.Tools["lookup_record"].Declaration().Description`：如果包含通用 `flight/status/delay/gate` 语义，则输出 `lookup_record` tool call；否则输出固定 fallback。
  - direct no-tool case 不受 tool description 影响，baseline 与 candidate 都通过，Phase 2 的 delta 应归为 `unchanged_pass`。
- fake lookup tool 返回固定航班记录，覆盖普通 flight status、delay、gate 信息；工具结果必须和 evalset expected tool result 精确匹配。
- fake model 的事件流通过 `llmagent` 正常进入 evaluation：
  - 第一次 `GenerateContent` 在需要工具时返回 assistant tool call response。
  - `llmagent` 执行 `lookup_record` 后，第二次 `GenerateContent` 基于 tool result 返回 final assistant response。
  - 不需要工具或 baseline fallback 时，第一次 `GenerateContent` 直接返回 final assistant response。
- `metrics.json` 只配置无模型依赖的确定性指标：
  - `tool_trajectory_avg_score`：精确匹配工具名、参数和工具结果。
  - `final_response_avg_score`：用 final response exact criterion 精确匹配最终回复。
- `RunRequest.InitialProfile = nil`，baseline 使用 candidate agent 原始 surface 值。
- `RunRequest.Judge = nil`，因为 deterministic metrics 不需要 LLM judge；Phase 1 测试必须覆盖 nil judge 下 evaluation 可正常运行。
- 实现最小 fake PromptIter workers：
  - backwarder：对失败 case 产生固定合法 `SurfaceGradient`，目标指向 `candidate#tool.lookup_record`。
  - aggregator：合并同一 surface 的 gradients，去重后返回 `AggregatedSurfaceGradient`。
  - optimizer：第 1 次优化返回通用 flight status lookup 描述，例如 `Use lookup_record to query flight status, delay, departure time, and gate information.`
- 输出最小 `optimization_report.json/.md` 到 `-output-dir`，包含 mode、seed、target surfaces、baseline train score、baseline validation score、candidate train score、candidate validation score、rounds、PromptIter acceptance、patches、cost USD=0、model call count、latency ms。
- Phase 1 不实现最终外置 gate、逐 case delta、复杂 attribution；报告字段使用明确占位值，例如 `gateDecision: "phase1-not-implemented"`、`attributionSummary: "phase1-not-implemented"`。

审阅点：candidate 是否自然走真实 `llmagent/runner/evaluation/PromptIter` 路径；PromptIter patch 是否真实反映到 fake model 看到的 tool declaration；样例数据是否贴近 issue。

## Phase 2: Validation Delta + Final Gate

目标：补齐“是否值得采用优化产物”的最终决策层。

- 实现逐 case validation delta：
  - `new_pass`
  - `new_fail`
  - `improved`
  - `regressed`
  - `unchanged_pass`
  - `unchanged_fail`
- 明确两层决策：
  - PromptIter 内置 acceptance：判断单轮 patch 是否进入 accepted profile。
  - 外置 final gate：检查 PromptIter 最终 `AcceptedProfile` 是否建议采用；它不重新接管 PromptIter 的逐轮 patch 选择，而是补充总分以外的发布门禁。
- final gate 默认规则：
  - validation 总分提升 `>= minValidationGain`。
  - 不允许新增 hard fail。
  - critical case 不允许退化；critical case 由 `config/promptiter.json` 的 `finalGate.criticalCaseIDs` 标记，不污染 evalset schema。
  - fake 模式总耗时 `< 3m`。
- fake 模式成本：
  - `cost.total_usd = 0`
  - `model_call_count` 统计 deterministic fake model 与 fake workers 调用次数。
  - `latency_ms` 使用真实 wall-clock 时间。
- 报告补充 gate decision、拒绝 / 接受理由、逐 case delta。
- 报告结构显式拆分 `baseline.train`、`baseline.validation`、`candidate.train`、`candidate.validation`，并用 validation baseline 与 candidate validation 生成逐 case delta 和 final gate 输入。

审阅点：final gate 与 PromptIter acceptance 的分工是否清晰；报告是否能解释为什么采用或拒绝候选。

## Phase 3: 失败归因 + 过拟合拒绝

目标：满足失败归因和“训练集提升但验证集退化必须拒绝”的核心验收。

- 实现 attribution 分类：
  - `final_response_mismatch`
  - `tool_not_called`
  - `wrong_tool_name`
  - `tool_arguments_mismatch`
  - `route_error`
  - `format_error`
  - `knowledge_insufficient`
  - `metric_failure`
- 归因信号来源：
  - `EvalMetricResult.MetricName`
  - `EvalMetricResult.EvalStatus`
  - `EvalMetricResult.Details`
  - actual / expected invocation 的 tool calls、tool args、final response。
- 过拟合诱导机制：
  - train 中保留 gate-only case，例如 `TR456` 只要求输出 gate code。
  - validation 中保留 held-out critical gate-only case，例如 `TR789`，并在 `config/promptiter.json` 的 `finalGate.criticalCaseIDs` 标记。
  - 第 1 轮 optimizer 返回通用描述，修复普通 flight 查询，但 `TR456` 仍因输出完整状态句而失败。
  - 第 2 轮 backwarder 观察到 `TR456` 仍失败，产生 gate-only 梯度。
  - 第 2 轮 optimizer 返回训练集特化描述，例如 `Use lookup_record for TR123/TR456 flights. For TR456, output ONLY the gate code.`
  - fake model 根据该特化描述让 `TR456` 通过，同时改善若干非 critical validation case，使验证集总分仍满足 PromptIter `minScoreGain`，因此第 2 轮 `OutputProfile` 会进入最终 `AcceptedProfile`。
  - 同一个特化描述会让 held-out critical `TR789` gate-only 验证 case 退化为 hard fail。
  - final gate 检查最终 `AcceptedProfile` 的 validation delta，发现 critical regression / new hard fail 后拒绝发布；报告展示“训练信号和总分变好，但关键验证 case 退化，因此拒绝”。
- 报告补充 failure attribution summary、每个失败 case 的 reason、过拟合拒绝说明。

审阅点：过拟合是否由真实 validation delta 驱动，而不是硬编码报告；失败归因是否可解释。

## Phase 4: Instruction Surface + Real-Model 可选模式 + 文档收口

目标：扩大需求覆盖，同时保持示例边界。

- 支持 `promptiter.json` 配置 target surface：
  - `tool`
  - `instruction`
- 默认样例仍跑 tool surface；增加 lightweight instruction config 或测试，证明 pipeline 不绑定 tool description。
- 保留 fake mode 为默认和验收主路径。
- 可选补充 real-model mode 说明：
  - 使用真实 OpenAI-compatible API 或其他 `model.Model` 实现可作为 README 中的扩展说明。
  - 不把真实 API 调用作为测试或验收前置条件。
  - 不提交依赖真实 API Key 的 golden output。
- 新增独立 `DESIGN.md`，300-500 字，覆盖：
  - 失败归因方法。
  - PromptIter 接入方式。
  - PromptIter acceptance 与 final gate 分工。
  - 防过拟合策略。
  - 审计报告可追溯性。
- 提交稳定示例输出文件：
  - `output/optimization_report.json`
  - `output/optimization_report.md`
- README 补齐 fake / real-model mode、运行命令、输出字段、阶段性限制。

审阅点：交付物是否完整，是否满足 issue 而没有扩成新框架。

## Test Plan

- Phase 1：
  - fake 模式 integration test 跑完整 pipeline。
  - 主路径断言 PromptIter 至少执行 1 个 round，首轮 patch 被验证集接受，validation score 提升，并生成报告文件。
  - 另设负例测试覆盖可解释 rejection；负例不能替代主路径 accepted + score improved 断言。
  - 断言 fake model 在 baseline 看到 loyalty-profile tool declaration，在 candidate round 看到 PromptIter patch 后的 flight/status/delay/gate tool declaration。
  - 断言 deterministic metrics 在 `RunRequest.Judge = nil` 时可正常评测。
  - 断言 `llmagent` 生成的 tool call、tool result、final response 能被 evaluation 捕获为 expected `evalset.Invocation`。
  - 测试 output-dir 使用 `t.TempDir()`，不覆盖提交在 `output/` 下的 golden report。
- Phase 2：
  - delta 单元测试覆盖 6 类状态。
  - gate 单元测试覆盖接受、validation gain 不足、new hard fail、critical regression、超时。
- Phase 3：
  - attribution 单元测试覆盖 8 类分类。
  - integration test 断言过拟合候选因 validation regression 被 final gate 拒绝。
- Phase 4：
  - instruction surface config 测试。
  - 验证 `go run . -mode fake` 可生成最终示例报告。

## Assumptions

- 每个阶段都保持可运行，不提交半成品链路。
- fake mode 是验收主路径；real-model mode 仅作可选演示说明，不绑定 ChatGPT、Codex 或任何特定模型。
- 不修改 `evaluation/workflow/promptiter`、`evaluation`、`runner`、`agent` 等核心包。
- 不新增第三方依赖，尽量避免修改 `examples/evaluation/go.mod` 与 `go.sum`。
- 若 maintainer 后续要求公共抽象，再从示例中提炼，而不是先行扩大范围。
