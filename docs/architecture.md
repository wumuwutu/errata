# errata (err) 架构说明

> 面向想自己改代码的项目作者。行号以 v0.1.2 为准，函数名比行号更可靠——漂移时以函数名为准。
> 设计意图与红线在 `docs/dev-guide.md`；本文只描述"代码现在长什么样"。

## 目录树（按架构分层）

```
入口
├── cmd/err/main.go            主二进制入口（只调 cli.Execute()）
└── cmd/err-eval/main.go       指纹评测独立工具（不污染主 CLI）

internal/
├── capture/                   捕获层 A：err run 的 PTY 执行与现场记录
│   ├── run.go                 PTY 执行器：stdout 走 PTY、stderr tee 旁路录制、退出码透传
│   └── context.go             现场捕获 SceneFor()：命令行、cwd、git HEAD、运行时版本、OS
│
├── watch（见 cli/watch.go）    捕获层 C：日志流监听（stdin 管道 / tail 文件），
│                              无退出码语义，识别出错误即记录
│
├── hooks/                     捕获层 B：shell hook（无感捕获）
│   ├── scripts/errata.zsh     preexec/precmd + stderr tee 分流 + 命令边界哨兵；
│   │                          成功路径由 __errata_failed_at 时间窗（SECONDS）门控；
│   │                          preexec 追加命令日志 sess-$$.cmds（epoch<TAB>命令，
│   │                          内建 printf 零子进程，err fix 草稿数据源）
│   ├── scripts/errata.bash    DEBUG trap + PROMPT_COMMAND（兼容 bash 3.2）；
│   │                          成功路径门控逻辑同 zsh；命令日志同 zsh
│   │                          （bash 3.2 无 EPOCHSECONDS/printf %T，退化为
│   │                          每条命令一次 date fork，仅该平台）
│   ├── hooks.go               脚本嵌入（go:embed）、rc 写入/精准移除
│   ├── sessions.go            CleanStaleSessions：清理 7 天前的 session 缓冲
│   │                          （sess-* 前缀同时覆盖 .err/.fifo/.cmds）；
│   │                          SessionsDir/CommandsLogPath（与脚本里的
│   │                          __errata_dir/sess-$$.cmds 保持同步）
│   └── *_test.go              真实 shell 集成测试 + PTY 端到端测试
│
├── fingerprint/               指纹层：同一错误 → 同一指纹
│   ├── fingerprint.go         管线入口 Fingerprint()：签名 → hex 指纹
│   ├── normalize.go           ANSI 剥离 + 归一化规则（uuid/ts/ip/addr/path/val/num）
│   ├── signature.go           语言注册表：registry 字面量显式定序——
│   │                          python, node, java, go, c，generic 代码中垫底
│   │                          （init() 按文件名字典序执行，探针顺序不能交给字母表）；
│   │                          python/node 精确提取器 + Extractor/Register
│   ├── java.go / go.go / c.go Java（Exception in thread 的类名+消息模板；无消息
│   │                          异常用栈顶帧）、Go（panic 行 / file.go:l:c 编译错误）、
│   │                          C（gcc/clang 的 file:l:c error，含 clang fatal error）
│   │                          精确提取器
│   ├── generic.go             unknown 保底提取器：只信明确错误标记 + shell 内建
│   │                          错误结构（shell: builtin: 消息，语言无关；操作数
│   │                          置为 <ARG>），注册表显式垫底，永不抢精确识别
│   └── simhash.go             自实现 64 位 SimHash（token 含 CJK 表意文字，
│                              本地化消息不失分辨力）+ 海明距离（相似阈值 6）
│
├── redact/redact.go           脱敏层（§9 隐私红线）：stderr 入库/取签名前过一遍——
│                              URL 内嵌凭证、key=value 密钥（password/token/secret/
│                              api_key/authorization 等）、知名 token 前缀
│                              （ghp_/gho_/github_pat_/sk-/AKIA…/xox…）、JWT，
│                              统一掩码为 ***；规则保守且集中在这一个文件
│
├── match/match.go             匹配层：Matcher 接口（Exact/Similar）+ SimHash 实现
│
├── store/                     存储层
│   ├── store.go               SQLite 存取（WAL + busy_timeout）+ DeleteError/ClearAll
│   └── migrate.go             schema_version + 有序迁移（只增不改，升级无损）；
│                              迁移 3：errors 重建为 AUTOINCREMENT（删除后 id 不复用）
│
├── cli/                       命令层：cobra，基本每个文件一个命令
│   ├── record.go              ★ 失败主管线 recordFailure（run/hook/watch 三条
│   │                          捕获路共用；写入前先过 redact 脱敏）
│   ├── draft.go               err fix 解法草稿（dev-guide §7.3 命令历史推断）：
│   │                          按 ERRATA_SESSION 定位本会话 sess-<pid>.cmds，
│   │                          取 last_seen 之后的命令，漏斗过滤（噪音查看类/
│   │                          err 自身/失败命令本身剔除；装包与环境变更 >
│   │                          同程序 > 其他；去重后至多 3 条 faint 编号候选），
│   │                          输入序号即采纳入库；err run/管道/跨会话静默无草稿
│   ├── solved.go              成功检测 solvedHint（同程序且同目标参数成功
│   │                          才提示"looks fixed"；precision 优先）
│   ├── hook_event.go          err hook-event（隐藏命令，hook 的回调入口，
│   │                          --seq 对应 hook 写入缓冲的 OSC 哨兵）
│   ├── watch.go               err watch：stdin 管道或 tail 文件（从末尾起，
│   │                          不回放历史）；行 → 块（空行/新错误标记切分，
│   │                          静默 500ms 兜底 flush）→ 喂主管线；流无退出码，
│   │                          识别出错误即记录（与 hook 路径的语义差异）
│   ├── export.go              err export：错误库导出 Markdown（按项目分组、
│   │                          组内时间正序；只读）
│   ├── root.go                根命令、版本号、启动时懒归档过期 pending
│   ├── confirm.go             破坏性命令的确认语义（delete 认 y，clear 只认完整 yes；
│   │                          确认提示亮红 ANSI 91）
│   └── run.go / fix.go / show.go / pending.go / list.go / stats.go /
│       history.go / ignore.go / init.go / doctor.go / delete.go / clear.go /
│       uninstall.go           同名用户命令；pending/list/history 默认只显示
│                              最近 20 条（--all 全量）
│
├── hint/hint.go               交互层：所有终端提示（--err-- 前缀，≤2 行，§7.6）
├── termx/termx.go             交互层：ANSI 调色板（改颜色只动这个文件）+
│                              runewidth 截断（CJK/emoji 不断字）+ ~/ 收缩 + TTY 判定
├── list/list.go               交互层：err list 的 bubbletea TUI（Update 为纯函数；
│                              光标窗口滚动只渲染可见行；w/s 导航、a/d 筛选、e 编辑）
├── config/config.go           viper 配置 + XDG 路径 + ignore 黑名单
└── eval/eval.go               评测：语料加载 + 两两判定 precision/recall/F1

eval/corpus.jsonl              指纹评测示例语料（格式见 docs/eval.md）
scripts/hook-it.{bash,zsh}     hook 集成测试脚本（参数：err 二进制路径）
```

## 数据流：两条捕获路径汇入同一主管线

```
A. err run python app.py
   cli/run.go: runWrapped()
     → capture/run.go:65 Run()            执行 + 旁路录制 stderr
     → 退出码 != 0 且 stderr 非空
     → cli/record.go:20 recordFailure()

B. shell hook（用户在 hooked shell 里跑任意命令）
   scripts/errata.{zsh,bash}
     preexec/DEBUG: 快照 stderr 缓冲偏移 + 命令行，并把该行追加到
       sess-$$.cmds（epoch<TAB>命令；err fix 草稿数据源），然后向 stderr
       写一个不可见 OSC 哨兵（\x1b]6973;errata;<seq>\a，终端静默忽略）。
       哨兵在 tee 管道里 FIFO 排在所有滞留字节之后——tee 异步落盘再慢
       （WSL 实测可达秒级），前一条命令的 stderr/prompt 回显/err 自身输出
       也永远不会越过哨兵进入本命令的增量（v0.1.5 修复命令错位）
     precmd/PROMPT_COMMAND: 读 $?，并无条件消费/清空 __errata_cmd
       （空行或 Ctrl-C 复用上一条的 $?，陈旧命令文本会造成错位归属）
       成功 → 仅当 __errata_failed_at 非空且距现在 ≤300s 才调用
              err hook-event --exit-code 0 --cwd $PWD --command C
              （本会话上次失败后的 5 分钟窗口，用 SECONDS 内建变量
              计时、零子进程，v0.1.12 起；窗口与 success_window_minutes
              默认值一致，shell 侧写死不读配置。窗口内每次成功都检查
              ——vim 存盘的成功不会吃掉修好重跑的提示，无关成功由
              solvedHint 的"同程序+同目标"拒掉；超窗直接跳过并清掉
              标志，回到零子进程。无失败会话的 prompt 路径零子进程：
              WSL 上 Defender 按文件大小扫描被执行的二进制，每次
              err 启动 wall-time 可达 1.5s，而每条命令的 prompt 都
              可能走到这里。代价：跨会话 pending（旧终端失败、新终端
              首条成功）不再触发 looks-fixed；err fix 手动路径不受影响）
              → cli/hook_event.go → cli/solved.go solvedHint()
              （同目录 5 分钟内有 pending，且成功命令与报错命令同程序
               （python3==python）且共享"目标参数"（剥掉程序名和 flag 后的
               非 flag token，如脚本名 demo7.py；两边都没有目标参数时退化
               为仅同程序）→ 灰色两行提醒，24h 内不重复；
               无关命令成功不提醒——两次用户实测纠偏，见 827be5c 与 v0.1.8）
       失败且 stderr 缓冲增长 → err hook-event --exit-code N --offset O \
              --seq S --stderr-file F --cwd D --command C
              → cli/hook_event.go → 读增量：从 offset 起读，取最后一个
              seq=S 哨兵之后的字节（找不到哨兵 = 还在 tee 管道里 →
              宁可漏报也不错位）；哨兵按序号匹配、忽略产品词负载，
              改名前的旧 hook 配新二进制不丢记录（v0.1.9）；
              seq=0 兼容更老的 无哨兵 hook（未重启 shell）
              → cli/record.go:20 recordFailure()

recordFailure(commandLine, dir, stderr, cfg, hintOut):
  1. ignore 黑名单 / err 自身命令 → 跳过
  2. redact/redact.go 脱敏：URL 内嵌凭证、key=value 密钥、知名 token
     前缀、JWT → ***（raw_sample、签名、命中提示永远只见脱敏后内容）
  3. fingerprint/fingerprint.go:9 Fingerprint()  → (lang, signature, hex)
     提取顺序（注册表字面量显式定序）：python → node → java → go → c
     → unknown（保底，generic.go）。
     签名空（无任何明确错误标记）→ 跳过（宁可漏报）
  4. store.Open()（migrate 自动升级 schema）
  5. cli/record.go:71 findHit(match.SimHash, fp) → 精确命中 or 相似降级
  6. store/store.go:147 UpsertError() → 新错误建记录+pending；旧错误 count++
  7. hint/hint.go Print() → ≤2 行灰色提示（见过 N 次/解法/相似错误），
     命令名（err fix/err show）青色（ANSI 36）、解法绿色（32）、
     looks-fixed 只带错误编号（err #N），ASCII 短横线

C. err watch（日志流监听）
   cli/watch.go: 无参读 stdin 管道（docker logs -f app 2>&1 | err watch，
   EOF 即退出），带参 tail 文件（Seek 到末尾，不回放历史，200ms 轮询）。
   行按块切分（空行/新错误标记；follow 模式下静默 500ms 强制 flush，
   否则日志里最后一条错误永远等不到终止符）→ 识别出签名即调
   recordFailure("watch: <源>", ...)。流式场景没有退出码：识别出错误
   文本就记录（与 hook 路径"非零退出码 + stderr 增长"的语义差异，
   README 已注明）。Ctrl-C（SIGINT/SIGTERM）优雅退出并打印捕获统计。
```

## SQLite schema（迁移 1–3，见 internal/store/migrate.go）

**errors** — 一条"错误档案"（§1.2 四要素）：

| 字段 | 来源 | 说明 |
|---|---|---|
| id | 自增 | 主键；迁移 3 起为 AUTOINCREMENT（删除后不复用旧 id） |
| fingerprint | 推导 | SimHash hex，UNIQUE；同错去重的键 |
| signature | 推导 | 归一化后的错误签名（如 `TypeError: ... <VAL>`） |
| raw_sample | 原始事实 | 最近一次出现的 stderr 原文（去 ANSI） |
| language | 推导 | python / node / java / go / c / unknown（提取器判定） |
| command / project_dir / git_commit / runtime / os | 原始事实 | 现场五要素（git/runtime 尽力而为，可空） |
| created_at / first_seen / last_seen | 原始事实 | 建条时间 / 首见 / 末见 |
| count | 推导 | 出现次数（upsert 时 +1） |

**fixes** — 用户确认的解法（产品灵魂）：

| 字段 | 说明 |
|---|---|
| error_id | 关联 errors.id |
| solution | 用户确认的解法（原始事实） |
| draft / commands_between / git_diff_ref | 预留字段；§7.3 草稿改为 err fix 时即时计算、不落库（v0.1.16 起命令历史推断已实现，LLM 压缩已砍） |
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

## 怎么加一门新语言（如 Rust）

注意分工：**不加任何文件，未知语言的错误也会被 generic.go 的保底提取器以
`unknown` 记录**（前提：输出里有明确错误标记）。只有当你想要更精确的签名
（提取异常类型+消息模板，而不是整行）时才需要：

1. 新建 `internal/fingerprint/rust.go`：实现
   `func rustSignature(text string, disabled map[string]bool) (string, bool)`
   （从 ANSI 剥离后的文本提取归一化签名；拿不准就返回 ok=false——precision 优先）。
2. 把它加进 `signature.go` 的 registry 字面量——位置即探针顺序，generic
   必须永远垫底。**不要用 init() + Register()**：init 按文件名字典序执行，
   探针顺序会变成字母表的意外（v0.1.14 起改为显式定序，signature_test.go
   的 TestRegistryOrderLocked 锁定该顺序）。
3. 加测试（仿 java/go/c 用例，含路径/行号漂移下同指纹的稳定性断言）；给
   `eval/corpus.jsonl` 加语料并跑 `go run ./cmd/err-eval` 确认阈值 0 下 FP=0。

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
