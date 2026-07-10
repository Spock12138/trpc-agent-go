**Phase 2 交接摘要**

Phase 1 已完成一个无 API Key 的 deterministic PromptIter 主闭环，新增目录在 [examples/evaluation/promptiter_regression_loop](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/README.md:1)。主链路真实经过 `llmagent -> runner -> evaluation.AgentEvaluator -> promptiter engine`，fake 只替换了 `model.Model` 和 PromptIter worker 接口。

当前入口是 [main.go](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/main.go:27)，核心流程在 [pipeline.go](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/pipeline.go:97)。默认 `-mode fake`，其他 mode 会返回 Phase 1 明确错误。`RunRequest` 里 `InitialProfile = nil`、`Judge = nil`，只使用 deterministic metrics。

当前可复现结果：baseline train/validation 都是 `0.3333333333`，candidate validation 是 `1.0`，PromptIter 第 1 轮 accepted，patch 目标是 `candidate#tool.lookup_record`。报告输出在 [optimization_report.json](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/output/optimization_report.json:1) 和 [optimization_report.md](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/output/optimization_report.md:1)。

Phase 2 应从 [report.go](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/report.go:40) 扩展报告结构，补齐 `validation_delta` 和 `final_gate`。建议新增 `analysis.go`，实现 per-case delta 六类：`new_pass`、`new_fail`、`improved`、`regressed`、`unchanged_pass`、`unchanged_fail`。final gate 只基于 `baseline.validation`、最终 accepted candidate validation、delta、critical case、cost/latency 做发布建议，不要改变 PromptIter 每轮 acceptance 语义。

Phase 2 还要补 `candidate.train`：当前 [report.go](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/report.go:157) 里明确是 `nil`，因为 Phase 1 没有对最终 `AcceptedProfile` 额外跑 train regression。实现时不要直接拿任意 `Round.Train` 当 candidate train，因为 round train 评估的是该轮输入 profile，不一定是最终 accepted profile。

测试当前是绿的：我刚在 `examples/evaluation/promptiter_regression_loop` 跑了 `go test .`，结果通过。Phase 2 建议新增/扩展测试覆盖 delta 六类、gate 通过、validation gain 不足、new hard fail、critical regression、timeout，以及报告字段来源必须来自最终 `AcceptedProfile`。

明确问题/注意事项：

- 当前工作区有无关变更：中文计划文件有删除/未跟踪的改名状态，另有 `.cache/`、`library/`、`这是一个伟大的开始.md` 变化；Phase 2 不应误碰这些。
- 当前终端读取中文 Markdown 时出现乱码，像是编码显示问题；代码和英文报告正常。
- 计划文档里的部分 API 名称已和实际代码不同：实际实现使用 `AggregatedSurfaceGradient.Gradients` 和 `LossSeverityP1`。
- metrics 使用了自定义 [metricsFileLocator](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/promptiter_regression_loop/pipeline.go:95)，让 train/validation 共享 `metrics.json`，不是默认 `<evalSetID>.metrics.json`。
- `phase1Pending` 仍包含 `final_gate`、`validation_delta`、`failure_attribution`、`trace_smoke`、`candidate_train_regression`、`design_doc`，这是预期未完成项，不应在 Phase 2 后继续把 `final_gate` 和 `validation_delta` 留作占位。