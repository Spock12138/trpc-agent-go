# 两主机工作区同步摘要：Evaluation + Optimization 闭环

## 目标

当前任务是解决 `构建 Evaluation + Optimization 的自动回归与提示词优化闭环issue.md` 对应的 issue。

两台主机轮换工作时，统一使用 GitHub 上的 workspace 分支作为同步中介：

```powershell
feat/eval-optimization-loop-workspace
```

这个分支只作为“移动工作区”，用于同步代码、草稿、阶段总结、实验记录和临时笔记。它不直接提交 PR。

最终提交上游时，从最新 `main` 新建干净 PR 分支：

```powershell
feat/eval-optimization-loop
```

只把确认要进入上游的代码、测试和正式文档迁移到这个干净分支，个人笔记、草稿、阶段记录不进入最终 PR。

## 核心原则

1. `feat/eval-optimization-loop-workspace` 是工作区分支，不是最终 PR 分支。
2. 切换主机前必须 commit + push。
3. 每次开始工作前必须 fetch/pull。
4. 笔记文件可以提交到 workspace 分支，方便两台机器同步。
5. 代码 commit 和笔记 commit 尽量分开，方便最终 cherry-pick。
6. 如果某个 commit 同时包含代码和笔记，最终整理 PR 时不要直接 cherry-pick 到干净分支；应使用 `cherry-pick -n` 后只暂存需要的代码文件。
7. 不要从 workspace 分支直接向上游提交 PR。

## 第一次在某台主机准备环境

进入仓库：

```powershell
cd D:\project\OpensourceTencent\trpc-agent-go
```

获取远端分支：

```powershell
git fetch origin
```

如果远端已经有 workspace 分支：

```powershell
git switch feat/eval-optimization-loop-workspace
```

如果本机还没有这个分支，但远端已经存在：

```powershell
git switch --track origin/feat/eval-optimization-loop-workspace
```

如果远端还没有这个分支，则从最新 main 创建：

```powershell
git switch main
git pull --ff-only origin main
git switch -c feat/eval-optimization-loop-workspace
git push -u origin feat/eval-optimization-loop-workspace
```

## 每次开始工作前

进入仓库：

```powershell
cd D:\project\OpensourceTencent\trpc-agent-go
```

确认当前状态：

```powershell
git status
git branch --show-current
```

切到 workspace 分支：

```powershell
git switch feat/eval-optimization-loop-workspace
```

拉取另一台机器的最新进度：

```powershell
git pull --ff-only
```

如果 `git pull --ff-only` 失败，通常说明两台机器都产生了提交，先不要强推。执行：

```powershell
git status
git log --oneline --decorate --graph -10
```

然后根据情况选择 rebase 或 merge。没有把握时先停下，避免覆盖另一台机器的工作。

开始写代码前，建议快速阅读：

```text
构建 Evaluation + Optimization 的自动回归与提示词优化闭环issue.md
构建 Evaluation + Optimization 的自动回归与提示词优化闭环--chatGPT的plan.md
构建 Evaluation + Optimization 的自动回归与提示词优化闭环--phase1.md
构建 Evaluation + Optimization 的自动回归与提示词优化闭环--两主机工作区同步摘要.md
```

也可以看最近提交：

```powershell
git log --oneline -10
```

## 工作过程中

推荐小步提交。

代码变更：

```powershell
git add -p
git commit -m "feat: add xxx"
```

测试或修复：

```powershell
git add -p
git commit -m "fix: handle xxx"
```

笔记、阶段总结、计划更新：

```powershell
git add "*.md"
git commit -m "docs: update workspace notes"
```

如果代码和笔记确实需要一起提交，也可以，但最终整理 PR 时要记得拆分。

## 每次结束工作前

确认当前改动：

```powershell
git status
```

如果还有未提交内容，先提交：

```powershell
git add -p
git commit -m "wip: save eval optimization workspace progress"
```

如果只是笔记：

```powershell
git add "*.md"
git commit -m "docs: save workspace notes"
```

推送到远端，让另一台机器可以接手：

```powershell
git push
```

结束前再次确认：

```powershell
git status
```

理想状态是：

```text
On branch feat/eval-optimization-loop-workspace
Your branch is up to date with 'origin/feat/eval-optimization-loop-workspace'.
nothing to commit, working tree clean
```

## 最终整理 PR 分支

不要从 workspace 分支直接提 PR。

从最新 main 新建干净分支：

```powershell
git switch main
git pull --ff-only origin main
git switch -c feat/eval-optimization-loop
```

查看 workspace 分支提交：

```powershell
git log --oneline main..feat/eval-optimization-loop-workspace
```

如果某些提交只包含正式代码、测试或正式文档，可以直接 cherry-pick：

```powershell
git cherry-pick <commit-hash>
```

如果某个提交混有代码和笔记，使用不自动提交的方式：

```powershell
git cherry-pick -n <commit-hash>
```

然后只暂存要进入 PR 的文件：

```powershell
git restore --staged .
git status
git add -p <需要进入PR的代码文件或目录>
git commit -m "feat: add eval optimization loop"
```

提交完成后，工作区里可能还残留不进入 PR 的笔记文件。先确认：

```powershell
git status
```

如果残留的是已跟踪文件的修改，可以丢弃：

```powershell
git restore <不进入PR的笔记文件>
```

如果残留的是新增的未跟踪笔记文件，先预览将要清理的文件：

```powershell
git clean -n <不进入PR的笔记文件>
```

确认只会删除不进入 PR 的笔记文件后，再清理：

```powershell
git clean -f <不进入PR的笔记文件>
```

整理完成后检查 PR 分支不要包含个人笔记：

```powershell
git status
git diff --name-only main...HEAD
```

确认只包含应该提交给上游的代码、测试和正式文档后再推送：

```powershell
git push -u origin feat/eval-optimization-loop
```

## 遇到冲突时

先不要强推，不要 reset，不要删除文件。

先看清楚状态：

```powershell
git status
git log --oneline --decorate --graph -20
```

常见处理方式：

```powershell
git pull --rebase
```

如果 rebase 过程中出现冲突，解决冲突后：

```powershell
git add <resolved-files>
git rebase --continue
```

完成后：

```powershell
git push
```

如果不确定冲突含义，先停下，不要用 `git push --force`。

## 当前工作入口

每次重新开始时，先读本文件，然后读 issue 和最近阶段记录。

推荐顺序：

1. `git switch feat/eval-optimization-loop-workspace`
2. `git pull --ff-only`
3. 阅读本摘要
4. 阅读 issue
5. 阅读最近阶段记录
6. `git log --oneline -10`
7. `git status`
8. 开始实现




固定的操作顺序：
可以。以后你就把它当成一个固定的“两主机交接协议”。

**第一次接入某台机器**

只在这台机器第一次使用 workspace 分支时执行：

```powershell
cd D:\project\OpensourceTencent\trpc-agent-go

git fetch spock
git switch --track spock/feat/eval-optimization-loop-workspace
```

如果本机已经有这个分支，以后不要再用 `--track`，直接切换即可：

```powershell
git switch feat/eval-optimization-loop-workspace
```

**每次开始工作前**

```powershell
cd D:\project\OpensourceTencent\trpc-agent-go

git switch feat/eval-optimization-loop-workspace
git fetch spock
git pull --ff-only
git status
```

确认看到类似：

```text
On branch feat/eval-optimization-loop-workspace
nothing to commit, working tree clean
```

然后再开始开发。

**工作过程中提交**

代码和笔记尽量分开提交：

```powershell
git status
git add -p
git commit -m "feat: xxx"
```

笔记类：

```powershell
git add "*.md"
git commit -m "docs: update workspace notes"
```

临时保存也可以：

```powershell
git add -A
git commit -m "wip: save workspace progress"
```

**每次结束工作后**

这是最重要的交接动作：

```powershell
git status
git add -A
git commit -m "wip: save workspace progress"
git push spock feat/eval-optimization-loop-workspace
git status
```

如果 `git status` 显示没有改动，可以跳过 `add/commit`，只确认是否需要 push：

```powershell
git status
git push spock feat/eval-optimization-loop-workspace
```

**另一台机器接手时**

```powershell
cd D:\project\OpensourceTencent\trpc-agent-go

git switch feat/eval-optimization-loop-workspace
git fetch spock
git pull --ff-only
git status
```

**铁律**

1. 开工前：`pull --ff-only`
2. 收工前：`commit + push`
3. 不要从 `feat/eval-optimization-loop-workspace` 直接提 PR
4. 最终 PR 从 `main` 新建干净分支 `feat/eval-optimization-loop`
5. workspace 分支可以乱一点，PR 分支必须干净一点

如果你只记一个最短版：

```powershell
# 开工
git switch feat/eval-optimization-loop-workspace
git pull --ff-only

# 收工
git add -A
git commit -m "wip: save workspace progress"
git push spock feat/eval-optimization-loop-workspace
```

这个流程就够用了。