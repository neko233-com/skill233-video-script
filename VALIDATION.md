# 验证说明

2026-11-10 实际执行记录。程序使用 Go 1.27.0 / Windows amd64；跨平台结果由 GitHub Actions 验证。没有运行或被阻塞的检查不视为通过。

| 验收项 | 结果 |
| --- | --- |
| skill-creator `quick_validate.py` | 通过，frontmatter 与 skill 结构合法 |
| `go test ./...`、`go vet ./...` | 通过，包含安装后独立构建与扩展策略调用 |
| Go 1.27 三平台 CI | Linux / Windows / macOS 的格式、vet、race tests、构建通过；具体提交见 [workflow runs](https://github.com/neko233-com/skill233-video-script/actions/workflows/test.yml) |
| 1000 单元、1000 历史人物、超过百万汉字合成样本 | 冷恢复与正文哈希通过；相关上下文不超过 6000 UTF-8 字节 |
| Codex CLI 0.163.0-alpha.10.2 原生实测 | 两个独立新会话续写两集均通过，均有读取 skill 的 trace 迹象；最终 unit 2 / revision 3 |
| DSH 0.2.2-rc.1 模型生成实测 | **阻塞**：headless 的 deepseek-official 路由返回 `MISSING_CREDENTIAL`；未伪装为通过 |

## 离线自动化

- `go test ./...`：真实两阶段账本恢复、CLI 参数到文件产物的完整流程、严格 JSON、状态变更、别名与区间冲突、知情泄漏、因果循环、前置失效、旧断言保护、正文与记录/元数据篡改、并发写入与 dry-run。
- 策略测试：全部内置与示例扩展逐项可解析、混合继承、显式参数消歧、媒介/视角/表达冲突、缺失依赖、循环与重复 ID；各题材与短剧/小说结构均可组合。
- 合成长文本：1000 个正文单元与 1000 个无关历史人物的不可变身份，每单元超过 1000 汉字，程序实际计数验证总量超过百万汉字；冷恢复 revision 1001/next unit 1001，逐正文哈希校验，context 不超过 6000 UTF-8 字节。不把无关人物身份塞入上下文，同时测试全局 world 规则不能丢失。此测试直接发布经同一 Apply 校验的记录，再冷读验证，不衡量 1000 次完整 CLI 提交的吞吐。
- `go vet ./...`；skill-creator 的 `quick_validate.py` 只验证 skill 结构。
- CI 使用 Go 1.27、Linux / Windows / macOS，执行 vet、race tests 与构建；CI 是否通过以具体 workflow run 为准。

## 真实宿主自动化

运行 `go run ./cmd/skill-eval --host codex --timeout 15m`：在隔离项目的原生 `.agents/skills` 安装相同 skill，两个独立新会话从已有 unit 0 续写两集。第一集接受 revision 2，沈砚保管怀表且不知道编号；第二集接受 revision 3，沈砚在 tick 20 检查表壳后才获知编号，h.number 回收且林岚身份与保管关系延续。两个会话均有 skill 读取迹象，`apply` / `verify` 通过；另外复核了两集工作稿与接受副本的 SHA-256 以及 handoff 的版本/下一单元数字。

自动化还覆盖工作稿在接受后被修改、交接版本落后两种失败，防止只检查文件存在就误报成功。首轮实测曾在 8 分钟限时内未完成交接；据此收紧常规续写的产物范围、强调断点以 records 为准，并增加交接验收。复跑的两阶段均完成；15 分钟是每阶段保护上限，不是生成耗时保证。

DSH 已核对原生发现目录和 headless 参数；实际模型请求因缺少凭据失败。需要在 DSH Models 中配置对应路由后再执行 `go run ./cmd/skill-eval --host dsh`。用户也可用 `--host all` 一并复跑两端；失败会保留为失败，不会降级模拟。报告只保存脱敏摘要，不保存原始宿主 trace、环境或凭据。

机器校验不能覆盖自然语言中的全部暗示和断言；真实短样本也不能证明百万字连载文学质量。隐式技能路由、完整长篇语义、实际观众留存与商业效果均不在本次自动化证明范围内。
