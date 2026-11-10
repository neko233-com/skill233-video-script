# DSH 与 Codex 原生适配

## 安装一个完整 skill 目录

本仓库根目录就是 skill，文件夹安装名使用 `skill233-video-script`。两端共用 `SKILL.md`、references、策略和 Go 源码；`agents/openai.yaml` 是 Codex 的可选展示元数据，DSH 无需解析它。无需插件、模型转发、后台进程或修改宿主配置。

通用项目安装路径：`<创作项目>/.agents/skills/skill233-video-script/`。Codex 与已验证的 DSH 都支持该发现位置。用户级共享目录：`~/.agents/skills/skill233-video-script/`；DSH 也支持 `$DSH_HOME/skills/skill233-video-script/`（默认 `~/.dsh/skills/`）。不要同时安装同名多份，以免发现重复。

在克隆仓库内用 Go 1.27 安装，`--target` 必须为绝对路径，已有目录会拒绝覆盖：

```text
go run ./cmd/skill-install --source . --target /absolute/project/.agents/skills/skill233-video-script
```

PowerShell 用户级共享安装（读取实际用户目录，不写死安装位置）：

```powershell
$skillTarget = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.agents/skills/skill233-video-script'
go run ./cmd/skill-install --source . --target $skillTarget
```

从 GitHub 下载时保留整个 skill，不能只复制 SKILL.md。安装器只复制执行所需资源；不会改写 AGENTS.md、宿主配置或模型认证。安装中断可能留下部分目录，先检查再处理；不会自动删除现有目录。升级时保留用户扩展策略在作品目录，用新的安装目录验证后再按用户授权切换。

## 调用方式

Codex：`使用 $skill233-video-script，写一个职场悬疑短剧，先立项再写前三集。`

DSH：`使用 skill233-video-script，按已有设定继续第12章，保持人物关系和角色知情一致。` 在已验证的 headless 中 `$skill233-video-script` 也可明确指向该技能。不同版本的 UI 列表展示方式可能不同，以实际发现与文件读取证据为准。

隐式触发通过 skill description 匹配；不通过泛化描述侵入普通文案任务。Codex 元数据允许自动选择。真实验收主要验证明确调用和工作流，不能代替大样本隐式路由评测。

路径总是相对于 SKILL_ROOT 或作品目录解析；不依赖宿主专用工具命名。宿主已有 shell/读写能力即可执行。新增程序只使用 Go 1.27 标准库；已有宿主 CLI 可能使用其他运行时，不属于本项目新代码。

## 可重复自动化

```text
go test ./...
go vet ./...
go run ./cmd/skill-eval --host codex
go run ./cmd/skill-eval --host dsh
go run ./cmd/skill-eval --host all --out /absolute/new-evaluation-directory
```

第三方依赖为零。离线测试不调用模型；live eval 使用本机已认证 CLI 的当前配置，消耗对应模型配额，未安装或认证失败会报失败，不会降级成模拟通过。测试使用新的隔离目录和项目级 `.agents/skills`，不修改全局技能与认证。每阶段默认超时 15 分钟，可用 `--timeout 20m` 调整。

每宿主有两个独立的新会话：已接受 unit 0 → 写第1集并接受 → 全新进程从文件恢复写第2集。runner 校验宿主读 skill 的日志迹象、实际正文和策略文件、工作稿与接受正文的哈希绑定、交接中的 accepted revision/next unit 数字、hash verify、unit/revision、不可变身份、保管关系、角色获知时间、带 knower 的断言与伏笔回收。skill 发现证据是宿主 trace 中技能名与 SKILL.md 的读取迹象；不等于对宿主内部路由实现的证明。

测试产物保留在 stdout 所示目录，报告为 `report.json`，正文绑定在 `work/blobs/`；不保存完整宿主 trace、环境或凭据。离线合成长文本测试验证 1000 单元、超过百万汉字样本的冷恢复、正文哈希与受限上下文，**不验证百万字小说的文学质量或无遗漏的语义一致性**。

来源：[Codex 官方 skill 文档](https://learn.chatgpt.com/docs/build-skills)；DSH 位置与 headless 参数以本机已安装 DeepSeek Harness 的技能发现文档和 `dsh headless --help` 为依据，验收记录见仓库 [VALIDATION.md](../VALIDATION.md)。
