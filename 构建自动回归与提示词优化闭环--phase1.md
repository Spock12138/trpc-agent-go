# Phase 1 修订详细计划：Deterministic PromptIter 主闭环

## Summary

Phase 1 新增 `examples/evaluation/promptiter_regression_loop/`，实现无 API Key 的最小闭环：baseline train/validation evaluation -> PromptIter round -> fake optimizer 生成 tool description patch -> candidate validation -> Phase 1 report。主路径必须真实经过 `llmagent -> runner -> evaluation -> PromptIter engine`；fake 只替代 `model.Model` 和 PromptIter worker 接口。

本修订采纳审阅中会阻塞实现的点：明确 `SurfaceValue` patch 形状、fake worker 最小合法行为、现有 evaluator 复用策略、tool call 格式、evalset JSON 形状、report 字段与完整计划对齐。`InitialProfile` 不设为必填：当前 engine 支持 `nil` baseline，Phase 1 明确使用 `InitialProfile = nil`，并用 `Describe()` 做 surface 校验。

## Key Changes

- 目录新增 `main.go`、`pipeline.go`、`fake.go`、`report.go`、`pipeline_test.go`、`README.md`、`config/`、`data/`、`output/`。`DESIGN.md` 不作为 Phase 1 必交付，列入 `phase1Pending`。
- CLI：`-mode fake` 为默认值；Phase 1 只接受 `fake`，其它 mode 返回明确错误。`-output-dir` 默认 `./output`，测试中必须传 `t.TempDir()`。
- `config/baseline_prompt.txt` 必须真实读取并传入 `llmagent.WithInstruction(...)`；report 记录 path、SHA256、summary。`config/promptiter.json` 只读取 `targetSurfaceIDs`、`maxRounds`、`acceptancePolicy.minScoreGain`、`stopPolicy.maxRoundsWithoutAcceptance`。
- `RunRequest.InitialProfile = nil`。不要手动复制 baseline surface 作为 initial profile；engine 会基于 agent exported structure normalize baseline。构建 runtime 后调用 `engine.Describe(ctx)` 校验 `candidate#tool.lookup_record` 存在，并记录 baseline tool description 供测试/report 使用。
- evaluation 复用现有 evaluator，不自建 evaluator：`tool_trajectory_avg_score` + `final_response_avg_score`。`RunRequest.Judge = nil`，`evaluation.New(...)` 不传 judge runner 或传 nil judge 配置路径，确保 deterministic metrics 不触发 LLM judge。

## Implementation Details

- Candidate agent：
  - 使用 `llmagent.New("candidate", llmagent.WithModel(fakeModel), llmagent.WithInstruction(prompt), llmagent.WithTools(newTravelTools(initialDescription)))`。
  - `initialDescription = "Look up a traveler loyalty-profile record."`，故意不包含 flight/status/delay/gate 语义。
  - target surface 用 `astructure.SurfaceID("candidate", astructure.SurfaceTypeTool, "lookup_record")` 得到 `candidate#tool.lookup_record`。

- fake optimizer patch 必须返回合法 tool `SurfaceValue`：
  ```go
  &promptiter.SurfacePatch{
      SurfaceID: request.Surface.SurfaceID,
      Value: astructure.SurfaceValue{
          Tools: []astructure.ToolRef{{
              ID:          request.Surface.Value.Tools[0].ID,
              Description: "Use lookup_record to query flight status, delay, departure time, and gate information.",
          }},
      },
      Reason: "Teach the lookup_record declaration that the neutral record fields represent flight operations data.",
  }
  ```
  `InputSchema` / `OutputSchema` 不手动填；`profilecompiler.SanitizePatchValue` 会校验 tool ID 不变，并从 baseline surface 补回 schema。

- fake backwarder 最小行为：
  - `Backward(ctx, req)` 对 nil request 或空 `AllowedGradientSurfaceIDs` 返回空 result。
  - 只对失败 loss 传入的 terminal step 返回 gradient；pass case 不会形成 loss，测试仍覆盖显式 pass/empty request 返回空。
  - 返回 `promptiter.SurfaceGradient{SurfaceID: "candidate#tool.lookup_record", Severity: promptiter.LossSeverityHigh, Gradient: "Tool declaration does not describe flight status lookup."}`。
  - `Upstream` 返回空 slice；不传播 predecessor。

- fake aggregator 最小行为：
  - 校验 `request.SurfaceID`、`request.NodeID`、`request.Type == SurfaceTypeTool`、`len(request.Gradients) > 0`。
  - 按 gradient 文本去重，返回 `AggregatedSurfaceGradient{SurfaceID: request.SurfaceID, NodeID: request.NodeID, Type: request.Type, Gradient: joinedText}`。
  - 不调用 runner，不依赖 LLM。

- fake model tool call 格式：
  ```go
  model.ToolCall{
      Type: "function",
      ID:   "call_lookup_" + recordID,
      Function: model.FunctionDefinitionParam{
          Name:      "lookup_record",
          Arguments: []byte(`{"query":"TR123"}`),
      },
  }
  ```
  当最后一条 request message 是 `RoleTool` 时，解析 tool result JSON 并输出 exact final response。lookup 请求但 description 仍是 loyalty profile 时输出固定 fallback，例如 `I do not have enough flight status information to answer that.`

- evalset / metrics：
  - `metrics.json` 包含 exact `tool_trajectory_avg_score` 和 exact `final_response_avg_score`：
    `criterion.finalResponse.text.matchStrategy = "exact"`。
  - train 3 条、validation 3 条；每组至少 2 条 lookup case + 1 条 direct no-tool case。
  - 单条 case 结构按现有 local evalset schema：
    ```json
    {
      "evalId": "flight_tr123_status",
      "conversation": [{
        "invocationId": "flight_tr123_status-1",
        "userContent": {"role": "user", "content": "Could you check the current status for flight TR123?"},
        "finalResponse": {"role": "assistant", "content": "Flight TR123 is delayed by 35 minutes and is now estimated to depart at 10:45 from gate B12."},
        "tools": [{
          "id": "tool_use_flight_tr123",
          "name": "lookup_record",
          "arguments": {"query": "TR123"},
          "result": {"recordId": "TR123", "state": "delayed", "minutes": 35, "location": "B12", "scheduled": "10:10", "updated": "10:45"}
        }]
      }],
      "sessionInput": {"appName": "promptiter-regression-loop-app", "userId": "traveler"}
    }
    ```

- report schema 与完整计划对齐：
  - `baseline.train` = round 1 `Train`，`baseline.validation` = `RunResult.BaselineValidation`。
  - `candidate.validation` = 最近 accepted round 的 `Validation`；若无 accepted round，则等于 baseline validation 并标记 no candidate accepted。
  - `candidate.train = null`，并在 `phase1Pending` 标注 `candidate_train_regression`。
  - `rounds[*].train` / `rounds[*].validation` 使用对象字段，不使用 `trainScore` / `validationScore` 顶层短字段；可在对象内保留 `score` 和 `cases` 的简化摘要。
  - `phase1Pending = ["final_gate", "validation_delta", "failure_attribution", "trace_smoke", "candidate_train_regression", "design_doc"]`。

## Test Plan

- `TestFakePipelineRunsPromptIterEndToEnd`：无 API Key、nil judge 跑完整 pipeline，生成 JSON/MD report。
- `TestBaselinePromptIsActuallyReadAndHashed`：修改临时 prompt 文件后 hash 改变，agent instruction 使用文件内容。
- `TestPromptIterPatchReachesModelToolDeclaration`：fake model 记录 baseline loyalty description 和 candidate flight/status/delay/gate description。
- `TestFakeOptimizerPatchIsValidSurfaceValue`：fake optimizer patch 经 `profilecompiler.SanitizePatchValue` 或完整 engine run 可成功编译应用。
- `TestBackwarderOnlyReturnsGradientForFailedCases`：空 request / 无 allowed surface 返回空；失败 request 返回目标 surface gradient。
- `TestDeterministicMetricsWithNilJudge`：exact tool trajectory + exact final response 在 `Judge = nil` 下通过。
- `TestToolCallAndFinalResponseCapturedByEvaluation`：actual invocation 捕获 tool call、tool result、final response。
- `TestDirectNoToolCaseRemainsStable`：direct no-tool case baseline/candidate 都 pass，且 tools 为空。
- `TestValidationScoreImprovesAfterAcceptedPatch`：baseline validation < accepted candidate validation，至少一轮 accepted。
- `TestReportSchemaMatchesFullPlanNames`：断言存在 `baseline.train`、`baseline.validation`、`candidate.validation`、`candidate.train:null`、`rounds[*].train`、`rounds[*].validation`。
- `TestReportWritesToConfiguredOutputDir`：使用 `t.TempDir()`，不覆盖仓库 `output/`。

## Assumptions

- Phase 1 是阶段性 PR，不关闭完整 issue。
- 不实现 final gate、per-case delta、failure attribution、trace smoke、candidate train regression、DESIGN.md。
- 不修改核心包、公共 API 或 go.mod/go.sum。
- fake model 行为按 record id + intent + tool description 能力词驱动，不为某个 evalId 写死通过路径。
