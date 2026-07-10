下面是当前 Phase 1 v2 实现摘要，给后续 Phase 2 plan 用。

**实现范围**
当前实现全部新增在：

`D:\project\OpensourceTencent\trpc-agent-go-eval-v2\examples\evaluation\promptiter_regression_loop\`

没有改 `evaluation/**`、`runner/**`、`agent/**`、`internal/**`、`go.mod`、`go.sum` 等 core 代码。

**核心目标**
Phase 1 v2 是一个 deterministic single-round demo，用来证明：

baseline evaluation 失败  
-> PromptIter 生成 tool description patch  
-> candidate validation 通过  
-> patch 确实通过真实链路影响了 candidate inference

真实链路是：

`llmagent -> runner.NewRunner -> evaluation.AgentEvaluator -> promptiter engine`

fake 只替代：

`model.Model`、`backwarder.Backwarder`、`aggregator.Aggregator`、`optimizer.Optimizer`

**关键行为**
- CLI 只支持 `-mode fake`。
- 默认 prompt 是中性的：`You are a helpful assistant.`
- 初始工具描述是 loyalty-profile，不含 flight/status/delay/gate。
- fake optimizer 只改 `lookup_record` 的 description：
  `Use lookup_record to query flight status, delay, departure time, and gate information.`
- fake model 只读 user messages、tool description、tool result。
- intent 只从用户文本关键词提取，不读 expected invocation / expected final response / case ID。
- `RunRequest.InitialProfile = nil`
- `Judge = nil`
- `MaxRounds = 1`
- `TargetSurfaceIDs = ["candidate#tool.lookup_record"]`
- 运行前用 `engine.Describe(ctx)` 校验 target surface 存在。

**数据与结果**
数据在：

`data/promptiter-regression-loop-app/`

包含：
- `metrics.json`
- `train.evalset.json`
- `validation.evalset.json`

metric 是 deterministic：
- `tool_trajectory_avg_score`
- `final_response_avg_score`
- final response 使用 exact match

当前 report 输出在：

- `output/optimization_report.json`
- `output/optimization_report.md`

结果形态：
- baseline train: `0.3333`
- baseline validation: `0.3333`
- candidate validation: `1.0`
- candidate train: `null`
- accepted round: `1`
- `phase1Pending` 保留：
  `final_gate`、`validation_delta`、`failure_attribution`、`trace_smoke`、`candidate_train_regression`、`design_doc`

**测试状态**
可用命令是在 `examples/evaluation` 模块下执行：

```powershell
$env:GOCACHE='D:\project\OpensourceTencent\.gocache'
go test ./promptiter_regression_loop
```

已通过。

注意：仓库当前结构下，根目录命令：

```powershell
go test ./examples/evaluation/promptiter_regression_loop
```

不可用，因为 `examples/evaluation` 是独立 Go module。

**Phase 2 可接的方向**
Phase 2 可以在这个 demo 基础上补：
- 多轮 PromptIter refinement
- validation delta report
- final gate / publish decision
- failure attribution
- trace smoke
- candidate train regression
- DESIGN.md 或更完整设计文档
- 更接近真实 optimizer/backwarder 的 worker 替换策略
- 是否需要把 example module 的测试命令在 README/CI 中标准化