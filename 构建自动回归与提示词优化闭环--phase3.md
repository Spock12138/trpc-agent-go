# Phase 3 Plan: Failure Attribution + 过拟合拒绝

## Summary

上下文充足，没有阻塞问题。采纳 DeepSeek 的必要优化：`CaseResult` 透传 invocation、fake optimizer 用 `callCount` 区分轮次、TR789 baseline 必须通过、validation 补足 4 个 case 形成 8 分制。

## Key Changes

- Engine 最小扩展：
  - 不改 `Engine` interface。
  - 在 `promptiter/engine.CaseResult` 增加可选 `ActualInvocation` / `ExpectedInvocation`。
  - 在 `adaptEvaluationCaseResult` 中从 `EvalMetricResultPerInvocation[0]` 透传 actual/expected invocation。
  - 现有 `summarizeEvaluationResult` 不需要改，归因逻辑单独消费 invocation。

- Failure attribution：
  - 报告新增 `attribution.perFailedCase` 和 `attribution.summary`。
  - 只对最终 `candidate.validation` 的失败 case 做归因。
  - 8 类互斥分类：`tool_not_called`、`wrong_tool_name`、`tool_arguments_mismatch`、`final_response_mismatch`、`route_error`、`format_error`、`knowledge_insufficient`、`metric_failure`。
  - 分类优先使用 actual/expected invocation；缺失时回退到 metric reason 和 trace terminal step。

- Fake overfit pipeline：
  - `config/promptiter.json` 改为 `maxRounds: 2`。
  - `fakeOptimizer.callCount == 1` 返回 partial patch：只声明支持 flight delay。
  - `fakeOptimizer.callCount == 2` 返回 overfit patch：强制所有 flight 问题都查工具，并支持 delay/gate/departure，但会破坏 cancelled/no-tool 场景。
  - fake model 改为按用户 intent 判断 capability：description 只包含 delay 时，gate/departure 请求仍不调用工具。
  - TR789 使用 cancelled no-tool 特例：baseline 和 round 1 直接回答正确；round 2 overfit patch 强制查工具并输出错误格式，形成 critical hard fail。

- Evalset score design：
  - validation 改为 4 cases × 2 metrics = 8 分。
  - baseline：只有 TR789 cancelled no-tool 通过，`2/8 = 0.25`。
  - round 1：TR789 + 1 个 delay case 通过，`4/8 = 0.50`，PromptIter accepted。
  - round 2：3 个 lookup case 通过，但 TR789 critical 退化为 0，`6/8 = 0.75`，PromptIter accepted 后被 final gate reject。
  - train 保持能驱动第 2 轮：round 1 后仍有 gate/departure 类 train loss，round 2 后 train 提升。

- Report / docs：
  - JSON 和 Markdown 展示 attribution、critical regression、new hard fail、final gate reject 原因。
  - `phase1Pending` 移除 `failure_attribution`，保留 Phase 4 的 trace smoke / design doc。
  - README 更新 Phase 3 行为边界；`DESIGN.md` 仍留 Phase 4。

## Test Plan

- Unit tests:
  - attribution 8 分类全覆盖。
  - failed metric reason 被保留。
  - failed metric 空 reason 仍由 PromptIter engine 拒绝。
  - invocation 缺失时归因可回退到 reason/trace。
  - `CaseResult` 正确透传 actual/expected invocation。

- Integration tests:
  - fake pipeline 跑满 2 轮。
  - round 1 accepted，round 2 overfit candidate 也被 PromptIter accepted。
  - `candidate.train` 高于 `baseline.train`。
  - final gate decision 为 `reject`。
  - gate reasons 包含 new hard fail / critical regression。
  - attribution 中包含 TR789 的失败解释。

- Verification commands:
  - `cd evaluation && go test ./workflow/promptiter/engine`
  - `cd examples/evaluation/promptiter_regression_loop && go test .`
  - `go run . -mode fake` 重新生成 golden reports。

## Assumptions

- 允许继续做 Phase 2 同级别的 engine 类型扩展，但不修改 `Engine` interface。
- Phase 3 不实现 trace smoke、`-mode trace-smoke` 或 `DESIGN.md`，这些仍属于 Phase 4。
- 不新增第三方依赖，不修改公共优化流程语义。
