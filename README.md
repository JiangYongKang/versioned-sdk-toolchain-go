# versioned-sdk-toolchain-go

一套**本地运行、可复用**的 schema 驱动 SDK 生成工具链。输入是多份本地接口描述
（JSON），输出是可被业务直接调用的 Go SDK 与调用元数据。整个流程不访问网络、
不依赖外部服务或真实凭据；它不是一个孤立脚本，而是由可组合的 Go 包和一个
`sdkgen` CLI 组成。

## 目录结构

```
cmd/sdkgen          CLI：generate / check-compat 两个子命令
pkg/schema          接口描述模型与语义化版本（major.minor.patch）
pkg/apierrors       四类稳定错误分类 + 可定位到接口/字段的 Issue 聚合
pkg/load            本地 JSON 描述解码（严格模式：拒绝未知字段）
pkg/validate        缺失引用 / 类型冲突 / 循环引用 / 重名 / 非法命名校验
pkg/compat          跨版本兼容性判定（逐项变更，兼容 vs 破坏性）
pkg/serialize       可插拔序列化注册表（默认 JSON，可注册自定义 Codec）
pkg/codegen         SDK + metadata.json 生成（确定性输出、原子落盘）
pkg/runtime         元数据驱动的调用运行时与底层异常归一化
examples/           petstore、多文档分片等示例描述
test/               端到端测试（加载→生成→编译→调用→兼容→并发）
```

## 输入描述格式

每个 JSON 文件描述一个服务版本的一个或多个分片（同一 `service`+`version` 的
多份文件会合并）。类型引用必须且只能设置 `ref / primitive / array / map` 之一：

```json
{
  "service": "petstore",
  "version": "1.0.0",
  "enums":  [{"name": "PetKind", "values": ["dog", "cat"]}],
  "structs": [
    {"name": "Pet", "fields": [
      {"name": "id",   "type": {"primitive": "long"}},
      {"name": "kind", "type": {"ref": "PetKind"}},
      {"name": "tags", "type": {"array": {"primitive": "string"}}, "optional": true}
    ]}
  ],
  "interfaces": [
    {"name": "GetPet", "method": "GET /pets/{id}",
     "input":  [{"name": "id", "type": {"primitive": "long"}}],
     "output": {"ref": "Pet"},
     "errors": []}
  ]
}
```

支持的 primitive：`string int long float double bool bytes timestamp`。
支持的类型形状：primitive、命名 ref（struct/enum）、`array`、
`map`（键固定为 string，值为任意类型）、`nullable` 标记。

## 使用方法

### 生成 SDK（可多份描述合并）

```sh
go run ./cmd/sdkgen generate -o build/sdk -pkg shopsdk \
    examples/multi/common_1.0.0.json examples/multi/orders_1.0.0.json
```

产物（**确定性**：相同输入逐字节一致；gofmt 干净）：

```
build/sdk/
  doc.go         包文档 + 服务名/版本
  types.go       struct / enum 的强类型定义
  errors.go      SDKError 与稳定错误分类（生成包零工具链依赖）
  client.go      每个 interface 一个强类型方法 + Backend 接口
  smoke_test.go  随包生成的冒烟测试（成功调用 + 异常归一化）
  metadata.json  调用元数据（运行时据此校验/分发）
```

业务侧实现 `Backend`（HTTP/RPC 任意）即可调用：

```go
c := shopsdk.New(myBackend)
pet, err := c.GetPet(ctx, &shopsdk.GetPetRequest{Id: 9})
switch shopsdk.Code(err) {
case shopsdk.CatInvalidInput:        // 请求本身不合法
case shopsdk.CatInvocationFailed:    // 底层调用失败（已归一化并包装原始错误）
}
```

不希望引用生成包时，也可用 `runtime.NewClientFromMetadata(metadata, backend)`
仅凭 `metadata.json` 构造客户端。

### 跨版本兼容性判定

```sh
go run ./cmd/sdkgen check-compat -old old.json -new new.json
```

逐项打印 `[OK]`（兼容）或 `[BREAK]`（破坏性），含接口/字段与原因；
存在破坏性变更时进程以退出码 2 失败。

## 版本兼容规则

新版本号必须**严格大于**旧版本，否则按 `invalid_input` 拒绝。逐项判定如下：

| 变更 | 判定 |
|---|---|
| 新增**可选** struct 字段 | 兼容 |
| 新增**必填** struct 字段 | 破坏性 |
| 删除字段 | 破坏性 |
| 可选→必填 | 破坏性；必填→可选 | 兼容 |
| 枚举扩容（新增 value） | 兼容 |
| 删除枚举 value | 破坏性 |
| 原始类型放宽 `int→long→float→double` | 兼容；反向收窄/换类 | 破坏性 |
| 字段改为 nullable | 兼容；nullable 改回非空 | 破坏性 |
| 接口新增**可选**入参 | 兼容；新增必填入参 | 破坏性 |
| 删除/改类型入参 | 破坏性 |
| 输出类型任何变化、增删输出 | 破坏性（成功调用语义不允许悄悄改变） |
| 新增 interface / struct / enum | 兼容；删除 | 破坏性 |
| interface 增删错误声明 | 兼容 |

所有判定结果都带 `Interface`、`Field`（字段路径，数组/Map 带 `[]`/`[map]`）
和 `Reason`，测试输出会打印接口、版本与判定依据。

## 校验：绝不静默跳过

`validate` 会把以下问题**一次性**收集为 `invalid_input` 的多 Issue 错误，
精确定位到文件/接口/字段：

- 引用了不存在的类型、未知 primitive、形状非法的类型引用；
- 跨分片同名 struct/enum/interface 定义不一致（类型冲突）；
- struct 之间的循环引用（渲染完整环路径，如 `A -> B -> A`）；
- 空名称、重名字段/参数/枚举值；
- 非法版本号（必须是无前导零的 `major.minor.patch`）；
- 非法 JSON、未知 JSON 字段（strict decode）。

## 稳定性与并发

- **确定性输出**：类型/枚举/接口/产物列表在输出前全部排序，元数据缩进固定；
  同一批输入重复生成结果逐字节相同。
- **原子落盘**：先写入同目录下的临时目录，再整体 rename 覆盖；
  同一输出目录加锁串行化，并发生成互不污染，外部永远看不到半成品；
  失败时保留旧目录、清理临时目录（已通过 `-race -count` 与并发测试验证）。

## 错误映射（统一分类）

| Category | 含义 |
|---|---|
| `invalid_input` | 描述文件畸形、校验失败、未知接口/格式、编码失败 |
| `version_incompatible` | 跨版本判定存在破坏性变更 / 版本协商不一致 |
| `generation_failed` | 生成内容非法或产物落盘失败 |
| `invocation_failed` | 运行期底层后端调用/解码失败 |

底层后端的原生错误不会直接透出：运行时把任意非分类错误归一化为
`invocation_failed`，原始错误通过 `Unwrap`/`errors.Is` 仍可取回；
已分类的错误（如后端主动返回的 `version_incompatible`）原样保留。
**成功调用的载荷与响应语义不被改变**（有专门测试断言）。

## 扩展方式：自定义序列化

实现 `serialize.Codec` 并注册即可，运行时用 `WithFormat("xxx")` 选择：

```go
type Codec interface {
    Name() string
    ContentType() string
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}

reg := serialize.NewRegistry()
reg.Register(myMsgpackCodec{})
c, _ := runtime.NewClient(bundle, backend,
    runtime.WithRegistry(reg), runtime.WithFormat("msgpack"))
```

## 本地验证（无需网络/凭据）

```sh
go test -race ./...          # 全部单测 + 端到端（含并发重复生成）
gofmt -l .                   # 代码风格检查
go vet ./...

# 手工冒烟
go run ./cmd/sdkgen check-compat \
    -old examples/petstore_v1.json -new examples/petstore_v2_compatible.json
go run ./cmd/sdkgen generate -o build/petstore-sdk -pkg petstore \
    examples/petstore_v1.json
go build ./build/petstore-sdk
go test ./build/petstore-sdk # 生成 SDK 的真实调用冒烟测试
```

测试覆盖：跨版本兼容/破坏性变更、畸形输入（缺引用、环、坏版本、未知字段）、
并发与重复生成稳定性、底层异常归一化与成功语义保持。失败场景均按错误
Category 分别断言，并打印相关接口、版本与判定依据。
