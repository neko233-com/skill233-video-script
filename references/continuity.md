# 连续性账本与命令

## 文件与真相层次

作品目录保存 `brief.md`（创作约束）、`bible.md`（世界与人物）、`outline.md`（当前卷/季计划）、`handoff.md`（断点）、`drafts/`（工作稿）以及工具创建的 `project.json`、`records/`、`blobs/`、`.story-cache/`。

- `records/00000001.json` 起是逐单元接受记录。revision 是提交序号；unit 0 为初始正史，此后 unit 1、2…为集/章序号。
- `blobs/<sha256>.md` 保存与记录绑定的完整正文。工作稿可以修改；已接受 blob 和记录不可手工改写。正文先写 blob，记录最后发布；中断可能留下未引用 blob，不会冒充成功提交。
- `.story-cache/` 是作品本地可重建的索引与校验快照，可在作品仓库的 `.gitignore` 中忽略；多个项目不共用缓存。缺失、落后或损坏时会回放权威记录重建；不能把缓存当成正文备份。
- 哈希链包含项目元数据和所有记录，用于发现意外修改，不是对有权重算整条链的人提供签名认证。
- 工具依赖文件系统硬链接，支持本地 NTFS/APFS/ext4；不支持时直接失败。写入锁 `.story.lock` 不自动过期，崩溃后确认无活动写入者再手工移除。提供进程中断完整性，不承诺断电或网络文件系统的目录持久性。
- 正史记录客观事实；角色获知通过 knowledge 指向**具体事实版本**。误信记录为角色自身的 `belief.*` 事实，不能把错误推断直接登记为客观真相。读者何时知道、叙述者的可靠性在 bible/outline 中单列。
- 未来剧情、候选方向放 outline，不预先写成已发生事件。秘密真相可在 unit 0 登记，角色不会自动知晓。

## 世界时间与状态变化

世界时间使用非负整数 tick，单位由 bible 约定（小时、天或场景时刻）。`unit` 是叙事顺序，`from/since/at` 是故事世界时间，允许后写早先发生的场景。精确日期、距离与行程仍由 Agent 人工核对。

事实与关系采用左闭右开区间 `[from, until)` / `[since, until)`；省略 until 表示持续有效。相同 subject/key 不允许区间重叠。状态改变用新 ID 和 `supersedes` 指向旧版本，工具将旧版本截止于新起点；locked 事实不能覆盖。`key` / `kind` 使用项目固定词表，避免“所在地”和“位置”绕开冲突检测。

关系 `from → to` 有方向，双向关系需要两条边或在 bible 中约定查询时双向解释。关系类型不同可以并存；互斥关系（敌对/同盟、已婚/离异等）优先采用同一个事实 key 的不同 value 表示，工具不会自动理解词义互斥。

## 使用与基本流程

使用发布包内预编译工具，不需要 Go。`story` 指 SKILL_ROOT 的 `bin/story`；Windows 是 `bin/story.exe`，使用绝对路径调用：

```text
story init --project /work/my-story --title 七号怀表 --format short-drama
story apply --project /work/my-story --batch /work/my-story/seed.json
story compose --dir /path/to/skill/strategies --ids genre.suspense,structure.short-drama,style.restrained
story context --project /work/my-story --entities c.lin,c.shen,i.watch --at 10 --depth 2 --knowledge-scope related --max-bytes 24000
story check --project /work/my-story --batch /work/my-story/unit-001.json --draft /work/my-story/drafts/001.md
story apply --project /work/my-story --batch /work/my-story/unit-001.json --draft /work/my-story/drafts/001.md
story verify --project /work/my-story
story graph --project /work/my-story --at 10
```

PowerShell 调用绝对路径时使用 `&`。目录和参数含空格时正常引用。只支持 `--help` 中当前存在的参数。辅助工具无网络、无模型调用；编译时使用 Go 1.27 和嵌入式 bbolt，用户不需要安装 Go 或数据库服务。

unit 0 的完整实例见 [seed.json](../examples/serial/seed.json)，首集增量见 [unit-001.json](../examples/serial/unit-001.json)。每次 `base_revision` 必须与当前账本一致。`check` 不写入，`apply` 在锁内重新检查；同一 revision 的并发写入最多成功一次。

## JSON 对象约定

所有 ID 全局唯一，使用小写 ASCII 字母开头，后接字母、数字、点、下划线或连字符，最长 96 字符。实体名字和内容可用任意语言。必填文本不能为空。

| 对象 | 字段与用途 |
| --- | --- |
| batch | `base_revision`, `unit`, `summary`；以下数组按需增加。unit > 0 必须有至少一项 claims 和非空正文 |
| entities | `id`, `kind`（character/location/organization/item/world）, `name`, `description`, 可选 `aliases`；新增实体，人物发展用事实表示 |
| facts | `id`, `subject`, `key`, `value`, `from`, `evidence`；可选 `until`, `locked`, `supersedes` |
| relations | `id`, `from`, `to`, `kind`, `since`, `evidence`；可选 `until`, `supersedes` |
| knowledge | `id`, `character`, `fact_id`, `at`, `evidence`；获知不会自动随事实版本变化 |
| events | `id`, `at`, `participants`, `summary`, `evidence`；可选 `causes`（事件 ID）, `requires`（当时有效的事实 ID）；因果不可成环、不可引用未来原因 |
| hooks | `id`, `question`, `plant_unit`（当前 unit）, `due_unit`, `entities`, `status`（open）, `evidence` |
| hook_updates | `id`, `status`（paid/retired）, `evidence`；retired 还需 `reason`；已关闭伏笔不能重复关闭 |
| claims | `subject`, `key`, `value`, `at`, `evidence`；可选 `knower`。要求对应事实当时有效；有 knower 时必须在此时间之前获知该版本 |

工具拒绝未知 JSON 字段；接受 UTF-8 与 UTF-8 BOM。字段零值遵循 Go JSON 约定，编写时仍应显式提供必填数字。`evidence` 引用实际场次/段落；不能用“已校验”代替出处。

## 检索、恢复与改稿

`context` 通过作品内 B+tree 索引按所选实体检索关系邻域、各属性在指定时间的有效版本、world 全局 locked 规则、所选实体的 locked 设定、近期事件、伏笔和近期摘要。`--knowledge-scope all` 保留所选角色全部已登记知情；`--knowledge-scope related` 按所选实体和全局世界实体过滤，返回 `knowledge_scope` 与 `knowledge_subjects` 标明覆盖范围。未列出的其他实体信息不能推断为角色不知道，相关时要把该实体加入 `--entities` 再查。校验仍使用完整账本状态。

预算按 UTF-8 **字节**计算，不是模型 token。可裁减事件和较早摘要并报告 `omitted_optional_count`；索引每个相关实体默认最多取 32 条近期事件，`more_events` 表示需要查更早记录。相关硬设定与该知情范围内的必要记录不会静默丢弃；超预算会报错。最近正文通过 `recent.draft.sha256` 定位到 blobs，按需读取。

关系邻域不是全书语义检索：`boundary_relations`、未返回的其他人物和更早历史需要按记录 ID 补读。非当前视角的作者秘密也可能出现，Agent 必须继续应用知情边界。日常查询只读取项目元数据、最新记录和相关索引条目；`check` / `apply` 加载索引中的全量元数据快照，其成本仍随正史规模增长，正文不会因此进入 AI 上下文。首次建索引、缓存损坏恢复和 `verify` 需要回放记录；`verify` 还逐个检查正文哈希。

索引只核对项目元数据和最新记录，不监控对旧记录的外部篡改。手工改过历史记录时先执行完整 `verify`，发现错误后停止续写；不要声称热索引能自动发现所有历史改动。缓存更新失败时 `apply` 返回已接受的 revision 与警告，后续查询会重建或回退，不应重复提交该单元。

旧断言和事件前置条件会在新状态中重新检查。倒叙可以增加信息，但不能悄悄使已接受正文失效。需要真正 retcon 时复制作品目录为新分支，保留受影响前的记录前缀与元数据，再从下一 revision 重建；在新分支重写相关集/章并 verify，原分支保留。没有自动 retcon/删除/强制覆盖命令。

机械校验覆盖引用、时序、状态区间、精确值、因果与知情条件；无法从自然语言推导事实、检查谎言动机、空间物理或保证所有断言都被提取。每单元需人工语义复核。伏笔到期是警告，不能自动替作者决定回收。
