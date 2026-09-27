# versioned-sdk-toolchain-go

一套**本地可运行、可复用**的 schema 驱动 SDK 生成工具链。输入是多份本地接口描述
（JSON），输出是可被业务直接调用的**类型化 Go SDK** 与**语言无关的调用元数据**。
全过程不访问外部服务、不需要真实凭据；生成出的 SDK 仅依赖 Go 标准库，可离线编译。

## 能力一览

- **多接口描述加载与强校验**：从 `<root>/<service>/<version>.json`（支持
  `{service}@{version}.json` 与目录递归）加载，缺失引用、类型冲突、循环引用、
  非法版本、重复操作/字段/枚举值等一律**报错而非静默跳过**，错误精确到
  service / version / op / field 与原因。
- **跨版本兼容判定与可重复生成**：
  - 兼容（继续生成）：新增**可选**字段、枚举**扩容**、新增命名类型、新增操作；
  - 破坏（明确拒绝并给出受影响范围）：删字段/操作/类型、改类型、改引用目标、
    删枚举值、枚举重排、必填/可选语义改变、新增**必填**字段。
  - 兼容门禁既在**同一批输入**内相邻版本间生效，也跨多次运行（保存在
    `.sdkgen/history.json`）生效。
- **并发安全 / 幂等**：同一输出根的并发生成由进程内互斥锁 + `flock` 串行化；
  渲染在私有 staging 目录进行，发布按文件原子 rename，失败回滚，不残留半成品；
  同一批输入重复生成直接复用（`reused=true`），独立运行产物**字节一致**。
- **可插拔序列化**：内置确定性 JSON，可注册自定义 codec（仓库内置 `textkv`
  示例扩展演示扩展方式）。
- **统一错误映射**：调用方稳定区分四类错误，底层/厂商异常不直接透出；
  成功调用的语义不被改变（解码结果原样返回）。

## 稳定错误分类（`sdkerrors.Kind`）

| Kind | 含义 | 典型来源 |
| --- | --- | --- |
| `input_error` | 非法入参 / 畸形接口描述 / 后端 400 | loader、codec 选择、NormalizeStatus(400) |
| `version_incompatible` | 跨版本破坏性变化 / 后端 409 版本冲突 | compat 门禁、NormalizeStatus(409) |
| `generation_failed` | 渲染或原子发布失败 | generator |
| `runtime_call_failed` | 运行期调用失败（含一切底层/厂商异常） | runtime、5xx、transport error |

错误消息格式稳定且确定性，例如：

```
version_incompatible: breaking change rejected for billing v1.0.0 -> v2.0.0;
affected locations: types.Money.fields.memo: field removed; ...
[service=billing, version=v1.0.0->v2.0.0]
```

## 目录结构

```
sdkerrors/   统一错误分类（生成期与运行期共用同一套 Kind）
schema/      内存模型 + 语义版本解析（v1 / v1.2 / v1.2.3）
loader/      本地 JSON 加载与结构校验（缺失引用/冲突/循环/非法版本）
compat/      跨版本 diff 与兼容性判定
serde/       序列化注册表 + 确定性 JSON + textkv 扩展示例
generator/   规划/门禁、确定性渲染、staging 原子发布、历史与幂等
runtime/     生成 SDK 使用的通用客户端、错误归一化、离线 LocalTransport
cmd/sdkgen/  CLI（generate / validate）
testdata/    billing v1/v2 示例接口描述
docs/        架构、兼容规则、扩展方式、本地验证
```

## 快速开始

```bash
# 仅校验并打印相邻版本兼容判定依据，不写文件
go run ./cmd/sdkgen validate --in ./testdata/schemas

# 生成 SDK 与元数据（幂等，可重复执行）
go run ./cmd/sdkgen generate \
  --in ./testdata/schemas \
  --out ./.output \
  --module sdk-generated

# 生成结果可独立离线编译
(cd .output/go && go build ./... && go vet ./...)
```

生成产物：

```
.output/
  metadata/<service>/<version>.json   # 语言无关调用元数据
  go/go.mod                            # 自包含 module
  go/sdkerrors/*.go                    # 最小统一错误分类副本
  go/runtime/*.go                      # 最小通用客户端 + 归一化 + LocalTransport
  go/sdk/<service>/<pkg>/...           # 类型化客户端、类型、内嵌 metadata.json
  .sdkgen/{manifest.json,history.json,lock}
```

类型化调用示例（生成物）：

```go
lt := runtime.NewLocalTransport()
lt.Handle("billing", "create_invoice", func(ctx context.Context, req *runtime.PreparedRequest) (*runtime.RawResponse, error) {
    return &runtime.RawResponse{Status: runtime.StatusOK,
        Body: []byte(`{"invoice_id":"inv-1","status":"open"}`)}, nil
})
c := billingv2.NewClient(runtime.NewClient(lt))
out, err := c.CreateInvoice(ctx, &billingv2.CreateInvoiceInput{ /* ... */ })
```

## 输入描述格式

见 `testdata/schemas/billing/v1.json`、`v2.json`。要点：

- 顶层：`name`（标识符）、`version`（`v1` / `v1.2` / `v1.2.3`）、
  `codec`（默认 `json`）、`types[]`、`operations[]`。
- 类型 `type` 支持 `string|integer|number|boolean|enum|array|map|object|ref`：
  - `enum` 用 `values`；`array/map` 用 `elem`；`object` 用 `fields[]`；
  - `ref` 用 `ref` 指向同接口内的命名类型（不允许悬空/跨文件引用）；
  - 字段可标 `optional:true`。
- 未知名字段会被拒绝（`DisallowUnknownFields`）。

## 本地验证

```bash
go test ./...                 # 全量单测/集成测试（含生成物离线编译与生成物内 go test）
go test -race ./...           # 并发重复生成竞态检测
go vet ./...
```

测试覆盖跨版本兼容/破坏、畸形输入、并发重复生成、独立 root 隔离、
跨运行破坏拒绝与回滚、底层异常归一化，以及生成物模块的端到端调用。
失败用例均按 `Kind` 与具体 service/version/field 断言，并在测试日志中打印
接口、版本与判定依据。

更多细节见：

- [docs/compatibility.md](docs/compatibility.md) — 版本与兼容性规则
- [docs/architecture.md](docs/architecture.md) — 架构、确定性与原子发布
- [docs/extensions.md](docs/extensions.md) — 序列化扩展与自定义接入方式
- [docs/verification.md](docs/verification.md) — 本地验证手册
