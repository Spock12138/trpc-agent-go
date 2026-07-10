# 自动回归与 PromptIter 优化闭环重提 PR 分阶段计划

## Summary

- 先关闭当前 PR，从干净 `main` 重新拆分实现；每个阶段对应一个可独立审阅的小 PR，避免再次把 API、示例、报告、文档、无关改动混在一起。
- 允许修改 `evaluation/workflow/promptiter/engine`，但只做最小兼容扩展；不修改 `Engine` 接口，不从 example import `internal/*`，不引入真实 API Key 或第三方依赖。
- 当前 PR 中与本 issue 无关的 `agent/invocation.go`、memory、docs、hostexec 等改动不进入新 PR；`agent/invocation.go` 的 known-state clone 风险应回退或另起独立修复 PR。
- 完整交付仍放在 `examples/evaluation/promptiter_regression_loop/`，但必须先让 `evaluation` module 和 `examples/evaluation` module 的边界编译稳定。

## Public API / Interface Changes

- 在 `evaluation/workflow/promptiter/engine` 新增可选接口，不改现有 `Engine`：
  ```go
  type ProfileEvaluator interface {
      EvaluateWithProfile(ctx context.Context, input EvalSetInput, profile *promptiter.Profile) (*EvaluationResult, error)
  }
  ```
  默认 engine 实现该接口，用于对最终 `AcceptedProfile` 额外跑 train regression，也用于 trace-smoke 的 profile evaluation 适配。
- 在 `promptiterengine.CaseResult` 增加可选字段：
  `ActualInvocation *evalset.Invocation`、`ExpectedInvocation *evalset.Invocation`。  
  `adaptEvaluationCaseResult` 从首个 `EvalMetricResultPerInvocation` 透传，缺失时保持 nil。
- 示例配置解析使用“raw config + resolved config”两层结构：数值、bool、slice 均用 pointer 判断是否显式配置。只有字段缺省时套默认值；显式 `0`、`false`、`[]` 都必须保留。
- 最终报告 schema 固定包含：`mode`、`seed`、`targetSurfaces`、`promptSource/hash`、`baseline.train/validation`、`candidate.train/validation/acceptedProfile`、`rounds`、`delta`、`attribution`、`gate`、`traceSmoke`、`cost`、`latencyMs`。
- attribution 分类采用 issue 原 8 类并新增 `unexpected_tool_call`：当 expected tools 为空但 actual tools 非空时使用该类，不再误报 `wrong_tool_name`。

## Phased Implementation

### Phase 0: PR Hygiene / 重开准备

- 关闭当前 PR，不在原 review thread 上继续堆修复。
- 从干净 `main` 新建分支；只带当前阶段需要的文件。
- 新 PR 描述中明确：当前阶段解决哪些 reviewer 问题，哪些留到后续阶段。
- 验证工作区不包含无关修改；计划文档可以留在本地，不随功能 PR 一起提交，除非作为 issue 说明需要。

### Phase 1: Engine Foundation PR

- 新增 `ProfileEvaluator` 可选接口和默认 engine 实现；不扩展 `Engine` interface。
- `EvaluateWithProfile` 复用现有 `loadStructure -> compileProfileRunOptions -> evaluate` 路径，支持 nil profile 和指定 `EvalSetInput`。
- `CaseResult` 透传 actual/expected invocation，保证 report 层可做精确工具归因。
- 补 engine 测试：profile override 确实影响 evaluation；多 run 时取 first run invocation；invocation 缺失时不 panic。
- 本阶段不新增 regression loop example，不生成 golden output。

### Phase 2: Minimal Deterministic Example PR

- 新增 `examples/evaluation/promptiter_regression_loop/` 的最小 fake pipeline：baseline train/validation、PromptIter round、candidate validation、基础 JSON/MD report。
- fake model / lookup tool 改成数据驱动：启动时从 train、validation、trace evalset 中的 expected/actual tool result 收集 record map；模型按 record id、intent、tool description capability 判断是否调用工具，不按 TR123/TR456 等固定 ID 分支。
- direct no-tool case 用用户输入中的显式 record facts 生成回答；overfit 行为由 prompt description capability 触发，不硬编码某个 evalId。
- fake backwarder / aggregator / optimizer 的计数全部用 `sync.Mutex` 或 `atomic`；optimizer 输出阶段用受控 round state，避免 race 影响 patch 选择。
- CLI 输出使用 `result.Report.Mode`，不打印原始 `modeFlag`。
- schema 测试禁止 unchecked type assertion；先 `require.IsType` / `require.NotNil` 再索引。
- 本阶段报告可先不包含 final gate、delta、attribution、trace smoke，但不能伪造完成状态；未完成项明确列入 pending。

### Phase 3: Delta / Gate / Candidate Train PR

- 用 `ProfileEvaluator` 对最终 `AcceptedProfile` 额外跑 train，生成 `candidate.train`；不得把任意 `Round.Train` 当作 candidate train。
- 实现 validation per-case delta 六类：`new_pass`、`new_fail`、`improved`、`regressed`、`unchanged_pass`、`unchanged_fail`。
- 实现 final gate：validation gain 阈值、reject new hard fail、reject critical regression、latency 上限、fake cost 必须为 0。
- 修复 config 可配置性：显式 `minValidationGain: 0`、`minScoreGain: 0`、`criticalCaseIDs: []`、`rejectOn...: false` 均有效。
- 报告写入改为“先内存生成 JSON/MD 内容 -> 写临时文件 -> 两者都成功后替换最终文件”；失败时不留下半生成的新报告。
- Markdown 与 JSON telemetry 对齐：两者都展示 cost、model call count、worker call count、latency。
- 更新 README 中 fake mode 输出字段说明。

### Phase 4: Failure Attribution / Overfit Rejection PR

- attribution 覆盖 baseline train、baseline validation、candidate validation 的失败 case；每条记录带 `phase`，summary 同时给 overall 和 by-phase。
- 归因优先级：actual/expected invocation 比对优先；其次 metric reason；最后 trace terminal output/error。
- terminal leaf 选择规则：优先选择没有后继的 leaf；多 leaf 时优先 `EndedAt` 最大的 step；`EndedAt` 为空时用 trace slice 中最后出现的 leaf，不用 `StepID` 字典序。
- 新增 `unexpected_tool_call` 分类，并同步 JSON/MD、summary、golden output 和测试。
- 构造过拟合场景：train 分数继续提升、validation 总分也提升，但 critical no-tool case 从 pass 退化为 hard fail，PromptIter accepted 后 final gate 必须 reject。
- validation 至少 4 cases × 2 metrics，保留成功优化、无效优化、优化后退化三类样例。
- 所有 failed metric reason 必须非空；空 reason 继续由 engine 测试覆盖拒绝行为。

### Phase 5: Trace Smoke / Docs / Final Golden PR

- 新增 `-mode trace-smoke`，只跑 trace evalset evaluation/report/attribution，不执行 PromptIter optimization，不产生 candidate 发布建议。
- trace-smoke 报告固定：
  `traceSmoke.enabled=true`、`optimizationSkipped=true`、`optimizationSkippedReason="trace mode replays actual output and cannot validate candidate inference"`。
- 补 `trace_smoke.evalset.json`，包含 expected conversation、actualConversation、executionTrace、tool/final response。
- README 明确 fake mode 是完整验收主路径，trace-smoke 只是 replay/evaluation/report 兼容性验证。
- 新增 `DESIGN.md` 300-500 字，覆盖失败归因、接受策略、防过拟合、PromptIter 接入、审计落盘。
- 刷新提交一份稳定 fake mode golden：`optimization_report.json` 和 `optimization_report.md`；trace-smoke 输出由测试和 README 命令覆盖，除非维护者要求提交第二份输出。

## Test Plan

- Phase 1:
  - `cd evaluation && go test ./workflow/promptiter/engine`
  - 覆盖 `EvaluateWithProfile`、profile override、actual/expected invocation 透传、nil invocation。
- Phase 2:
  - `cd examples/evaluation && go test ./promptiter_regression_loop`
  - `go test -race ./promptiter_regression_loop`
  - 覆盖 fake pipeline、data-driven records、normalized mode 输出、checked schema assertion、race-safe fake workers。
- Phase 3:
  - 覆盖 delta 六分类、final gate 全拒绝路径、candidate train 来源于最终 accepted profile、config 显式零值/空列表、report paired write、Markdown telemetry。
- Phase 4:
  - 覆盖 attribution 9 分类、baseline/candidate attribution phase、unexpected tool call、terminal leaf by `EndedAt`、overfit reject integration。
- Phase 5:
  - 覆盖 trace-smoke skip reason、trace invocation 归因、README 命令可跑、fake/trace mode 耗时均小于 3 分钟。
- 最终合并前统一跑：
  - `go test ./agent`
  - `cd evaluation && go test ./workflow/promptiter/engine`
  - `cd examples/evaluation && go test ./promptiter_regression_loop`
  - `cd examples/evaluation/promptiter_regression_loop && go run . -mode fake`

## Assumptions

- 当前 PR 会关闭，新实现以多 PR/多阶段形式重新提交。
- 允许对 `evaluation/workflow/promptiter/engine` 做最小导出能力补齐，但不破坏现有 `Engine` interface。
- hidden sample 仍在 deterministic travel/record lookup 域内；若完全换领域，README 说明 fake mode 不保证跨领域泛化。
- `unexpected_tool_call` 作为原 8 类归因的兼容扩展被接受，因为它提升报告可解释性且不移除原有分类。
- 不新增外部依赖，不修改 `examples/evaluation/go.mod` / `go.sum`，除非 Go 工具链因新增 import 自动要求且必须解释原因。
