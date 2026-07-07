## 回答

**不，当前 `concurrent_retry_writes` 并未真正验证重试/幂等性。** 它只是在并发条件下做了 4 次成功写入，其中 2 次内容相同。这模拟的是「用户调用两次 `AddMemory`」，不是「底层因 retry 导致重复写入」。名字里的 `retry` 有名无实。

## 修复方案

两个方向，可以只选一个或两个都做：

### 方案 A（最小改动）：诚实命名 + 明确范围

把 `concurrent_retry_writes` 重命名为 `concurrent_writes`，去掉 retry 语义。在 PR 描述中声明 retry/idempotency 不在本次范围内。这是让评审者最直接放行的方式。

### 方案 B（彻底修复）：新增注入式异常测试

利用已有的 `runReplayCaseWithBackendInjection` 机制，在 `TestReplayConsistencyAnomaly_SQLitePublicAPIInjection` 中新增一条 case：在 SQLite 后端注入一条与已有 memory 内容相同的记录，模拟「重试导致重复写入」，验证 diff report 能捕获 memory 数量不一致。

新增的注入 case 大致如下：

```go
{
    name: "memory_retry_duplication",
    tc:   replayCaseByName(t, "concurrent_writes"),  // 或 memory_add_update_search
    inject: func(t *testing.T, ctx context.Context, backend backendBundle, key session.Key) {
        // 模拟重试导致的重复写入：注入与已有记录相同内容的 memory
        require.NoError(t, backend.memoryService.AddMemory(
            ctx,
            memory.UserKey{AppName: key.AppName, UserID: key.UserID},
            "Concurrent write records preferred response style.", // 与已有记录相同
            []string{"concurrency", "preference"},
        ))
    },
    section:  "memory",
    pathGlob: "$.memory[*]*",
},
```

这样当 SQLite 后续处理不当产生重复时，diff report 会清晰显示 `$.memory` 数组长度不一致，从而证明框架能捕获重试幂等性问题。

---

**推荐**：两个方案都做 — 先 A 诚实命名，再 B 补一个真正的重试异常用例，这样既解决了评审者的质疑，也让框架确实覆盖了重试场景。要我直接帮你改吗？