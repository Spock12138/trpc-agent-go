- 下面这段可以直接贴到新窗口，作为 Phase 4 开发上下文。

  **项目上下文**
  仓库：`D:\project\OpensourceTencent\trpc-agent-go`
  
  当前目标围绕：
  
  ```
  examples/evaluation/promptiter_regression_loop/
  ```
  
  实现 “Evaluation + PromptIter 自动回归与提示词优化闭环”。
  
  主路径要求一直保持不变：
  
  - 使用 deterministic fake model，不依赖 API key。
  - 仍走真实链路：`llmagent -> runner -> evaluation.AgentEvaluator -> evaluation/workflow/promptiter/engine`。
  - fake 只替代 model 和 PromptIter worker，不绕过真实 patch/profile 应用路径。
  
  **Phase 3 状态**
  Phase 3 已完成并通过审阅。
  
  Phase 3 目标：
  
  - Failure attribution。
  - 即使 PromptIter 因训练/validation 总分提升接受了 overfit patch，只要 validation critical case 退化，final gate 必须拒绝发布。
  
  **Phase 3 已实现内容**
  Engine 最小扩展：
  
  - 没有修改 `Engine` interface。
  
  - 在 
  
    ```
    evaluation/workflow/promptiter/engine/evaluate.go
    ```
  
     的 
  
    ```
    CaseResult
    ```
  
     新增：
  
    - `ActualInvocation *evalset.Invocation`
    - `ExpectedInvocation *evalset.Invocation`

  - ```
    adaptEvaluationCaseResult
    ```
  
     从第一条 run 的第一条 per-invocation metric 透传：
  
    - `runResult.EvalMetricResultPerInvocation[0].ActualInvocation`
    - `runResult.EvalMetricResultPerInvocation[0].ExpectedInvocation`
  
  - 新增 helper：`firstRunInvocations(...)`。
  
  - `engine_test.go` 已补测试，确认多 run 场景仍取第一 run，并正确透传 invocation。
  
  报告 attribution：
  
  - ```
    OptimizationReport
    ```
  
     新增：
  
    - `attribution.perFailedCase`
    - `attribution.summary`
  
  - 只对最终 `candidate.validation` 的失败 case 做归因。

  - 支持 8 个互斥分类：

    - `tool_not_called`
    - `wrong_tool_name`
    - `tool_arguments_mismatch`
    - `final_response_mismatch`
    - `route_error`
    - `format_error`
    - `knowledge_insufficient`
    - `metric_failure`
  
  - 分类优先用 actual/expected invocation；缺失时回退 metric reason 和 trace terminal step。
  
  - 单测覆盖了 8 分类、reason 保留、invocation 缺失回退等路径。

  Fake overfit pipeline：

  - `config/promptiter.json` 的 `maxRounds` 已改为 `2`。

  - ```
    fakeOptimizer
    ```

    ：
  
    - 第 1 次返回 partial patch：只声明 delay 能力。
    - 第 2 次返回 overfit patch：强制所有 flight/TR 问题都查工具。
  
  - fake model 按 user intent 和 tool description capability 决定是否查工具。
  
  - overfit patch 会破坏 critical no-tool case。

  Validation 8 分制：

  - validation 现在是 4 cases × 2 metrics = 8 分。
  - 当前报告关键分数：
    - baseline validation：`0.25`
    - round 1 validation：`0.50`
    - round 2 / candidate validation：`0.75`
    - baseline train：`0.3333333333`
    - candidate train：`0.6666666667`
  - PromptIter round 1 accepted。
  - PromptIter round 2 overfit 也 accepted。
  - 但 final gate 因 critical no-tool case 退化而 reject。

  关键 validation case：

  - ```
    flight_tr789_cancelled_no_tool
    ```
  
    - baseline：通过，`2/2`
    - round 2 candidate：失败，`0/2`
    - attribution：`wrong_tool_name`
    - gate reasons 包含：
      - `new hard fail count is 1`
      - `critical regression count is 1`
  
  **输出报告**
  已执行：
  
  ```
  cd D:\project\OpensourceTencent\trpc-agent-go\examples\evaluation\promptiter_regression_loop
  go run . -mode fake
  ```
  
  已刷新：
  
  - `examples/evaluation/promptiter_regression_loop/output/optimization_report.json`
  - `examples/evaluation/promptiter_regression_loop/output/optimization_report.md`
  
  报告中：
  
  - `gate.decision = reject`

  - `candidate.accepted = true`

  - `attribution.perFailedCase` 包含 `flight_tr789_cancelled_no_tool`
  
  - ```
    phase1Pending
    ```

     当前只剩：

    - `trace_smoke`
    - `design_doc`
  
  **验证通过**
  已通过：
  
  ```
  cd D:\project\OpensourceTencent\trpc-agent-go\evaluation
  go test ./workflow/promptiter/engine
  
  cd D:\project\OpensourceTencent\trpc-agent-go\examples\evaluation\promptiter_regression_loop
  go test .
  ```

  **当前工作树注意事项**
  相关修改文件：
  
  - `evaluation/workflow/promptiter/engine/engine.go`
  - `evaluation/workflow/promptiter/engine/evaluate.go`
  - `evaluation/workflow/promptiter/engine/engine_test.go`
  - `examples/evaluation/promptiter_regression_loop/` 整个示例目录仍是未跟踪目录，里面包含 Phase 1/2/3 示例文件和输出。
  
  无关变更仍未处理：
  
  - 若干中文计划文件删除/新增。
  - `.cache/`
  - `library/`
  - `这是一个伟大的开始.md`
  
  这些无关变更不要回滚、不要清理，除非用户明确要求。
  
  **Phase 4 可接续方向**
  从当前 `phase1Pending` 看，Phase 4 最自然的剩余工作是：
  
  - `trace_smoke`
  - `design_doc`
  
  当前没有未解决疑问。