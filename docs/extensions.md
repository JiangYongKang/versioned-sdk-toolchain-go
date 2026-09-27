# 扩展方式

工具链在三个层面预留了复用扩展点，全部可在本地完成，无需外部服务。

## 1. 新增序列化实现（serde.Codec）

实现接口并注册即可：

```go
type Codec interface {
    Name() string
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}

reg := serde.MustDefaultRegistry()
if err := reg.Register(myCodec{}); err != nil { // 重名/nil/空名 -> input_error
    log.Fatal(err)
}
```

接口描述里通过顶层 `codec` 选择：

```json
{ "name": "billing", "version": "v3.0.0", "codec": "mywire", ... }
```

仓库内置 `serde.TextKVCodec`（名 `textkv`）作为参考实现：它只支持扁平
`map[string]any` 标量载荷，演示了：

- 确定性输出（键排序、固定转义）；
- 对不支持的类型返回稳定的 `input_error`；
- 编解码往返不改变值语义。

CLI 中通过 `--codec textkv` 可把该示例扩展一并注册。

> 注意：生成器用的是**工具链**的 codec 注册表；生成出的独立 module 默认只内置
> JSON。若业务希望在生成物运行期使用自定义 wire 格式，可把该 codec 的源码放入
> 生成 module（或在业务仓库内实现同签名接口）并在构造 `runtime.Client` 时注册。

## 2. 新增底层调用实现（runtime.Transport）

业务只需实现一个接口即可对接任意后端（HTTP、gRPC、内存 mock、录制回放等）：

```go
type Transport interface {
    Call(ctx context.Context, req *PreparedRequest) (*RawResponse, error)
}
```

- 返回 `RawResponse{Status, Body}`，由 `runtime.Client` 统一做状态码→Kind 映射；
- 直接返回的错误（任意厂商类型，如连接失败、panic 恢复错误）会被包装为
  `runtime_call_failed`，厂商错误类型不会出现在稳定错误表面，只能通过
  `errors.Is/As` 与 `Unwrap()` 触达。

仓库提供：

- `runtime.LocalTransport`：进程内路由表，用于离线联调/测试；
- `runtime.FailingTransport`：始终抛“厂商错误”，用于验证归一化。

## 3. 新增接口描述来源 / 自定义校验

- `loader.Loader` 目前扫描本地 JSON 文件；`generator.GenerateFrom` 直接接受
  内存中的 `*schema.Document`，因此可以从任意来源（Git 仓库、打包资源、数据库
  导出等）构造文档后复用同一条“门禁→渲染→原子发布”流水线。
- `loader.Options.Extensions` 可扩展接受的文件扩展名；要支持 YAML/TOML 等，
  可在外部解码为等价 DTO 后复用现有校验思路，或在自己的加载器中直接产出
  `schema.Interface`。

## 4. 扩展兼容性策略（谨慎）

规则集中在 `compat` 包（`diffTypes/diffFields/diffEnums`）。新增规则时：

1. 必须归类为 `ChangeCompatible` 或 `ChangeBreaking`；
2. 必须给出稳定的 `Field`/`Op` 路径与 `Detail` 原因；
3. 保持输出排序确定；
4. 在 `compat/compat_test.go` 增补“接口 + 版本对 + 判定依据”的用例。

破坏性变化默认应保守拒绝；放宽规则意味着旧客户端可能在新后端上失败。
