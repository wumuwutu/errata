# 指纹评测（Fingerprint Evaluation）

对应开发文档 §6.4。目标：用语料实验回答两个问题——

1. 海明阈值取多少？（precision/recall 曲线）
2. 每条归一化规则各贡献多少？（消融对比）

## 语料格式

`eval/corpus.jsonl`，JSONL，每行一条：

```json
{"raw": "<原始 stderr，换行用 \\n>", "language": "python|node", "group": "<同错组 id>"}
```

- `raw`：完整的 stderr 文本（traceback / 堆栈原样保留，包括路径、行号）。
- `language`：标注语言。`python` / `node` 走精确提取器；其余语言的错误若含明确错误标记（`Exception in thread`、`error:`、`panic:`、`command not found` 等）会由保底提取器以 `unknown` 记录。
- `group`：**人工标注**的"同错组"。判断标准：两条报错如果是"同一个问题、换台机器/换个时间还会认成同一个"，就同组。路径、行号、PID、时间戳、引号内的值不同 → 仍然同组；异常类型或消息模板不同 → 不同组。

## 标注方法

1. 来源：自己的终端 history、公开 issue、同事贡献（见开发文档 §6.4）。
2. 每条起一个语义化 group id（如 `py-module-requests`），同错的复用同一 id。
3. 目标规模 300–500 条；当前 `eval/corpus.jsonl` 是 18 条的示例骨架。
4. 红线：**precision 优先**。拿不准是否同错时，分成不同组（错分同组会抬高 FP、把阈值逼向保守）。

## 运行

```sh
go run ./cmd/err-eval                          # 全量规则，阈值 0..10
go run ./cmd/err-eval -disable val,num         # 消融：关掉引号值/数字规则
go run ./cmd/err-eval -corpus my-corpus.jsonl -max-threshold 20
```

输出是每个海明阈值下的两两判定指标（TP/FP/FN、precision、recall、F1）：

- 预测：两条语料的指纹海明距离 ≤ 阈值 → 判同错。
- 真实：`group` 相同 → 同错。
- 无法识别签名的语料永不匹配任何条目，其同组对计入 FN（漏报惩罚 recall）。

选阈值的原则（§6.3 宁可漏报不可错报）：**取 precision = 1.000 的最大阈值**。

## 单元测试

`internal/eval` 的测试除覆盖评测逻辑外，还包含 `TestSampleCorpusSanity`：
示例语料在全规则、阈值 0 下必须 FP=0、FN=0——往语料里加条目时如果破坏了
这个不变量，说明新条目的归一化行为需要检查。
