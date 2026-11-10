# skill233-video-script

面向 **Codex 与 DeepSeek Harness（DSH）** 的原生 Agent Skill。借鉴 DreamScript 对题材、核心梗、人设与故事结构的专项组织方式，支持原创短剧、小说改编、剧本诊断及超长篇连载。

由宿主 Agent 写作；本项目提供按需加载的写作方法、可扩展策略和 Go 1.27 连续性工具，不是新的训练模型，不依赖 DreamScript 私有接口。

## 能力

- **28 个内置策略**：18 类题材、4 类结构、6 类风格。爱情、悬疑、复仇、职场、家庭、古装、武侠、仙侠、奇幻、科幻、末世、喜剧、惊悚、青春、竞技、战争、儿童、文学；短剧/小说使用不同节奏。
- **开放组合与继承**：JSON 扩展题材或文风，多父继承、参数显式消歧、硬冲突和循环检测。自定义策略放作品目录，不改内置资源。
- **持续叙事账本**：稳定人物 ID 与别名、定向关系、生效时间、锁定事实、状态版本、证据出处、角色知情、事件因果、伏笔回收。
- **逐单元交付**：检索 → 写作 → 断言提取 → 机械与语义复核 → 接受正文 → 更新交接；换会话从文件恢复。
- **可复现验证**：离线单元与 CLI 测试、1000 单元冷恢复测试，以及可选的 Codex / DSH 真实跨会话创作验收。

## 安装与使用

需要宿主 Agent 的文件/命令行能力。确定性辅助工具使用 **Go 1.27.0+**，仅标准库，无数据库、API key 或后台服务。

```text
gh repo clone neko233-com/skill233-video-script
```

进入克隆目录，在 PowerShell 安装至两端可发现的用户共享位置：

```powershell
$skillTarget = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.agents/skills/skill233-video-script'
go run ./cmd/skill-install --source . --target $skillTarget
```

也可用 `--target /absolute/project/.agents/skills/skill233-video-script` 安装到创作项目；已有目录拒绝覆盖。完整 [宿主适配与测试](references/hosts.md)。

调用示例：

> 使用 $skill233-video-script，创作一个家庭悬疑短剧：失联十年的姐姐突然回来，却不知道家里一直为她留着另一副碗筷。先建立核心关系与结构，再写前三集。

> 使用 $skill233-video-script，续写这部长篇小说第201–203章。采用克制的第一人称，先恢复关系、伏笔和知情，再逐章落盘校验。

> 使用 $skill233-video-script，加载我在作品 strategies 目录扩展的赛博悬疑策略，把这段小说改编成两集短剧，保留原文映射。

技能入口：[SKILL.md](SKILL.md)；[策略扩展](references/strategies.md)；[账本数据与命令](references/continuity.md)；[创作及诊断方法](references/writing.md)。

## 独立使用辅助工具

在仓库内构建；Windows 将输出名改为 `bin/story.exe`：

```text
go build -o bin/story ./cmd/story
```

用构建的 `story` 执行 `--help`。可按 [两集示例](examples/serial/brief.md) 初始化项目、接受 seed、组合提示词、检索设定并接受第一集。辅助工具本身**不生成正文**。

```text
story compose --dir strategies --dir examples/strategies --ids genre.cyber-mystery,structure.novel-serial,style.sparse-poetic
```

## 验证与边界

```text
go test ./...
go vet ./...
go run ./cmd/skill-eval --host all
```

最后一条会启动真实宿主并使用模型配额，需要本机可用且已认证的 `codex` / `dsh`；离线 CI 不启动它。实际执行证据见 [VALIDATION.md](VALIDATION.md)。

账本校验结构化断言，不自动理解自然语言，也不能保证模型提取没有遗漏、作品必然爆款或任意长度全文绝无矛盾。百万字测试使用合成样本，验证文件恢复与上下文预算；长篇审美、节奏和完整语义仍需要创作评审。元数据重放成本随作品增长；文件存储需要硬链接支持，哈希链用于意外修改检测。

设计参考：[Dreamina 官方产品发布](https://www.sina.cn/news/detail/5358269411628839.html)。本项目未使用其私有权重或训练数据，不作模型等价效果承诺。
