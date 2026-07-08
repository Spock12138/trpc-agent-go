# 优化 Replay Consistency Review 修复方案

## Summary

- 把 `test/go.mod` 从 `go 1.25.0` 改回 `go 1.24.4`，避免无必要提高 test/e2e 工具链下限。
- 不直接比较 raw `SummaryBoundary.LastEventID`，改为比较归一化后的事件锚点位置。
- 在代码注释和文档中明确 `last_event_index = -1` 是 sentinel，表示 boundary 有非空 `LastEventID`，但该 ID 未能映射到当前 snapshot 的事件列表。
- 用户未跟踪的中文 `.md` 学习笔记保持原样，不删除、不提交。

## Key Changes

- 在 `makeReplaySnapshot` 中只取一次事件列表，并复用给 events 和 summaries：
  ```go
  events := sess.GetEvents()
  Events:  normalizeReplayEvents(events)
  Summary: normalizeReplaySummaries(cloneReplaySummaries(sess), events)
  ```

- 调整内部 snapshot schema：
  ```go
  type summaryBoundary struct {
      Version        int    `json:"version"`
      FilterKey      string `json:"filter_key"`
      CutoffAt       string `json:"cutoff_at,omitempty"`
      LastEventIndex *int   `json:"last_event_index,omitempty"`
  }
  ```

- 新增内部 helper，将 `LastEventID` 映射为稳定语义位置：
  ```go
  func replaySummaryLastEventIndex(events []event.Event, lastEventID string) *int
  ```
  规则：
  - 空 `LastEventID`：返回 `nil`，字段省略。
  - 找到匹配事件 ID：返回 zero-based event index。
  - 非空但找不到：返回 `-1` sentinel。
  - 不把 raw `LastEventID` 写入 snapshot。

- 文档更新：
  - 英文：`SummaryBoundary` version, filter key, cutoff, and normalized last-event anchor.
  - 中文：`SummaryBoundary` 的 version、filter key、cutoff，以及归一化后的 last-event 锚点。
  - 追加一句说明：unmatched/non-empty anchors are reported as `last_event_index: -1`；中文同义说明。

## Test Plan

- 替换现有 `TestReplayConsistencySnapshotNormalize_IgnoresSummaryLastEventID` 为语义锚点测试：
  - raw event ID / raw `LastEventID` 不同，但都映射到 index `0` 时，不产生 diff。
  - 一个后端有 anchor、另一个后端没有 anchor 时，产生 summary diff。
  - 一个后端 anchor 有效、另一个后端 `LastEventID` 非空但找不到时，产生 `last_event_index` diff，右侧值为 `-1`。

- diff 断言：
  - `Section == "summary"`
  - `Path == $.summary["branch/a"].boundary.last_event_index`
  - `Allowed == false`
  - `Context["summary_filter_key"] == "branch/a"`

- 验证命令：
  ```powershell
  cd D:\project\OpensourceTencent\trpc-agent-go\test
  go test ./... -run "TestReplayConsistencySnapshot"
  go test ./...
  ```

## Assumptions

- 使用完整 `sess.GetEvents()` 的 zero-based index 作为 summary boundary anchor 的语义位置；如果事件顺序本身有问题，现有 events diff 会单独暴露。
- 不比较 raw `LastEventID`，因为 `event.Event.Clone()` 会重生成 ID，且 replay harness 已明确忽略 event ID。
- 不运行 `go mod tidy`，除非测试暴露必须更新 module metadata。
- 提交时精确 add：
  ```powershell
  git add test/go.mod test/replay_consistency_test.go docs/mkdocs/en/session/replay_consistency.md docs/mkdocs/zh/session/replay_consistency.md
  ```
