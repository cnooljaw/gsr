# GSR 开发、审查与提交工作流

本项目当前采用单人开发、单分支工作流。Agent 负责实现和验证，用户负责审查变更并决定是否提交。除非用户明确要求，不创建或切换分支，不提交，不推送远端。

## 1. 标准循环

~~~text
Agent：修改
  → Agent：运行最小相关测试
  → 用户：审查 diff
  → （用户要求修正时）Agent：修正并重新测试
  → 用户明确说“commit/提交”
  → Agent：整理、暂存、复核并创建本地提交
~~~

“审查”与“提交”是两个独立决定。用户只要求审查时，工作树必须继续保持未提交状态。

## 2. Agent 修改阶段

开始前先确认工作树，避免覆盖用户已有改动：

~~~bash
cd /Users/lijiawang/Documents/cocos/laya/gsr
git status --short
~~~

然后只修改当前任务范围内的文件，并按 AGENTS.md 的验证规则运行最小相关检查。例如 Go 代码切片通常按以下顺序推进：

~~~bash
gofmt -w path/to/changed.go
go test ./path/to/affected/package
git diff --check
git diff --stat
~~~

每个 `Test...` 测试函数前写一句注释，说明它保护的可观察行为，方便审查者从测试入口快速理解契约；不要只重复函数名。

本阶段 Agent 不执行以下操作：

~~~text
git add
git commit
git push
git reset --hard
git checkout --
git clean
~~~

完成后，Agent 应汇报：修改了哪些文件、验证执行了什么、验证是否通过、是否存在未解决问题。工作树留给用户审查。

## 3. 用户审查阶段

### VS Code

在 VS Code 打开 /Users/lijiawang/Documents/cocos/laya/gsr 根目录，点击左侧“源代码管理”：

- Changes：查看未暂存修改。
- Staged Changes：查看已经暂存的修改。
- 点击文件可以逐行查看 diff。

命令行等价物：

~~~bash
git diff                 # 未暂存修改
git diff --cached        # 已暂存修改
git diff HEAD            # 相对 HEAD 的全部已跟踪修改
git diff --check         # 检查空白错误
~~~

默认工作流不要求用户先执行 git add。如果用户已经有自己的暂存内容，Agent 不得擅自取消暂存或覆盖它。

### Codex Review

可以点击 Review，或在 Codex/IDE 扩展中输入 /review：

- Last turn：只看 Codex 最近一轮修改。
- Review uncommitted changes：看当前仓库所有未提交内容，包括 staged、unstaged 和 untracked 文件。
- Review against a base branch：看当前分支相对基准分支的变化。
- Review a commit：审查已经创建的某个提交。

单人开发时，优先使用 Last turn 或 Review uncommitted changes。如果用户审查后要求修正，Agent 继续修改和测试，不提交。

## 4. 用户授权提交后的步骤

只有收到明确的“commit/提交”后，才执行以下流程。先重新确认工作树，因为用户可能在审查期间又修改了文件：

~~~bash
git status --short
git diff --stat
git diff --check
~~~

确认本次任务所属文件后，优先逐个暂存，不使用无差别的 git add .：

~~~bash
git add -- path/to/file1 path/to/file2
git diff --cached --name-only
git diff --cached --stat
git diff --cached --check
~~~

确认暂存 diff 只包含用户授权的内容后创建本地提交：

~~~bash
git commit -m "实现 Core Runtime 第一阶段"
git status --short
git log -1 --oneline
~~~

提交信息使用中文，描述本次可验证的行为或设计变化。提交完成后，Agent 汇报提交 hash、提交标题、包含的文件以及工作树是否干净。

“提交”默认只表示本地 git commit。只有用户另外说“推送/push/提交到远端”时，才执行 git push。

## 5. 常见判断

默认测试包含 `127.0.0.1` 临时端口上的 TCP 集成门禁，不访问外部网络。若沙箱报告 `bind: operation not permitted`，应在允许本机回环监听的测试环境运行并记录验证结果；不要将权限限制误判为代码故障，也不要静默跳过该门禁或用扩大 timeout 代替排查。

| 用户意图 | Agent 行为 |
| --- | --- |
| “改一下” | 修改并做最小验证，不提交 |
| “审核/审查一下” | 展示或解释 diff，不提交 |
| “这里有问题，修正” | 修改、测试、等待再次审查，不提交 |
| “commit/提交” | 复核工作树，暂存授权内容并创建本地提交 |
| “push/推送到远端” | 只有在已有提交且用户明确授权后才推送 |

如果文件归属、提交范围或用户改动无法明确区分，Agent 不猜测，先报告冲突并请求确认。
