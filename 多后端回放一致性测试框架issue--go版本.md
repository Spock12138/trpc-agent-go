背景和价值
项目支持 InMemory、SQL、Redis 等 Session / Memory 后端，并支持多轮对话、状态读写、事件追加、长期记忆、Session Summary 等能力。生产环境经常先用 InMemory 开发，再切换到 SQL、Redis 或其他持久化后端。如果不同后端在同一条 Agent 轨迹下保存的事件顺序、state、memory 或 summary 不一致，就会导致回放错乱、上下文丢失、长期记忆污染、摘要覆盖错误等问题。
该题要求构建一个可复用的回放一致性框架，用同一组标准化输入驱动多个后端，并自动生成差异报告。它既是测试工具，也是后端实现质量的基准。除事件和状态外，还需要覆盖 Session Summary 的生成、保存、读取和更新语义，避免长对话压缩后出现摘要丢失或跨后端不一致。

trpc-agent-go 的 Session / Memory 后端更丰富，包含 InMemory、SQLite、Redis、Postgres、MySQL、ClickHouse 以及向量类 Memory 后端；Session 语义也包含 Events、State、filter-key Summaries、Tracks、事件分页、TTL 等能力。因此 Go 版实现不应只复刻 Python 的事件 / state 比较，还应覆盖 Go 仓库特有的 Summary filter-key 和 Track 观测轨迹。

任务描述
设计并实现一个 Session / Memory / Summary replay harness。输入一组多轮对话、工具事件、state 更新、memory 写入和 summary 更新操作，分别写入不同后端，再读取出来做规范化比较，判断事件、状态、记忆和摘要是否一致。对无法做到完全一致的字段，需要提供合理的归一化策略或差异解释。

具体要求
至少覆盖以下 replay case：
1.单轮普通对话：只有 user message 和 assistant text event。
2.多轮对话：连续追加多轮 user / assistant event，并检查读取顺序。
3.工具调用对话：包含 tool call、tool response、tool call args extension 等信息。
4.State 更新：同一 session 内多次写入、覆盖、删除或清空 state key。
5.Memory 写入和读取：模拟用户偏好、事实记忆、任务经验或历史摘要。
6.Summary 生成和更新：检查 summary 内容、filter-key、版本、更新时间和 session 归属。
7.Summary 与事件截断：模拟长对话压缩后，检查保留事件、summary 和后续新事件能否共同还原上下文。
8.Track 事件：模拟工具执行耗时、子任务状态、异常记录等 track 数据，并检查跨后端保存和读取一致性。
9.并发或乱序写入：模拟多个工具调用或子 Agent 事件交错追加，检查最终回放顺序和归一化结果。
10.异常恢复：模拟写入中途失败、重复写入或重试，检查是否出现重复 event、脏 state、重复 memory 或错误 summary。
后端接入要求：
● 必选：InMemory Session 后端。
● 必选：SQLite Session 后端，或一个等价的本地持久化后端。
● 可选集成：Redis、Postgres、MySQL、ClickHouse，通过环境变量开启。
● Memory 至少覆盖 InMemory 与一个持久化或模拟持久化实现。
● 若某后端暂不支持事件分页、TTL、Track 或某类 Memory 查询，需要在报告中明确标记为 unsupported，并说明是否属于 allowed_diff。比较时需要至少处理：
● Event：author、role、content、tool call、tool response、branch、tag、filterKey、stateDelta、extensions。
● State：key、value、覆盖顺序、删除语义、最终状态。
● Memory：memory id、content、metadata、scope、检索结果顺序或相似度差异。
● Summary：filter-key、summary text、版本、session 归属、覆盖关系、更新时间。
● Track：track name、event type、关联 invocation、时间序列、错误信息、耗时字段。必须对自动生成 ID、时间戳、JSON 字段顺序、map 遍历顺序、后端私有 metadata、浮点相似度或耗时指标做归一化。

交付物
● 多后端 replay consistency 测试框架代码，例如 session/replaytest/、internal/replaytest/、tests/replay_consistency/ 或等价目录。
● 至少 10 条公开 replay case，覆盖普通对话、多轮对话、工具调用、state、memory、summary、track、并发写入和异常恢复。
● session_memory_summary_track_diff_report.json 示例输出，记录 case、后端、字段路径、基准值、对比值、allowed_diff 和差异解释。
● 后端接入说明，解释如何运行轻量模式，以及如何通过环境变量开启 Redis / Postgres / MySQL / ClickHouse 集成模式。
● 一份 150 – 300 字设计说明，解释归一化策略、summary 比较策略、track 比较策略、allowed_diff 规则和后端接入方式。
● 必要的单元测试或集成测试，保证比较器、归一化器、报告生成器本身可测。

验收标准
1.至少支持 InMemory 与一个持久化后端或模拟持久化后端的对比。
2.公开提供的 10 条 replay case 必须 100% 检测出人为注入的不一致。
3.正常 case 误报率 ≤ 5%。
4.summary 丢失、summary 覆盖错误、summary 归属 session 错误三类问题检出率必须达到 100%；Go 实现还需要覆盖 summary filter-key 错误。
5.差异报告必须能定位到 session id、event index、summary id 或 summary filter-key、字段路径和两个后端的值；Go 实现还需要支持 track name 或 memory id 定位。
6.轻量模式完整运行耗时 ≤ 30 秒；集成模式可通过环境变量开启或跳过。