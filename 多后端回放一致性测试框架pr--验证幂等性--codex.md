更推荐：补一个真实 anomaly 测试
做一个“负向/自检”case，不一定要真的改 SQLite/InMemory 后端，而是在 harness 里引入一个 test-only backend wrapper，例如：

包一层 memory service/store；
对某个指定 AddMemory 调用模拟 retry anomaly；
只在 backend B 上额外插入同一条 memory，或让 retry 产生重复记录；
运行 replay consistency；
断言返回的 diff report 包含 section=memory、对应 path、left/right 差异、backend 信息和 context。
这样可以直接回应 reviewer 的核心要求：证明 diff report 能抓到 retry duplicate effects。这个测试不需要声明当前产品已经实现幂等；它验证的是一致性框架可以发现后端在 retry 场景下的异常。