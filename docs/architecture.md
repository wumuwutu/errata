# dejavu (err) 架构说明

> 面向想自己改代码的项目作者。行号以 v0.1.2 为准，函数名比行号更可靠——漂移时以函数名为准。
> 设计意图与红线在 `docs/dev-guide.md`；本文只描述"代码现在长什么样"。

## 目录树与每个文件的职责

```
cmd/err/main.go              二进制入口：只调 cli.Execute()（放 cmd/err 下，
                             go install 产出的二进制才叫 err）
cmd/err-eval/main.go         指纹评测独立入口（不污染主 CLI）

internal/cli/                cobra 命令层（每个文件一个命令）
  root.go                    根命令、版本号、启动时懒归档过期 pending
  run.go                     err run：PTY 包装执行（runWrapped）
  hook_event.go              err hook-event（隐藏）：shell hook 的回调入口；
                             --seq 对应 hook 写入缓冲的 OSC 哨兵
  record.go                  recordFailure：失败命令的"指纹→匹配→存储→提示"
                             主流程（run 和 hook 两条捕获路径共用）
  solved.go                  solvedHint：成功命令的"好像解决了？"提示
  fix.go                     err fix：无参时直接取最近一条 pending（两行摘要
                             含触发命令 + 立即输入 solution）
  show.go / pending.go / list.go / stats.go / history.go /
  ignore.go / init.go / doctor.go / uninstall.go
                             同名用户命令；*_test.go 为对应测试

internal/capture/
  run.go                     PTY 执行器 Run()：stdout 走 PTY、stderr 管道 tee
                             （透传红线）、stdin 按是否 TTY 分流、窗口尺寸中继
  context.go                 现场捕获 SceneFor()：命令行、cwd、git HEAD、
                             python/node --version、OS

internal/hooks/
  hooks.go                   hook 脚本嵌入（go:embed）、rc 写入/精准移除
  sessions.go                CleanStaleSessions：清理 7 天前的 session 缓冲
  scripts/dejavu.zsh         zsh hook：preexec/precmd + stderr tee 分流
                             + 命令边界哨兵
  scripts/dejavu.bash        bash(3.2+) hook：DEBUG trap + PROMPT_COMMAND
                             + 命令边界哨兵
  integration_test.go        驱动 scripts/hook-it.* 跑真实 shell 集成测试
  pty_e2e_test.go            真实 PTY 端到端：命令归属/vim 不提示/Ctrl-C
                             不误记/looks-fixed 两行提示

internal/fingerprint/
  normalize.go               ANSI 剥离 + 归一化规则（uuid/ts/ip/addr/path/val/num）
  signature.go               语言注册表（Extractor/Register）+ python/node 提取器
  generic.go                 保底提取器（unknown）：只信明确错误标记
                             （Exception in thread/panic:/fatal:/error:/shell 经典），
                             注册在注册表最后，永不抢 python/node 的识别
  simhash.go                 自实现 64 位 SimHash + 海明距离 + 相似阈值 6
  fingerprint.go             Fingerprint()：管线入口（签名→hex 指纹）

internal/match/match.go      Matcher 接口（Exact/Similar）+ SimHash 实现
internal/store/store.go      SQLite 存取（WAL + busy_timeout）
internal/store/migrate.go    schema_version 表 + 有序迁移（只增不改）
internal/hint/hint.go        命中提示 / 解决提示（--err-- 前缀，faint 灰底 +
                             命令名青色 + 关键词亮绿/亮白，≤2 行，§7.6）
internal/termx/termx.go      NO_COLOR 感知 ANSI 调色板 + runewidth 显示宽度
                             截断（CJK/emoji 不断字）+ ~/ 收缩 + TTY 判定
internal/config/config.go    viper 配置 + XDG 路径 + ignore 黑名单
internal/list/list.go        err list 的 bubbletea model（Update 为纯函数）
internal/eval/eval.go        语料加载 + 两两判定 precision/recall/F1
eval/corpus.jsonl            示例语料（格式见 docs/eval.md）
scripts/hook-it.{bash,zsh}   hook 集成测试脚本（参数：err 二进制路径）
```

## 数据流：两条捕获路径汇入同一主管线

```
A. err run python app.py
   cli/run.go: runWrapped()
     → capture/run.go:65 Run()            执行 + 旁路录制 stderr
     → 退出码 != 0 且 stderr 非空
     → cli/record.go:20 recordFailure()

B. shell hook（用户在 hooked shell 里跑任意命令）
   scripts/dejavu.{zsh,bash}
     preexec/DEBUG: 快照 stderr 缓冲偏移 + 命令行，然后向 stderr 写一个
       不可见 OSC 哨兵（\x1b]6973;dejavu;<seq>\a，终端静默忽略）。
       哨兵在 tee 管道里 FIFO 排在所有滞留字节之后——tee 异步落盘再慢
       （WSL 实测可达秒级），前一条命令的 stderr/prompt 回显/err 自身输出
       也永远不会越过哨兵进入本命令的增量（v0.1.5 修复命令错位）
     precmd/PROMPT_COMMAND: 读 $?，并无条件消费/清空 __dejavu_cmd
       （空行或 Ctrl-C 复用上一条的 $?，陈旧命令文本会造成错位归属）
       成功 → err hook-event --exit-code 0 --cwd $PWD --command C
              → cli/hook_event.go → cli/solved.go:21 solvedHint()
              （同目录 5 分钟内有 pending，且成功命令与报错命令是同一程序
               （python3==python）→ 灰色两行提醒，24h 内不重复；
               无关命令（如 ls）成功不提醒——用户实测纠偏，见 827be5c）
       失败且 stderr 缓冲增长 → err hook-event --exit-code N --offset O \
              --seq S --stderr-file F --cwd D --command C
              → cli/hook_event.go → 读增量：从 offset 起读，取最后一个
              seq=S 哨兵之后的字节（找不到哨兵 = 还在 tee 管道里 →
              宁可漏报也不错位）；seq=0 兼容旧版 hook（未重启 shell）
              → cli/record.go:20 recordFailure()

recordFailure(commandLine, dir, stderr, cfg, hintOut):
  1. ignore 黑名单 / err 自身命令 → 跳过
  2. fingerprint/fingerprint.go:9 Fingerprint()  → (lang, signature, hex)
     提取顺序：python（精确）→ node（精确）→ unknown（保底，generic.go）。
     签名空（无任何明确错误标记）→ 跳过（宁可漏报）
  3. store.Open()（migrate 自动升级 schema）
  4. cli/record.go:71 findHit(match.SimHash, fp) → 精确命中 or 相似降级
  5. store/store.go:147 UpsertError() → 新错误建记录+pending；旧错误 count++
  6. hint/hint.go Print() → ≤2 行灰色提示（见过 N 次/解法/相似错误），
     命令名（err fix/err show）青色（ANSI 36），ASCII 短横线
```

## SQLite schema（迁移 1 + 2，见 internal/store/migrate.go）

**errors** — 一条"错误档案"（§1.2 四要素）：

| 字段 | 来源 | 说明 |
|---|---|---|
| id | 自增 | 主键 |
| fingerprint | 推导 | SimHash hex，UNIQUE；同错去重的键 |
| signature | 推导 | 归一化后的错误签名（如 `TypeError: ... <VAL>`） |
| raw_sample | 原始事实 | 最近一次出现的 stderr 原文（去 ANSI） |
| language | 推导 | python / node / unknown（提取器判定） |
| command / project_dir / git_commit / runtime / os | 原始事实 | 现场五要素（git/runtime 尽力而为，可空） |
| created_at / first_seen / last_seen | 原始事实 | 建条时间 / 首见 / 末见 |
| count | 推导 | 出现次数（upsert 时 +1） |

**fixes** — 用户确认的解法（产品灵魂）：

| 字段 | 说明 |
|---|---|
| error_id | 关联 errors.id |
| solution | 用户确认的解法（原始事实） |
| draft / commands_between / git_diff_ref | 预留（§7.3 草稿机制，未实现） |
| created_at | 记录时间 |

**pending** — 状态机（§7.2）：

| 字段 | 说明 |
|---|---|
| error_id | 关联 errors.id |
| detected_at | 首次捕获时间 |
| status | pending / resolved（err fix 或 list 编辑解法）/ archived（超 archive_after_days，懒归档） |
| reminded_at | 迁移 2 新增；成功提示的 24h 去重标记 |

**errors_fts**（FTS5 虚表）：signature + solution 全文索引，insert/fix 时同步。
**schema_version**：单行版本表；`Open()` 时按序补齐迁移。

## 怎么加一门新语言（如 Java）

注意分工：**不加任何文件，未知语言的错误也会被 generic.go 的保底提取器以
`unknown` 记录**（前提：输出里有明确错误标记）。只有当你想要更精确的签名
（提取异常类型+消息模板，而不是整行）时才需要：

1. 新建 `internal/fingerprint/java.go`：实现
   `func javaSignature(text string, disabled map[string]bool) (string, bool)`
   （从 ANSI 剥离后的文本提取归一化签名；拿不准就返回 ok=false——precision 优先）。
2. 文件里 `func init() { Register("java", javaSignature) }`。
3. 加测试（仿 python/node 用例）；给 `eval/corpus.jsonl` 加语料并跑
   `go run ./cmd/err-eval` 确认阈值 0 下 FP=0。

## 怎么加一个新命令

1. `internal/cli/foo.go`：`var fooCmd = &cobra.Command{...}` + `func init() { rootCmd.AddCommand(fooCmd) }`。
2. 需要数据时经 `config.DBPath()` → `store.Open()`（迁移自动跑）。
3. 输出写 `cmd.OutOrStdout()`（可测试）；耗时/后台行为禁止出现在 prompt 路径（hook-event）。

## 测试怎么跑

```sh
go test ./...                    # 单测（含 store/fingerprint/eval/list 等）
go test ./internal/hooks/        # 真实 bash/zsh 集成测试（驱动 scripts/hook-it.*）；
                                 # 缺 shell 自动跳过，-short 也跳过
go run ./cmd/err-eval            # 指纹评测（docs/eval.md）
```
