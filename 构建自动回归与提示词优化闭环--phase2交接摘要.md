- 给新窗口的上下文摘要如下：

  **项目与目标**

  仓库：`D:\project\OpensourceTencent\trpc-agent-go`

  当前工作围绕 `examples/evaluation/promptiter_regression_loop/`，目标是分阶段实现 “Evaluation + PromptIter 自动回归与提示词优化闭环”。

  主路径要求：

  - 使用 deterministic fake model，不依赖 API key。
  - 仍走真实链路：`llmagent -> runner -> evaluation.AgentEvaluator -> evaluation/workflow/promptiter/engine`。
  - fake 只替代 model 和 PromptIter worker，不绕过真实 patch/profile 应用路径。

  **已完成：Phase 2**

  用户要求执行 Phase 2 后，已实现并验证：

  - 在 `evaluation/workflow/promptiter/engine` 新增可选接口：

  ```
  type ProfileEvaluator interface {
      EvaluateWithProfile(ctx context.Context, input EvalSetInput, profile *promptiter.Profile) (*EvaluationResult, error)
  }
  ```

  - 默认 engine 实现 `EvaluateWithProfile`，但没有修改 `Engine` interface。
  - 示例 pipeline 在 `engine.Run(...)` 后，使用最终 `runResult.AcceptedProfile` 额外跑 train evalset，填充 `candidate.train`。
  - 报告新增：
    - `delta.perCase`
    - `delta.summary`
    - `gate.decision`
    - `gate.reasons`
    - 非空且来源正确的 `candidate.train`
  - validation delta 六分类已实现：
    - `new_pass`
    - `new_fail`
    - `improved`
    - `regressed`
    - `unchanged_pass`
    - `unchanged_fail`
  - final gate 已实现：
    - validation gain 阈值
    - new hard fail 拒绝
    - critical regression 拒绝
    - latency 上限
    - fake mode cost 必须为 0
  - `phase1Pending` 已移除 Phase 2 完成项，仍保留后续项。

  验证通过：

  ```
  cd D:\project\OpensourceTencent\trpc-agent-go\evaluation
  go test ./workflow/promptiter/engine
  
  cd D:\project\OpensourceTencent\trpc-agent-go\examples\evaluation\promptiter_regression_loop
  go test .
  ```

  也执行过：

  ```
  go run . -mode fake
  ```

  并重新生成了 golden reports。

  **当前工作树注意事项**

  当时 `git status --short` 显示：

  - `evaluation/workflow/promptiter/engine/engine.go`
  - `evaluation/workflow/promptiter/engine/evaluate.go`
  - `evaluation/workflow/promptiter/engine/engine_test.go`

  为已修改文件。

  `examples/evaluation/promptiter_regression_loop/` 显示为未跟踪目录，里面包含 Phase 1/2 示例文件和输出。

  还有一些与本任务无关的中文计划/摘要文件删除、未跟踪文件、`.cache/`、`library/`、`这是一个伟大的开始.md` 修改等状态；之前没有处理这些无关变更。

  **Phase 3 已输出计划，但尚未实现**

  用户随后要求输出 Phase 3 plan。先给过初版 plan，DeepSeek 审阅后指出三个需要补齐点，最终采纳并输出了优化后的 Phase 3 plan。

  Phase 3 目标：

  - Failure attribution
  - 训练集提升但 validation critical case 退化时，final gate 必须拒绝发布

  最终锁定的 Phase 3 方案：

  1. Engine 最小扩展

     - 不改 `Engine` interface。

     - 在 

       ```
       promptiter/engine.CaseResult
       ```

        增加可选字段：

       - `ActualInvocation`
       - `ExpectedInvocation`

     - 在 `adaptEvaluationCaseResult` 中，从：

  ```
  runResult.EvalMetricResultPerInvocation[0]
  ```

  透传：

  ```
  ActualInvocation
  ExpectedInvocation
  ```

  1. Attribution 报告

  新增：

  - `attribution.perFailedCase`
  - `attribution.summary`

  只对最终 `candidate.validation` 的失败 case 做归因。

  8 个互斥分类：

  - `tool_not_called`
  - `wrong_tool_name`
  - `tool_arguments_mismatch`
  - `final_response_mismatch`
  - `route_error`
  - `format_error`
  - `knowledge_insufficient`
  - `metric_failure`

  优先使用 actual/expected invocation，缺失时回退到 metric reason 和 trace terminal step。

  1. Fake overfit pipeline
     - `config/promptiter.json` 改为 `maxRounds: 2`。
     - `fakeOptimizer.callCount == 1` 返回 partial patch，只声明支持 flight delay。
     - `fakeOptimizer.callCount == 2` 返回 overfit patch，强制所有 flight 问题查工具，并支持 delay/gate/departure，但破坏 cancelled/no-tool 场景。
     - fake model 按 user intent 和 description capability 判断是否调用工具。
     - TR789 要设计为 baseline 通过的 cancelled no-tool critical case；round 2 overfit patch 导致它错误调用工具或错误格式输出，从而 hard fail。
  2. Validation 8 分制

  需要补第 4 个 validation case，形成：

  - 4 cases × 2 metrics = 8 分
  - baseline：`2/8 = 0.25`
  - round 1：`4/8 = 0.50`，PromptIter accepted
  - round 2：`6/8 = 0.75`，PromptIter accepted，但 TR789 critical 从 `2/2` 退化为 `0/2`
  - final gate 因 `new hard fail` / `critical regression` 拒绝发布

  1. Phase 3 测试计划

  需要覆盖：

  - attribution 8 分类
  - failed metric reason 保留
  - 空 failed metric reason 被 engine 拒绝
  - invocation 缺失时归因可回退
  - `CaseResult` 正确透传 actual/expected invocation
  - fake pipeline 跑满 2 轮
  - round 1 accepted，round 2 overfit 也被 PromptIter accepted
  - `candidate.train > baseline.train`
  - final gate decision 为 `reject`
  - gate reasons 包含 new hard fail / critical regression
  - attribution 中包含 TR789 的失败解释

  **当前无未解决疑问**

  没有需要用户额外确认的问题。Phase 3 的关键实现选择已经锁定：采纳 DeepSeek 的优化建议，并按上述方案执行。