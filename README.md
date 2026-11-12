# skill233-video-script

面向 **Codex 与 DeepSeek Harness（DSH）** 的全局 Agent Skill。支持短剧、电影、B站长视频、抖音剧情短视频、商品叙事视频、各类剧情场景，以及长篇连载小说；可原创、改编、续写和诊断。

由宿主 Agent 写作；本项目提供按需加载的写作方法、可扩展策略和 Go 1.27 连续性工具，不是新的训练模型，不依赖 DreamScript 私有接口。

## 能力

- **33 个内置策略**：18 类题材、9 类结构、6 类风格。长篇连续 AI 剧、电影、B站、抖音、商品视频和小说各有创作与拍摄提示，可与多种题材、文风组合。
- **开放组合与继承**：JSON 扩展题材或文风，多父继承、参数显式消歧、硬冲突和循环检测。自定义策略放作品目录，不改内置资源。
- **持续叙事账本**：稳定人物 ID 与别名、定向关系、生效时间、锁定事实、状态版本、证据出处、角色知情、事件因果、伏笔回收。
- **全局工具、作品级索引**：各作品的 `.story-cache` 独立保存可重建的 B+tree 索引；按人物、关联、时间和主题范围检索，不把整部长篇放进 AI 上下文。`verify` 仍会完整校验权威记录和正文。
- **逐单元交付**：检索 → 写作 → 断言提取 → 机械与语义复核 → 接受正文 → 更新交接；换会话从文件恢复。
- **可复现验证**：离线单元与 CLI 测试、1000 单元冷恢复测试，以及可选的 Codex / DSH 真实跨会话创作验收。

## 安装与使用

用户只需下载与系统和架构匹配的 [发布包](https://github.com/neko233-com/skill233-video-script/releases/latest)，**无需安装 Go**。包内含可直接运行的 `bin/story`（Windows 为 `bin/story.exe`）、完整 Skill 和第三方许可；不需要数据库服务、API key 或后台进程。

从 Release 下载对应系统的 `skill233-video-script-<系统>-<架构>.zip`，核对同页 `SHA256SUMS` 后，将包内 `skill233-video-script` 文件夹解压到用户共享目录 `~/.agents/skills/`。例如 Windows PowerShell：

```powershell
$skillsDir = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.agents/skills'
New-Item -ItemType Directory -Force -Path $skillsDir | Out-Null
Expand-Archive -LiteralPath '已下载的发布包.zip' -DestinationPath $skillsDir
```

目标已存在时先备份并检查，避免覆盖本地修改。macOS / Linux 使用对应包并确保 `bin/story` 有执行权限。完整 [宿主适配与测试](references/hosts.md)。

调用示例：

> 使用 $skill233-video-script，创作一个家庭悬疑短剧：失联十年的姐姐突然回来，却不知道家里一直为她留着另一副碗筷。先建立核心关系与结构，再写前三集。

> 使用 $skill233-video-script，续写这部长篇小说第201–203章。采用克制的第一人称，先恢复关系、伏笔和知情，再逐章落盘校验。

> 使用 $skill233-video-script，加载我在作品 strategies 目录扩展的赛博悬疑策略，把这段小说改编成两集短剧，保留原文映射。

> 使用 $skill233-video-script，为一款可折叠露营桌写 45 秒抖音剧情商品视频；按实拍镜头、对白、字幕和道具交付，只使用我提供的产品参数。

> 使用 $skill233-video-script，把这个故事改写为 15 分钟 B站长视频的画面与旁白脚本，并标出可拍摄素材。

技能入口：[SKILL.md](SKILL.md)；[策略扩展](references/strategies.md)；[账本数据与命令](references/continuity.md)；[创作及诊断方法](references/writing.md)。

## 独立使用辅助工具

使用发布包内 `bin/story --help`（Windows 为 `bin/story.exe`）。可按 [两集示例](examples/serial/brief.md) 初始化项目、接受 seed、组合提示词、检索设定并接受第一集。辅助工具本身**不生成正文**。

```text
story compose --dir strategies --dir examples/strategies --ids genre.cyber-mystery,structure.novel-serial,style.sparse-poetic
```

## 开发验证与边界

以下命令只面向开发者；发布包用户不需要 Go。项目开发与构建使用 Go 1.27：

```text
go test ./...
go vet ./...
go run ./cmd/skill-eval --host all
```

最后一条会启动真实宿主并使用模型配额，需要本机可用且已认证的 `codex` / `dsh`；离线 CI 不启动它。实际执行证据见 [VALIDATION.md](VALIDATION.md)。

账本校验结构化断言，不自动理解自然语言，也不能保证模型提取没有遗漏、作品必然爆款或任意长度全文绝无矛盾。百万字测试使用合成样本；长篇审美、节奏和完整语义仍需要创作评审。缓存快照的加载与写入仍随项目元数据规模增长；首次建索引和完整 `verify` 需要回放历史。文件存储需要硬链接支持，哈希链用于意外修改检测。

设计参考：[Dreamina 官方产品发布](https://www.sina.cn/news/detail/5358269411628839.html)。本项目未使用其私有权重或训练数据，不作模型等价效果承诺。
