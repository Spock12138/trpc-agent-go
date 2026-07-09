我建议采纳 DeepSeek 的大部分审阅，但把 `candidate.train` 的 API 方案再收窄一点。

**优化后的 Phase 2 Plan**

1. **采纳：确认 `candidate.train` 是 Phase 2 前置阻塞点**  
   `examples/evaluation` 是独立 module，见 [go.mod](D:/project/OpensourceTencent/trpc-agent-go/examples/evaluation/go.mod:1)，所以示例代码不能直接 import 根模块的 `internal/profilecompiler`。  
   因此 `candidate.train` 不能在 example 里硬拼 profile 编译逻辑。

2. **调整：不直接修改 `Engine` 接口，优先新增可选 profile evaluation 能力**  
   不建议直接给 `Engine` interface 加 `EvaluateWithProfile`，因为这会破坏所有外部自定义 `Engine` 实现。更稳的方案是：

   - 在 `promptiter/engine` 包内新增可选接口，例如 `ProfileEvaluator`
   - 让默认 `*engine` 实现它
   - example 中用 type assertion 调用

   形态类似：

   ```go
   type ProfileEvaluator interface {
       EvaluateWithProfile(ctx context.Context, input EvalSetInput, profile *promptiter.Profile) (*EvaluationResult, error)
   }
   ```

   这样仍然需要改 engine 包，但不破坏现有 `Engine` interface。比直接扩展 `Engine` 更符合“不改变已有公共行为”的约束。

3. **采纳：pipeline 补 `candidateTrain` 明确步骤**  
   `runFakePipeline` 在 `engine.Run(...)` 后新增：

   - 取最终 `runResult.AcceptedProfile`
   - 调用 profile evaluation 跑 train evalset
   - 将结果通过 `reportInput.candidateTrain` 传入 `buildOptimizationReport`

   继续明确禁止把任意 `Round.Train` 当成 `candidate.train`。

4. **采纳：`new hard fail` 封装为函数**  
   Phase 2 仍按 deterministic exact-match 域定义：

   ```go
   baseline 不是 hard fail && candidate 是 !passed && score == 0
   ```

   但实现上封装为 `isHardFail(caseSummary) bool`，方便后续 soft metric 改阈值。

5. **采纳：新增边界测试**  
   增加 `TestCandidateTrainUsesAcceptedProfileNotLastRound`。  
   构造 round 1 accepted、round 2 rejected 的场景，断言 `candidate.train` 来自最终 accepted profile，而不是最后 rejected `OutputProfile`。

6. **保持：过拟合集成场景仍放 Phase 3**  
   Phase 2 只做 final gate 机制和单测级拒绝场景。  
   “训练集提升但 critical validation 退化，因此 gate 拒绝”的完整多轮 pipeline 留到 Phase 3。

**Phase 2 仍然交付**

- `validation_delta` 六分类
- `final_gate` 发布决策
- `candidate.train` 非空且来源正确
- 报告移除已完成的 `phase1Pending` 项
- JSON / Markdown 报告补齐 delta、gate、candidate train
- `go test .` 通过

**仍需确认的疑问**

是否允许 Phase 2 对 `evaluation/workflow/promptiter/engine` 增加一个可选 profile evaluation 能力？  
如果完全不允许改 engine 包，那么 `candidate.train` 只能退化为绕路方案或继续延期，这会影响 Phase 2 的完整性。