# 题材、结构、风格策略

## 选择与组合

策略是声明式 JSON 提示词，不执行代码、不注册工具、不切换模型。18 个题材是起点，开放扩展支持用户的混合题材、地方语言、非典型节奏和自定义风格。

先 `story strategies --dir <SKILL_ROOT>/strategies` 查看已校验的注册表，再组合：

```text
story compose --dir /skill/strategies --ids genre.romance,genre.workplace,structure.short-drama,style.cinematic
story compose --dir /skill/strategies --ids genre.literary,structure.novel-serial,style.restrained
story compose --dir /skill/strategies --dir /story/strategies --ids genre.cyber-mystery,structure.novel-serial,style.sparse-poetic
```

输出提示词应保存到作品目录的 `active-strategy.md`，在 brief/handoff 记录选用 ID。策略变化不自动改变旧正文和正史。参数 `medium` 应与 project.format 一致；Agent 负责检查，compose 不读取项目。

## 扩展文件

在用户作品 `strategies/` 下新增任意 `.json` 文件，根是策略数组。重复 `--dir` 同时加载内置与自定义目录；目录递归扫描，重复 ID 拒绝，不覆盖内置文件。完整示例见 [custom.json](../examples/strategies/custom.json)。

```json
[
  {
    "id": "genre.local-mystery",
    "kind": "genre",
    "title": "地方人情悬疑",
    "extends": ["genre.suspense"],
    "settings": {"social-scale": "small-town"},
    "prompts": ["让熟人社会的利益与人情成为证据流转的阻力；地方细节服务人物行动。"],
    "checks": ["地方习俗是否有来源或被明确作为虚构，是否被当成刻板标签？"]
  }
]
```

字段：`id`（以 `genre.` / `structure.` / `style.` 对应 kind 开头）、`kind`、`title`、非空 `prompts` 和 `checks` 数组；可选 `extends`、`conflicts`（策略 ID 数组）、`settings`（字符串键值）。settings 键遵循 ID 字符规则。扩展不限于已有题材，也可从零创建策略。

## 多策略优先级与冲突

1. 用户明确要求、作品不可变正史及已接受因果优先于所有策略。
2. 继承只允许同 kind；父策略先进入，子策略追加。共享祖先的同一提示只保留一次。缺失父 ID、循环继承、未知冲突引用和重复 ID 在加载时失败。
3. 子策略自己的 settings 可以**显式**解决其父策略之间的同名参数差异；检查项与提示词保留，子提示要说明如何细化父规则。
4. 选择多个独立策略时，同名参数的不同值直接报错。例如 short-drama 与 novel 的 medium、第一人称与外部视角、克制与高密度抒情。不能依赖传入顺序覆盖。选择兼容项，或创建一个继承父项并声明解决参数的新策略，只选择该子策略。
5. `conflicts` 是对称效果：任一侧声明即禁止同时出现在完整继承链。硬性不兼容不能通过子 settings 绕过，应调整策略选择。

未编码为 settings/conflicts 的语义矛盾仍需 Agent 解决。例如“每章悬念”与“生活流余韵”不一定冲突，应依作品目标解释；解释不了就向用户说明实际取舍。不能声称机器能检测任意提示词矛盾。

## 题材变奏与策略验证

每个题材同时提供核心机制、持续冲突来源、应追踪的领域状态和定向诊断。例如悬疑追踪证据链，修真追踪境界/资源/代价，家庭追踪亲缘/照护/承诺，竞技追踪赛程/能力/伤病。

改编、原创、诊断共用机制，但产物不同：原创落实到正文；改编增加原文映射；诊断输出位置、问题、修法与影响范围。风格扩展优先描述可观察特征、节奏、视角、对白习惯与反例，不用特定作者姓名代替具体规则。

新增策略后执行 `strategies` 和至少一个实际 `compose`，再选一段场景试写并用该策略 checks 检查；只通过 JSON 校验不代表风格已经生效。
