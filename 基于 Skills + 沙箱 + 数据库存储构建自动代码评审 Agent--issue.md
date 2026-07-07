背景和价值
tRPC-Agent 的 Skill 体系可以把可复用工作流封装为 SKILL.md、文档和脚本，并通过 skill load / skill run 在隔离 workspace 中执行。CodeExecutor 支持本地、容器和 Cube / E2B 沙箱，Session / Memory / SQL 存储可以持久化审查任务、代码片段、诊断结果和历史经验，Filter、Permission 和 Telemetry 可以记录审查链路中的拦截、耗时、异常和风险分布。
把这些能力组合起来，可以构建一个面向真实工程场景的自动 CR Agent：读取 diff，识别风险，必要时在沙箱中运行静态检查或测试，把结论结构化落库，并支持后续评测、监控和回放。该题难点在于它不是“让 LLM 评论代码”，而是要把 Skills、沙箱执行、数据库、治理策略、审查规则、结果结构化、监控审计和安全边界串成一个可验证系统。

trpc-agent-go 已有 tool/skill、tool/workspaceexec、tool/hostexec、tool/codeexec、codeexecutor/container、codeexecutor/e2b、artifact、session/sqlite、tool.PermissionPolicy 和 telemetry。Go 版实现应围绕 Go 项目代码评审场景设计，例如 go test、go vet、staticcheck 可选执行、diff hunk 解析、Go 并发 / context / error handling / resource lifecycle 规则，而不是只做通用文本点评。

任务描述
设计并实现一个自动代码评审 Agent 原型。输入一个 git diff、PR patch 或本地变更目录，Agent 通过代码评审 Skill 加载规则和脚本，在治理策略允许后进入沙箱执行必要检查，把发现的问题按严重级别、文件、行号、证据和修复建议结构化输出，并将审查任务、拦截记录、监控摘要和结果写入数据库或可替代的持久化存储。

具体要求
系统至少包含以下能力：
1.CR Skill：提供一个 code-review Skill，包含 SKILL.md、规则文档、脚本目录和使用说明。规则至少覆盖安全风险、goroutine / context 泄漏、资源关闭、错误处理、测试缺失、敏感信息泄漏、数据库事务或连接生命周期问题中的 4 类。
2.沙箱执行：支持通过 codeexecutor/container 或 codeexecutor/e2b workspace runtime 执行 go test、go vet、diff 解析脚本或自定义规则脚本；本地 runtime 只能作为开发 fallback，不能作为默认生产方案。
3.工具链接入：可通过 skill_run、workspace_exec、codeexec 或 wrapper 编排检查脚本，但高风险命令必须先经过 PermissionPolicy 或安全 wrapper 决策。
4.输入解析：支持读取 unified diff、文件路径列表或 git 工作区变更，提取变更文件、hunk、上下文、候选行号和 Go package 信息。
5.结构化审查结果：输出 findings，字段至少包含 severity、category、file、line、title、evidence、recommendation、confidence、source、rule_id。
6.数据库存储：设计并实现最小 schema，用于保存 review task、input diff 摘要、sandbox run、permission / filter decision、finding、artifact、最终报告。可以使用 SQLite 作为默认实现，但接口应保留切换 SQL 后端的空间。
7.去重和降噪：同一文件同一行同一类问题不能重复报；低置信度问题应进入 warnings 或 ask / needs_human_review，不能混入高置信 findings。
8.安全边界：沙箱执行需要有超时、输出大小限制、环境变量白名单、敏感信息脱敏、artifact 限制和失败记录。
9.监控审计：记录每次 review 的总耗时、沙箱执行耗时、工具调用次数、Permission 拦截次数、finding 数量、各 severity 分布、异常类型分布。
输入输出要求：
● 输入支持 --diff-file、--repo-path 或测试 fixture。
● 输出 review_report.json 和 review_report.md。
● SQLite 或等价持久化存储中可查询每次 review 的 task 状态、sandbox run、permission decision、监控摘要、findings、artifact 和最终结论。
● 支持 dry-run / fake model / deterministic rule-only 模式，保证没有真实模型 API Key 时也能测试 diff 解析、沙箱执行、落库和报告生成链路。
● 建议放在 examples/skills_code_review_agent/、examples/code_review_agent/ 或等价目录。

交付物
● Go 示例目录和入口，例如 main.go、CLI 或可运行测试。
● skills/code-review/SKILL.md、规则文档、沙箱执行脚本和 Agent 编排代码。
● 数据库 schema、存储实现、初始化脚本或 migration。
● 至少 8 条测试样例：无问题 diff、安全问题、goroutine / context 泄漏、资源未关闭、数据库连接生命周期问题、测试缺失、重复 finding、沙箱执行失败、敏感信息脱敏。
● review_report.json、review_report.md 示例输出和 README。
● 一份 300 – 500 字方案设计说明，解释 Skill 设计、沙箱隔离策略、Permission / Filter 策略、监控字段、数据库 schema、去重降噪和安全边界。
● 单元测试覆盖 diff 解析、finding 去重、敏感信息脱敏、落库查询、sandbox 失败不崩溃。

验收标准
1.公开提供的 8 条 diff 样本必须全部可运行并生成审查报告。
2.隐藏样本上高危问题检出率 ≥ 80%，误报率 ≤ 15%。
3.数据库必须能完整记录 task、sandbox run、finding 和 report，并支持按 task id 查询；Go 实现还需要记录 Permission / safety decision。
4.沙箱执行必须具备超时控制和输出大小限制；超时或失败不能导致整个评审任务崩溃。
5.敏感信息脱敏检出率 ≥ 95%，报告和数据库中不能出现明文 API Key、token、password。
6.dry-run / fake model 模式下完整评审流程耗时 ≤ 2 分钟。
7.高风险脚本或命令必须先经过 Filter / Permission 决策，deny / needs_human_review / ask 不能直接进入沙箱执行。
8.报告必须包含 findings 摘要、严重级别统计、人工复核项、治理拦截摘要、监控指标、沙箱执行摘要和可执行修复建议。