# 架构说明

```
JSON 描述文件
     │  loader（解码 + 结构校验：缺失引用/冲突/循环/非法版本）
     ▼
 schema.Document
     │  generator.plan（按服务分组、版本排序、compat 门禁）
     ▼
 buildPlan  ── 不兼容 ──▶ version_incompatible（渲染前中止）
     │  通过
     ▼
 render（确定性渲染：元数据 JSON + 类型化 Go 包 + 自包含运行时）
     ▼
 私有 staging 目录（os.MkdirTemp，.staging-*）
     │  writeStaging 全部成功
     ▼
 publish（逐文件 temp+rename 原子替换；旧文件先备份，失败回滚）
     ▼
 history.json 提交已接受版本 ──▶ manifest.json 最后落盘
```

## 包职责

- `schema`：内存类型模型；语义版本 `ParseVersion/Compare`。
- `loader`：`encoding/json`（`DisallowUnknownFields`）解码为 DTO，
  再 `convertAndValidate`：标识符/版本/枚举/对象规则、引用闭包检查、
  命名类型引用图 DFS 环检测。错误全部是带定位信息的 `*sdkerrors.Error`。
- `compat`：无副作用的结构 diff，产出稳定排序的变更清单。
- `serde`：线程安全的 codec 注册表（拒绝重复注册/空名/nil），内置确定性 JSON。
- `generator`：编排、指纹、门禁、渲染、原子发布、历史/清单、文件锁。
- `runtime`：`Client.Invoke` 统一执行模型（序列化→transport→状态映射→反序列化）。

## 确定性（determinism）

- 所有 JSON 经 `stableJSON`：map 键排序、固定缩进、无 HTML 转义、无尾换行抖动。
- 渲染前按 service/version 排序；map 迭代的地方都显式排序。
- 整批输入有稳定指纹 `hashDocument`（排序后序列化再 SHA-256）；
  每个 artifact 在 manifest 中记录 SHA-256。
- 代码不含时间戳、随机数、主机名等易变内容，所以相同输入→字节一致产物。

## 幂等复用

`Generate` 第一步读 `.sdkgen/manifest.json`：若其 `input_sha256` 与当前批次相同，
直接返回既有 artifact 列表（`Reused=true`），不渲染、不写盘。

## 并发模型

同一输出根：

1. `sync.Map` 内的 per-root `sync.Mutex`（进程内串行）；
2. `.sdkgen/lock` 上的 `flock(LOCK_EX)`（跨进程串行）；
3. 每批次在独立 `.staging-*` 目录渲染，互不接触；
4. 发布时每个目标文件“写同目录临时文件 → `rename`”（POSIX 原子），
   被替换文件先备份；任一步失败则逆序回滚，再删除 staging。

因此并发调用观察到的状态只能是“上一版完整产物”或“新版完整产物”，
不会出现半成品；重复批次中后到者会看到已发布的 manifest 并走复用。

跨输出根：各自独立的 staging / state，目录不重叠，天然隔离。

## 运行期错误归一化

`runtime.Client.Invoke` 把异常按下面顺序归并到四类 Kind：

1. 请求为空 / codec 未知 / 序列化失败 → `input_error`；
2. context 取消 / transport 返回任意厂商错误 → `runtime_call_failed`（厂商错误
   文本只保留为 `Unwrap()` 的 cause，**不进入**稳定的 `Reason`）；
3. transport 响应状态：400→`input_error`，409→`version_incompatible`，
   5xx/503 及其它未映射状态→`runtime_call_failed`；
4. 反序列化失败 → `runtime_call_failed`。

成功路径不做任何语义改写：解码后的 `out` 与原始 `Raw` 都原样返回。

## 生成物的自包含策略

生成的 Go module 自带 `sdkerrors` 与 `runtime` 的最小副本（仅标准库），
因此无需 `replace` 或网络即可 `go build ./...`。二者的 Kind 与归一化行为与
工具链本体保持一致；类型化客户端通过 `go:embed metadata.json` 内嵌调用元数据，
线上操作名使用描述中的原始 wire 名（如 `create_invoice`），Go 方法名才是
导出名（`CreateInvoice`）。
