# 七号怀表

- 媒介：连续悬疑短剧，本次验证只写两集，每集 300–600 字。
- 策略：genre.suspense + structure.short-drama + style.restrained。
- 人物：c.lin 林岚，修复师；c.shen 沈砚，调查员；i.watch 旧怀表。
- 初始正史见 seed.json，已接受 unit 0；从 revision 1 继续，不重新初始化。
- 第1集世界时间 tick 10：林岚将怀表交给沈砚保管；保持 f.name 和 f.secret 不变；此时沈砚未检查表壳，不知道编号。
- 第2集世界时间 tick 20：沈砚亲自打开表壳后才获知 f.secret；给编号一个有依据的局部答案并回收 h.number。怀表继续由沈砚保管。
- 字段词表：identity 为身份；owner 的 value 使用保管者实体 ID；secret 为表壳内客观秘密。
