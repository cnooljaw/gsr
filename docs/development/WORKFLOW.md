# 开发、审查与提交

默认在当前分支工作。Agent 完成修改与验证，用户审查后决定是否提交；创建或切换分支、推送远端均需用户明确要求。

## 修改与验证

开始前检查 `git status --short`，保留用户已有的无关改动。只修改当前任务涉及的文件。

新行为先搜索已有用例，在相关验证层补充可失败的测试，再完成最小实现。Go 改动运行 gofmt；验证范围、时间预算与 race 使用规则统一见 [Go 编码规范：验证门禁](GO-CODE-STYLE.md#5-验证门禁)，本文件不重复维护另一套门禁。

纯文档整理只检查 diff、格式、路径及锚点；代码示例内容发生变化时才补编译或运行验证。完成后说明改动、实际执行的验证和未验证范围，工作树留待审查。

## 审查

在 VS Code 源代码管理或 Codex Review 中查看本轮 diff。命令行可使用：

```sh
git diff
git diff --cached
git diff HEAD
git diff --check
```

审查、审核或“看看 diff”不等于提交授权。需要修正时继续“修改 → 验证 → 审查”，不自动提交。不取消用户已有暂存内容。

## 提交

用户明确说“提交 / commit”后，检查并逐个暂存本次所属文件：

```sh
git status --short
git diff --check
git add -- path/to/file1 path/to/file2
git diff --cached --name-only
git diff --cached
git diff --cached --check
git commit -m "描述本次可验证变化的中文标题"
git status --short
git log -1 --oneline
```

归属不明时先确认，不代替用户决定。提交后报告 hash、内容及工作树状态。“提交”仅指本地 commit；只有另行明确授权才 push。不使用破坏性清理命令。

## 临时文件与资料归属

- 临时程序、profile、trace 和本地实验统一放根目录 `/_scratch/`。该目录被 Git 忽略，Go 的 `./...` 也跳过下划线开头的目录；仅写入 `.gitignore` 不能阻止 Go 扫描普通 `tmp/`。
- WebSocket Sproto 集成测试程序输出到 `/_scratch/sproto-echo`。可重建产物、日志、Node 依赖和工具缓存继续按 `.gitignore` 管理，不提交。
- 源码、测试、schema、使用者需要的生成代码、锁文件、许可证以及 TS 消费副本来源指纹保留跟踪。
- Sproto 权威编解码、通用基准和优化研究由独立 gosproto 仓库维护，GSR 仅维护接入与集成。旧实验资料不恢复到 GSR 的 Go 可扫描目录。
- 2026-10-23 的本机旧资料归档位于 `/Users/lijiawang/Documents/cocos/laya/archives/gsr-20261023/`，查找恢复资料时使用其中的 `MANIFEST.json`；它不是构建依赖。逐文件清理过程保留在 Git 历史。
- 保留 Go 构建与测试缓存，不以清缓存作为常规验证步骤，不擅自清理 `.git`、本地配置或工具恢复资料。

## 环境限制

默认 Go 测试包含本机回环 TCP。若沙箱报 `bind: operation not permitted`，应在允许回环监听的环境验证并说明限制，不能把权限限制误判为实现故障或静默跳过。测试超时先定位具体用例、goroutine 和外部依赖，不反复重试或延长时间掩盖问题。
