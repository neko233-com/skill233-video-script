# DSH 与 Codex 原生适配

## 安装一个完整 skill 目录

文件夹安装名使用 `skill233-video-script`。两端共用 `SKILL.md`、references、策略与对应平台的原生程序；`agents/openai.yaml` 是 Codex 的可选展示元数据，DSH 无需解析它。最终用户无需安装 Go、数据库服务、插件或后台进程。

通用项目安装路径：`<创作项目>/.agents/skills/skill233-video-script/`。Codex 与已验证的 DSH 都支持该发现位置。用户级共享目录：`~/.agents/skills/skill233-video-script/`；DSH 也支持 `$DSH_HOME/skills/skill233-video-script/`（默认 `~/.dsh/skills/`）。不要同时安装同名多份，以免发现重复。

从 [GitHub Releases](https://github.com/neko233-com/skill233-video-script/releases/latest) 下载匹配操作系统和 CPU 的 ZIP；提供 Windows、Linux、macOS 的 amd64 和 arm64 包，并可用发布页的 `SHA256SUMS` 校验。保留 ZIP 中整个 `skill233-video-script` 目录。目标目录已存在时不要直接覆盖，先备份并迁移自定义内容。

Windows PowerShell 用户级共享安装，先将示例文件名替换为下载文件的实际路径：

```powershell
$skillsDir = Join-Path ([Environment]::GetFolderPath('UserProfile')) '.agents/skills'
New-Item -ItemType Directory -Force -Path $skillsDir | Out-Null
Expand-Archive -LiteralPath '已下载的发布包.zip' -DestinationPath $skillsDir
```

Linux/macOS 可用系统解压工具将压缩包解到 `~/.agents/skills/`；如解压后无执行权限，运行 `chmod +x ~/.agents/skills/skill233-video-script/bin/story`。项目级安装则解压到 `<创作项目>/.agents/skills/`。Windows 可用 `Get-FileHash -Algorithm SHA256`、Linux 用 `sha256sum`、macOS 用 `shasum -a 256` 与 `SHA256SUMS` 对照。安装不修改宿主认证；作品级 `.story-cache/` 由程序按需创建，正史和用户策略仍在各自作品目录。

## 调用方式

Codex：`使用 $skill233-video-script，写一个职场悬疑短剧，先立项再写前三集。`

DSH：`使用 skill233-video-script，按已有设定继续第12章，保持人物关系和角色知情一致。` 在已验证的 headless 中 `$skill233-video-script` 也可明确指向该技能。不同版本的 UI 列表展示方式可能不同，以实际发现与文件读取证据为准。

隐式触发通过 skill description 匹配；不通过泛化描述侵入普通文案任务。Codex 元数据允许自动选择。真实验收主要验证明确调用和工作流，不能代替大样本隐式路由评测。

路径总是相对于 SKILL_ROOT 或作品目录解析；不依赖宿主专用工具命名。宿主已有 shell/读写能力即可执行。原生程序使用 bbolt 本地索引，发布包包含第三方许可证，无需用户安装 Go。

## 开发者可重复自动化

以下命令仅供维护者运行，需要 Go 1.27 和相应宿主；普通使用者不需要 Go。

```text
go test ./...
go vet ./...
go run ./cmd/skill-eval --host codex
go run ./cmd/skill-eval --host dsh
go run ./cmd/skill-eval --host all --out /absolute/new-evaluation-directory
```

离线测试不调用模型；live eval 使用本机已认证 CLI 的当前配置，消耗对应模型配额，未安装或认证失败会报失败，不会降级成模拟通过。测试使用新的隔离目录和项目级 `.agents/skills`，不修改全局技能与认证。每阶段默认超时 15 分钟，可用 `--timeout 20m` 调整。

每宿主有两个独立的新会话：已接受 unit 0 → 写第1集并接受 → 全新进程从文件恢复写第2集。runner 校验宿主读 skill 的日志迹象、实际正文和策略文件、工作稿与接受正文的哈希绑定、交接中的 accepted revision/next unit 数字、hash verify、unit/revision、不可变身份、保管关系、角色获知时间、带 knower 的断言与伏笔回收。skill 发现证据是宿主 trace 中技能名与 SKILL.md 的读取迹象；不等于对宿主内部路由实现的证明。

测试产物保留在 stdout 所示目录，报告为 `report.json`，正文绑定在 `work/blobs/`；不保存完整宿主 trace、环境或凭据。离线合成长文本测试验证 1000 单元、超过百万汉字样本的冷恢复、正文哈希与受限上下文，**不验证百万字小说的文学质量或无遗漏的语义一致性**。

来源：[Codex 官方 skill 文档](https://learn.chatgpt.com/docs/build-skills)；DSH 位置与 headless 参数以本机已安装 DeepSeek Harness 的技能发现文档和 `dsh headless --help` 为依据，验收记录见仓库 [VALIDATION.md](../VALIDATION.md)。
