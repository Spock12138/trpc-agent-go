# trpc-agent-go 多后端回放一致性测试框架计划

## Summary

在 `D:\project\OpensourceTencent\trpc-agent-go\test` module 内新增 Go 版 replay consistency harness，第一版 runnable matrix 只覆盖 `InMemory + SQLite`，同时比较 `session / events / state / memory / summary / tracks`。报告默认写到仓库根目录 `D:\project\OpensourceTencent\trpc-agent-go\session_memory_summary_track_diff_report.json`，也支持 `TRPC_AGENT_REPLAY_REPORT_PATH` 覆盖。

Go 版不机械复刻 Python：summary 按 Go 原生 `Session.Summaries[filterKey]` 和 `SummaryBoundary` 比较；memory 用 `AddMemory / UpdateMemory / SearchMemories` 显式驱动；track 用 `session.TrackService.AppendTrackEvent` 覆盖。

## Key Changes

- 在 `D:\project\OpensourceTencent\trpc-agent-go\test\replay_consistency_test.go` 新增 `package e2e` 测试框架，包含 test-internal 的 `replayCase / eventSpec / memoryOpSpec / summaryStep / trackSpec / allowedDiffRule / backendBundle / diffEntry`，不新增运行时 public API。
- 更新 `D:\project\OpensourceTencent\trpc-agent-go\test\go.mod`：加入 `session/sqlite`、`memory/sqlite` 和 `github.com/mattn/go-sqlite3`，并用 local `replace` 指向 `..\session\sqlite`、`..\memory\sqlite`。
- 后端工厂固定为：
  - `in_memory`：`session/inmemory.NewSessionService` + `memory/inmemory.NewMemoryService`
  - `sqlite`：临时 SQLite 文件 DB + `session/sqlite.NewService` + `memory/sqlite.NewService`
  - 两者都注入 deterministic fake summarizer，summary 同步执行，禁用异步不确定性。
- Snapshot 归一化规则：
  - events 去掉生成型 `id/timestamp/response.id/response.timestamp`，保留 role/content/tool calls/tool response/branch/filterKey/tag/stateDelta/extensions/actions。
  - state 将 `[]byte` 规范化为 JSON 或 UTF-8 字符串，map key 排序。
  - memory 按内容、topics、metadata 建 canonical key；不把后端生成 memory ID 当等值字段，但报告中保留左右 raw ID 用于定位。
  - summary 比较 filter-key map、summary text、boundary version/filterKey/cutoff/last event ref；`UpdatedAt` 只校验非零并归一化。
  - tracks 按 track name 和事件顺序比较，payload 做 canonical JSON，timestamp 用固定 spec 时间或归一化。
- Diff/report 规则：
  - report entry 包含 `case/session_id/backend_a/backend_b/section/path/left/right/allowed/reason/context`。
  - `context` 可携带 `event_index`、`summary_filter_key`、`memory_key`、`left_memory_id/right_memory_id`、`track_name`。
  - `allowed_diff` 默认为空，只接受显式 section + path glob + backend pair + reason；ID/时间差异通过归一化处理，不靠 allowed_diff 放行。
- 至少 10 个公开 replay cases：
  - single turn、multi turn、tool call/response/extensions、session/app/user state、scoped state/filterKey、memory CRUD/search、full summary、filter-key summary、summary overwrite/boundary、track events、interleaved child invocations、recovery anomaly。
- 异常检测测试：
  - 用真实 SQLite public API 注入 duplicate event、state pollution、memory pollution、summary overwrite。
  - 用 snapshot mutation 覆盖 partial loss、summary loss、wrong session attribution、wrong filter-key、track payload drift，确保 diff path 和 report context 精确。
- 文档新增到 `D:\project\OpensourceTencent\trpc-agent-go\docs\mkdocs\zh\session\replay_consistency.md` 和对应英文页，并在 `D:\project\OpensourceTencent\trpc-agent-go\docs\mkdocs.yml` 的 Session 导航挂载。文档说明轻量模式、报告路径、归一化策略、allowed_diff、以及未来外部后端如何按 env-gated backend factory 接入。

## Test Plan

- 运行 targeted 测试：`cd D:\project\OpensourceTencent\trpc-agent-go\test && go test ./... -run ReplayConsistency -count=1`
- 运行 e2e module 全量测试：`cd D:\project\OpensourceTencent\trpc-agent-go\test && go test ./...`
- 验证正常矩阵 report 为 `[]`，注入测试能 100% 定位到 event index、summary filter-key、memory canonical key/raw ID、track name 和字段 path。
- 确认轻量模式无外部服务依赖，运行时间小于 30 秒。

## Assumptions

- 第一版 PR 只把 `InMemory + SQLite` 放入 runnable matrix；Redis/Postgres/MySQL/ClickHouse 不在首 PR 接入。
- 默认 report 文件使用 Go issue 名称 `session_memory_summary_track_diff_report.json`，不同于 Python 的 `session_memory_summary_diff_report.json`。
- Go summary 不生成 Python 式 summary event，也不比较 historical events；只比较 Go session summary map 与 boundary 语义。





# Go 回放一致性测试分阶段计划

## Summary

按 6 个阶段推进，每阶段都能独立验证，避免一次性塞完整 harness。第一版范围固定为 `InMemory + SQLite`，外部 Redis/Postgres/MySQL/ClickHouse 只预留接口和文档说明，不进入 runnable matrix。

## 阶段 1：测试模块与后端工厂

目标：先让 Go `test` module 能创建两个等价后端。

- 修改 `D:\project\OpensourceTencent\trpc-agent-go\test\go.mod`，加入 `session/sqlite`、`memory/sqlite`、`go-sqlite3`，并配置 local `replace`。
- 新建 `D:\project\OpensourceTencent\trpc-agent-go\test\replay_consistency_test.go`，使用 `package e2e` 和 Tencent license header。
- 实现最小结构：`backendBundle`、`makeBackends(t)`、`closeBackends(t)`、临时 SQLite DB helper、deterministic fake summarizer。
- 只写一个 smoke test：创建 InMemory/SQLite session + memory service 并关闭。

完成标准：`cd test && go test ./... -run ReplayConsistencySmoke -count=1` 通过。

## 阶段 2：Snapshot、Normalize、Diff、Report

目标：先把比较器做扎实，不急着接真实 replay case。

- 定义 snapshot schema：`session/events/state/memory/summary/tracks`。
- 实现归一化：
  - events 去掉生成型 ID/时间戳，保留 role/content/tool calls/tool response/branch/filterKey/tag/stateDelta/extensions。
  - state 的 `[]byte` 尝试按 JSON 解析，否则按字符串比较。
  - memory 按内容/topics/metadata 排序，raw ID 只用于 report 定位。
  - summary 按 filter-key map 比较，保留 boundary version/filterKey/cutoff/lastEventID。
  - tracks 按 track name + index 比较，payload canonical JSON。
- 实现 recursive diff、`allowedDiffRule`、report entry、默认 report path：仓库根目录 `session_memory_summary_track_diff_report.json`，env 覆盖 `TRPC_AGENT_REPLAY_REPORT_PATH`。
- 写纯内存单测：event/state/memory/summary/track 的 snapshot mutation 都能产生精确 path。

完成标准：比较器测试通过，report entry 至少包含 `case/session_id/backend_a/backend_b/section/path/left/right/allowed/reason/context`。

## 阶段 3：基础 Replay Matrix

目标：真实驱动 InMemory 和 SQLite，先覆盖 events/state/memory。

- 定义 `replayCase`、`eventSpec`、`memoryOpSpec`、`memoryQuerySpec`。
- 实现 `_runCase` 的 Go 版：CreateSession、AppendEvent、UpdateSessionState 或 stateDelta、Add/Update/Delete/Search memory、GetSession 后生成 snapshot。
- 加入 6 个基础 case：
  - single turn
  - multi turn
  - tool call + tool response + args extension
  - session/app/user/scoped state
  - memory add/update/search
  - interleaved child invocation / branch order
- 实现 matrix 比较：所有 backend pair diff 后写 report，未 allowed diff 时测试失败。

完成标准：正常矩阵 report 为 `[]`，`cd test && go test ./... -run ReplayConsistencyMatrix -count=1` 通过。

## 阶段 4：Go 特有 Summary 与 Track

目标：补齐 Go issue 相比 Python 多出的核心风险面。

- Summary：
  - 用 fake summarizer 返回确定性文本。
  - 调用 `CreateSessionSummary(ctx, sess, filterKey, true)`。
  - 覆盖 full summary、filter-key summary、summary overwrite/update、boundary metadata。
  - 比较 `Session.Summaries[filterKey]` 和 `GetSessionSummaryText`，不做 Python 式 summary event/historical events。
- Track：
  - 通过 `session.TrackService.AppendTrackEvent` 写入工具耗时、子任务状态、错误 payload。
  - snapshot 比较 track name、payload、事件顺序。
- 将 replay cases 扩到至少 10 条公开 case。

完成标准：summary filter-key 错误和 track payload/order 错误能被 snapshot diff 明确定位。

## 阶段 5：异常注入与 allowed_diff

目标：证明 harness 能发现问题，而不是只验证正常路径。

- 真实 SQLite/public API 注入：
  - duplicate event
  - state pollution
  - memory pollution
  - summary overwrite
- Snapshot mutation 注入：
  - partial event loss
  - summary loss
  - wrong session attribution
  - wrong summary filter-key
  - track payload drift
- allowed_diff 测试：
  - 只有显式 section + path glob + backend pair + reason 命中的 diff 才 allowed。
  - 默认不允许任何业务字段差异。
- 验证 report context 包含 event index、summary filter-key、memory raw ID/canonical key、track name。

完成标准：issue 要求的人工注入不一致 100% 检出，正常 case 误报为 0。

## 阶段 6：文档与收尾

目标：让贡献者知道怎么跑、怎么看报告、怎么扩展后端。

- 新增中文文档：`D:\project\OpensourceTencent\trpc-agent-go\docs\mkdocs\zh\session\replay_consistency.md`。
- 可选新增英文对应页，并在 `D:\project\OpensourceTencent\trpc-agent-go\docs\mkdocs.yml` Session 导航挂载。
- 文档说明：
  - 轻量模式只跑 InMemory + SQLite。
  - report path 与 env override。
  - 归一化策略、summary filter-key 策略、track 比较策略、allowed_diff 规则。
  - 外部后端后续如何接入 env-gated backend factory，当前首版标记为 deferred/unsupported。
- 最后运行：
  - `cd D:\project\OpensourceTencent\trpc-agent-go\test && go test ./...`
  - 必要时跑 targeted：`go test ./... -run ReplayConsistency -count=1`

完成标准：测试、文档、报告示例语义都满足 issue 验收，且轻量模式小于 30 秒。





# 阶段一计划：测试模块与后端工厂

## 目标

只完成最小可运行骨架：让 `D:\project\OpensourceTencent\trpc-agent-go\test` module 能同时构造、使用、关闭 `InMemory` 与 `SQLite` 的 Session/Memory/Track 后端。阶段一不实现 snapshot、diff、report，也不添加 replay cases。

## 文件改动

- 修改 `D:\project\OpensourceTencent\trpc-agent-go\test\go.mod`：
  - `require` 增加：
    - `github.com/mattn/go-sqlite3 v1.14.32`
    - `trpc.group/trpc-go/trpc-agent-go/session/sqlite v0.0.0`
    - `trpc.group/trpc-go/trpc-agent-go/memory/sqlite v0.0.0`
  - `replace` 增加：
    - `trpc.group/trpc-go/trpc-agent-go/session/sqlite => ../session/sqlite`
    - `trpc.group/trpc-go/trpc-agent-go/memory/sqlite => ../memory/sqlite`
- 新增 `D:\project\OpensourceTencent\trpc-agent-go\test\replay_consistency_test.go`：
  - package 使用现有 e2e 测试包：`package e2e`
  - 文件头使用仓库 Tencent Apache-2.0 license header
  - import 使用别名避免冲突：
    - `sessinmemory`
    - `sesssqlite`
    - `meminmemory`
    - `memsqlite`
    - `_ "github.com/mattn/go-sqlite3"`

## 实现内容

- 定义 `backendBundle`：
  - `name string`
  - `sessionService session.Service`
  - `trackService session.TrackService`
  - `memoryService memory.Service`
  - `summarizer *deterministicSummarizer`
- 定义 `deterministicSummarizer`，实现 `session/summary.SessionSummarizer`：
  - `ShouldSummarize` 固定返回 `true`
  - `Summarize` 返回当前配置文本，默认 `"smoke summary"`
  - `SetPrompt`、`SetModel` 空实现
  - `Metadata` 返回 `map[string]any{"deterministic": true}`
- 定义 `openSQLiteDB(t, name string) *sql.DB`：
  - 使用 `t.TempDir()` 生成临时 db 文件路径
  - `sql.Open("sqlite3", path)`
  - 不在 helper 内关闭 DB，因为 SQLite service owns DB and closes it in `Close`
- 定义 `makeReplayBackends(t) []backendBundle`：
  - `in_memory`：
    - `sessinmemory.NewSessionService(sessinmemory.WithSummarizer(sum))`
    - `meminmemory.NewMemoryService(meminmemory.WithMinSearchScore(0), meminmemory.WithMaxResults(0))`
  - `sqlite`：
    - session 和 memory 使用两个独立 SQLite DB，避免两个 service 共同拥有同一个 `*sql.DB`
    - `sesssqlite.NewService(sessionDB, sesssqlite.WithSummarizer(sum))`
    - `memsqlite.NewService(memoryDB, memsqlite.WithMinSearchScore(0), memsqlite.WithMaxResults(0))`
  - 注册 `t.Cleanup` 调用 `closeReplayBackends`
- 定义 `closeReplayBackends(t, backends)`：
  - 逐个关闭 `memoryService.Close()`
  - 再关闭 `sessionService.Close()`
  - 所有 close error 用 `require.NoError`

## Smoke Test

新增 `TestReplayConsistencySmoke_BackendsConstructUseAndClose`：

- 对每个 backend：
  - 创建 session：`CreateSession(ctx, session.Key{AppName: "replay-smoke", UserID: "user-1", SessionID: backend.name + "-session"}, session.StateMap{"stage": []byte("smoke")})`
  - `GetSession` 验证 session 非空、state 存在
  - `AddMemory` 写入 `"smoke memory for replay consistency"`
  - `SearchMemories` 查询 `"smoke"`，验证至少一条结果
  - `AppendTrackEvent` 写入 track `"smoke-track"`，payload 为 canonical JSON：`{"status":"ok"}`
  - 再次 `GetSession` 验证 tracks 中包含 `"smoke-track"`
- 这个 smoke test 只验证后端基础可用，不做跨后端等值比较。

## 验证命令

- 修改后先在 test module 内整理依赖：
  - `cd D:\project\OpensourceTencent\trpc-agent-go\test`
  - `go mod tidy`
- 运行阶段一 targeted 测试：
  - `go test ./... -run ReplayConsistencySmoke -count=1`
- 若 SQLite 编译失败，确认 `CGO_ENABLED=1`，因为 `github.com/mattn/go-sqlite3` 需要 CGO。

## 完成标准

- `go test ./... -run ReplayConsistencySmoke -count=1` 通过。
- `test/go.mod` 和 `test/go.sum` 只出现阶段一必要依赖变更。
- 新文件只包含后端工厂、fake summarizer、SQLite helper、close helper、smoke test。
- 不生成 `session_memory_summary_track_diff_report.json`，report 留到阶段二实现。





# 第五步计划：异常注入与 allowed_diff 验证

## Summary

第五步目标是证明 replay consistency harness 不只是验证正常路径，而是能稳定发现人工制造的不一致，并给出精确 diff path 与 report context。实现范围继续限定在 `test/replay_consistency_test.go`，不新增运行时 public API，不接入外部后端，不改文档。

本阶段优先做 snapshot mutation，再做 SQLite/public API 注入，最后收紧并验证 `allowed_diff` 规则。

## Key Changes

- 新增异常验证 helper：
  - `refreshReplayCaseResultSnapshot(t, ctx, backend, key)`：在 public API 注入后重新 `GetSession + ReadMemories + makeReplaySnapshot`。
  - `runReplayCaseWithBackendInjection(t, ctx, tc, targetBackend, inject)`：正常跑 InMemory/SQLite，然后只对目标 backend 注入异常并刷新 snapshot。
  - `requireReplayDiff(t, diffs, section, pathGlob, context)`：按 section/path/context 定位目标 diff，允许 path glob。
  - `requireReplayReportFields(t, reportPath)`：解析 report 并确认字段完整。
- 新增 snapshot mutation 测试，建议命名：
  - `TestReplayConsistencyAnomaly_SnapshotMutations`
  - 覆盖 `partial_event_loss`、`summary_loss`、`wrong_session_attribution`、`wrong_summary_filter_key`、`track_payload_drift`、`track_order_drift`
  - 验证 context：`event_index`、`summary_filter_key`、`track_name`、`track_event_index`
- 新增 SQLite/public API 注入测试，建议命名：
  - `TestReplayConsistencyAnomaly_SQLitePublicAPIInjection`
  - 只污染 `sqlite` backend，再和 `in_memory` 比较
  - 覆盖 duplicate event、state pollution、memory pollution、summary overwrite
  - 验证 diff 均为 unallowed，并写入 temp report 检查字段完整
- 收紧 `allowedDiffRule`：
  - 只有 `Section`、`Path`、`BackendA`、`BackendB`、`Reason` 全部显式填写才允许
  - `Reason` 必须非空白
  - `Section` 不允许空或 `*`
  - `Path` 不允许空或单独 `*`，但允许如 `$.memory[*].content` 的 path glob
  - `BackendA/BackendB` 不允许空或 `*`，但匹配仍支持左右 backend 顺序互换
- 新增 allowed_diff 专项测试，建议命名：
  - `TestReplayConsistencyAllowedDiffRules_RequireExplicitMatch`
  - 覆盖无规则、缺 reason、section/path/backend 不匹配、通配 section/backend、有效 path glob、反向 backend pair

## Test Cases And Scenarios

- Snapshot mutation：
  - 删除右侧 `Events[0]`，期望 `section=events`，path 命中 `$.events[0]`，context 有 `event_index=0`
  - 删除 `Summary["branch/a"]`，期望 `section=summary`，context 有 `summary_filter_key=branch/a`
  - 修改 `Session.ID`，期望 `section=session`，path 为 `$.session.id`
  - 把 `Summary["branch/a"]` 移到 `Summary["branch/wrong"]`，期望两个 summary diff 都能定位 filter key
  - 修改 `Tracks[0].Events[0].Payload`，期望 context 有 `track_name` 与 `track_event_index`
  - 交换同一 track 下两个事件，期望 diff path 精确到 `$.tracks[*].events[*]`
- SQLite/public API 注入：
  - duplicate event：基于 `single_turn`，对 sqlite 追加重复/额外 event，期望 events diff
  - state pollution：基于 `single_turn`，对 sqlite `UpdateSessionState` 写入污染 key，期望 state diff
  - memory pollution：基于 `memory_add_update_search`，对 sqlite `AddMemory` 写入额外 memory，期望 memory diff 且 context 有 memory key/raw ID
  - summary overwrite：基于 `full_summary`，对 sqlite 改 summarizer text 后 `CreateSessionSummary(..., "", true)`，期望 summary text diff
- 正常矩阵：
  - `TestReplayConsistencyMatrix_BasicCases` 仍必须通过，report 为 `[]`
  - 不在仓库根目录留下 `session_memory_summary_track_diff_report.json`

## Test Plan

在 `D:\project\OpensourceTencent\trpc-agent-go\test` 下运行：

```powershell
$env:CGO_ENABLED="1"
$env:CC="D:\tools\mingw\mingw64\bin\gcc.exe"
$env:GOPATH="D:\project\OpensourceTencent\.gopath"
$env:GOCACHE="D:\project\OpensourceTencent\.gocache"
& "D:\go\go1.26.4.windows-amd64\go\bin\go.exe" test ./... -run ReplayConsistency -count=1
& "D:\go\go1.26.4.windows-amd64\go\bin\go.exe" test ./... -count=1
```

验收标准：

- 正常 replay matrix 零误报
- 注入不一致 100% 检出
- 注入 diff 默认全部 unallowed
- allowed_diff 只在显式规则完整命中时 allowed
- report entry 保持包含 `case/session_id/backend_a/backend_b/section/path/left/right/allowed/reason/context`
- summary/track context 精确包含 filter key、track name、track event index

## Assumptions

- 第五步继续只覆盖 `InMemory + SQLite`。
- 不引入 `EnqueueSummaryJob`，summary 异常仍基于同步 `CreateSessionSummary`。
- 不把 replay timestamp 改回绝对历史时间，继续使用阶段四的 `replayBaseTime`。
- track payload/order 注入优先 mutate snapshot，避免 `state["tracks"]` 产生无关 diff。
- 阶段五完成后新增 `多后端回放一致性测试框架--阶段五执行摘要.md`，记录注入项、测试结果和踩坑。






# 阶段六计划：文档与收尾

## Summary

阶段六目标是把 replay consistency harness 的使用方式、报告语义、归一化策略、异常检测能力和扩展边界写入文档，并完成最终验证。实现范围以文档为主，不再扩大测试 harness 功能。

## Key Changes

- 新增中文文档：
  - `docs/mkdocs/zh/session/replay_consistency.md`
  - 导航挂到 `docs/mkdocs.yml` 中文 Session 分组下，建议标题：`回放一致性测试`
- 新增英文文档：
  - `docs/mkdocs/en/session/replay_consistency.md`
  - 导航挂到 `docs/mkdocs.yml` 英文 Session 分组下，建议标题：`Replay Consistency`
- 文档必须说明：
  - 当前轻量矩阵只跑 `InMemory + SQLite`
  - 默认 report：仓库根目录 `session_memory_summary_track_diff_report.json`
  - env override：`TRPC_AGENT_REPLAY_REPORT_PATH`
  - 正常矩阵期望 report 为 `[]`
  - snapshot 覆盖 `session/events/state/memory/summary/tracks`
  - ID/时间类生成字段通过 normalize 处理，不靠 `allowed_diff`
  - summary 按 Go 原生 `Session.Summaries[filterKey]`、`SummaryBoundary`、`GetSessionSummaryText` 比较
  - track 按 track name、event order、payload、timestamp 比较
  - 异常注入覆盖 snapshot mutation 与 SQLite/public API pollution
  - 外部 Redis/Postgres/MySQL/ClickHouse 当前 deferred/unsupported，不进入 runnable matrix

## allowed_diff 文档规则

文档中的 `allowed_diff` 示例必须使用阶段五后的严格语义：

```json
{
  "section": "memory",
  "path": "$.memory[*].content",
  "backend_a": "in_memory",
  "backend_b": "sqlite",
  "reason": "known backend-specific normalization gap"
}
```

必须明确：

- `section` 必填，不能是空或 `*`
- `path` 必填，不能是空或单独 `*`
- `backend_a/backend_b` 必填，不能是空或 `*`
- `reason` 必填且非空白
- backend pair 支持左右顺序互换
- path 支持局部 glob，例如 `$.memory[*].content`
- 默认不允许任何业务字段差异

## Test Plan

实现后在 `D:\project\OpensourceTencent\trpc-agent-go\test` 执行：

```powershell
$env:CGO_ENABLED="1"
$env:CC="D:\tools\mingw\mingw64\bin\gcc.exe"
$env:GOPATH="D:\project\OpensourceTencent\.gopath"
$env:GOCACHE="D:\project\OpensourceTencent\.gocache"
& "D:\go\go1.26.4.windows-amd64\go\bin\go.exe" test ./... -run ReplayConsistency -count=1
& "D:\go\go1.26.4.windows-amd64\go\bin\go.exe" test ./... -count=1
```

同时检查：

- 仓库根目录没有残留 `session_memory_summary_track_diff_report.json`
- `docs/mkdocs.yml` 英文和中文 Session 导航都包含新页面
- 中英文文档内容一致，尤其是 `allowed_diff` 严格规则一致
- 新增 `多后端回放一致性测试框架--阶段六执行摘要.md`，记录文档文件、导航变更、测试结果和最终验收状态

## Assumptions

- 阶段六不改运行时 public API。
- 阶段六不新增 replay case 或异常注入测试。
- 阶段六不接入外部后端，只在文档中说明后续扩展方向。
- 中英文文档都新增，避免 mkdocs 双语导航不一致。





# 多后端回放一致性测试框架--阶段六执行摘要

更新时间：2026-07-01

## 阶段目标

阶段六完成 replay consistency harness 的文档与收尾，让贡献者知道怎么运行、怎么看 report、如何理解 normalize/summary/track/allowed_diff，以及外部后端当前为何不进入默认矩阵。

## 已完成改动

新增中文文档：

- `docs/mkdocs/zh/session/replay_consistency.md`

新增英文文档：

- `docs/mkdocs/en/session/replay_consistency.md`

更新导航：

- `docs/mkdocs.yml`
  - 英文 Session 分组新增 `Replay Consistency: session/replay_consistency.md`
  - 中文 Session 分组新增 `回放一致性测试: session/replay_consistency.md`

## 文档覆盖内容

中英文文档均说明：

- 轻量矩阵当前只跑 `InMemory + SQLite`
- targeted 和全量测试命令
- SQLite 需要 CGO 与 C 编译器
- 默认 report 路径：`session_memory_summary_track_diff_report.json`
- env override：`TRPC_AGENT_REPLAY_REPORT_PATH`
- 正常矩阵 report 应为 `[]`
- report entry 字段与 context 定位信息
- snapshot 覆盖 `session/events/state/memory/summary/tracks`
- 生成型 ID/时间字段通过 normalize 处理
- Go summary 使用原生 `Session.Summaries[filterKey]`、`SummaryBoundary`、`GetSessionSummaryText`
- track 按 track name、event order、payload、timestamp 比较
- 异常注入覆盖 snapshot mutation 和 SQLite/public API injection
- `allowed_diff` 必须显式完整配置
- 外部 Redis/PostgreSQL/MySQL/ClickHouse 等后端当前 deferred/unsupported

## allowed_diff 文档规则

文档中的示例包含完整字段：

- `section`
- `path`
- `backend_a`
- `backend_b`
- `reason`

文档明确：

- `section` 不能空或 `*`
- `path` 不能空或单独 `*`
- `backend_a/backend_b` 不能空或 `*`
- `reason` 必须非空白
- backend pair 支持左右顺序互换
- path 支持局部 glob，例如 `$.memory[*].content`
- ID/时间差异应优先修 normalize 或 runner，不用 `allowed_diff` 放行

## 验证结果

已在 `D:\project\OpensourceTencent\trpc-agent-go\test` 执行：

```powershell
$env:CGO_ENABLED="1"
$env:CC="D:\tools\mingw\mingw64\bin\gcc.exe"
$env:GOPATH="D:\project\OpensourceTencent\.gopath"
$env:GOCACHE="D:\project\OpensourceTencent\.gocache"
& "D:\go\go1.26.4.windows-amd64\go\bin\go.exe" test ./... -run ReplayConsistency -count=1
```

结果：

```text
ok  	trpc.group/trpc-go/trpc-agent-go/test	0.510s
```

已执行全量未缓存测试：

```powershell
& "D:\go\go1.26.4.windows-amd64\go\bin\go.exe" test ./... -count=1
```

结果：

```text
ok  	trpc.group/trpc-go/trpc-agent-go/test	0.546s
```

额外检查：

- `docs/mkdocs.yml` 英文导航包含 `Replay Consistency`
- `docs/mkdocs.yml` 中文导航包含 `回放一致性测试`
- 仓库根目录未生成/残留 `session_memory_summary_track_diff_report.json`

## 收尾状态

阶段一到阶段六目标已闭环：

- 后端工厂与 smoke test
- snapshot/normalize/diff/report
- 基础 replay matrix
- Go 特有 summary 与 track
- 异常注入与严格 `allowed_diff`
- 中英文文档与导航

后续若要扩展外部后端，建议按 env-gated backend factory 单独规划，不要让默认本地测试依赖外部服务。

## Git 与 PR 准备状态

当前功能分支：

- `feat/replay-consistency-harness`

本地提交：

- `2868ecee test: add replay consistency harness`

该提交包含的 issue 必要文件：

- `docs/mkdocs.yml`
- `docs/mkdocs/en/session/replay_consistency.md`
- `docs/mkdocs/zh/session/replay_consistency.md`
- `test/go.mod`
- `test/go.sum`
- `test/replay_consistency_test.go`

远端状态：

- `origin` 指向上游 `https://github.com/trpc-group/trpc-agent-go.git`
- `spock` 指向个人 fork `https://github.com/Spock12138/trpc-agent-go.git`
- 当前分支已关联 `spock/feat/replay-consistency-harness`

明确不进入 PR/commit 的复盘与计划文件仍保持未跟踪状态：

- `多后端回放一致性测试框架--chatGPT的plan.md`
- `多后端回放一致性测试框架--阶段一执行摘要.md`
- `多后端回放一致性测试框架--阶段二执行摘要.md`
- `多后端回放一致性测试框架--阶段三执行摘要.md`
- `多后端回放一致性测试框架--阶段四执行摘要.md`
- `多后端回放一致性测试框架--阶段四计划.md`
- `多后端回放一致性测试框架--阶段四到阶段五交接摘要.md`
- `多后端回放一致性测试框架--阶段五执行摘要.md`
- `多后端回放一致性测试框架--阶段六执行摘要.md`
- `多后端回放一致性测试框架issue--go版本.md`

## PR 描述注意事项

PR 描述中测试命令建议使用通用写法，避免暴露本机路径：

```bash
CGO_ENABLED=1 go test ./... -run ReplayConsistency -count=1
CGO_ENABLED=1 go test ./... -count=1
```

不建议把本地 `CC`、`GOPATH`、`GOCACHE` 或 Go 安装路径写进 PR 描述。可以保留一句说明：SQLite 测试需要 CGO，因为 test module 使用 `github.com/mattn/go-sqlite3`。

## 审查重点

审查时优先确认：

- harness 是否只影响 `test` 模块与文档，不改变 runtime public API
- 默认测试矩阵是否仍为本地可运行的 `InMemory + SQLite`
- 正常矩阵 report 是否为空数组
- 异常注入是否能稳定产生 unallowed diff
- `allowed_diff` 是否仍要求显式 section/path/backend/reason，避免宽泛放行
- report 字段是否保持 `case/session_id/backend_a/backend_b/section/path/left/right/allowed/reason/context`
- summary/track 文档是否和 Go 侧实现一致





八荣八耻

以臆猜接口为耻，以查档求证为荣
以模糊开工为耻，以对齐需求为荣
以脑补业务为耻，以请示规则为荣
以新增冗余为耻，以复用存量为荣
以省略校验为耻，以完备测例为荣
以乱改架构为耻，以恪守规范为荣
以不懂装懂为耻，以坦诚存疑为荣
以批量乱改为耻，以分步迭代为荣