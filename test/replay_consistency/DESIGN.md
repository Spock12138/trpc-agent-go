# Replay Consistency 设计

`session/replaytest` 提供 runner、normalize、compare、report，不依赖数据库；`test` 保留 backend factory、用例、断言与故障注入。ID/时间戳归一化，state 保留字节类型；summary 比较 filter-key/boundary，track 比较名称、顺序、payload。`allowed_diff` 必须限定 section、path、backend pair、reason。retry 分写入前后：前者恢复基线，后者验证 memory/state/summary 幂等并报告重复 event。
