# 本地验证手册

全部验证均可离线完成（仅需 Go 1.25+，无外部服务、无凭据）。

## 1. 构建与静态检查

```bash
go build ./...
go vet ./...
gofmt -l .   # 应无输出
```

## 2. 单元与集成测试

```bash
go test ./...
go test -race ./...            # 并发/竞态
go test -v ./compat/           # 跨版本规则（含判定依据日志）
go test -v ./loader/           # 畸形输入、循环引用、冲突
go test -v ./runtime/          # 成功语义 + 状态/异常归一化
go test -v ./generator/        # 生成、幂等、原子、并发、生成物编译
go test -v ./cmd/sdkgen/       # CLI 退出码与输出
```

### 关键测试与断言点

| 测试 | 验证内容 |
| --- | --- |
| `loader.TestMalformedInputs` | 每种畸形输入都是 `input_error`，且能定位到具体 field/原因 |
| `loader.TestCycleDetected` | 命名类型循环引用被拒绝，诊断含完整引用链 |
| `compat.TestCompatibleChanges` | 可选字段/枚举扩容/新操作/新类型判为兼容 |
| `compat.TestBreakingChanges` | 删字段/改类型/删枚举/重排/删操作等判为破坏并定位 |
| `generator.TestRepeatedGenerationIsReusedAndStable` | 重复生成 `reused=true`，独立 root 字节一致 |
| `generator.TestConcurrentSameRoot` | 12 并发生成：仅一次发布，其余复用，无 staging 泄漏 |
| `generator.TestConcurrentDistinctRoots` | 不同 root 不互相污染 |
| `generator.TestConcurrentBreakingRejected` | 兼容/破坏批次竞争时无半成品，历史不损坏 |
| `generator.TestBreakingChangeAcrossRunsRejected` | 跨运行破坏性变化被拒，旧 manifest 存活 |
| `generator.TestGeneratedGoModuleCompiles` | 生成物离线 `go build ./...` |
| `generator.TestGeneratedSDKEndToEnd` | 向生成物注入测试并 `go test`：成功语义 + 归一化 |
| `runtime.TestForeignErrorNormalized` | 厂商错误不进入稳定 Reason，仅作 cause 保留 |
| `runtime.TestBackendStatusMapping` | 400/409/5xx 映射到对应 Kind |
| `cmd/sdkgen` 子进程测试 | 退出码 0/2/3/4 区分成功/输入/版本/生成失败 |

## 3. 手工端到端

```bash
# 校验随仓库提供的 billing v1/v2（v2 新增可选 memo、GBP 枚举、list_invoices）
go run ./cmd/sdkgen validate --in ./testdata/schemas

# 生成
rm -rf .output
go run ./cmd/sdkgen generate --in ./testdata/schemas --out ./.output --module sdk-generated

# 再生成一次：应打印 reused=true
go run ./cmd/sdkgen generate --in ./testdata/schemas --out ./.output --module sdk-generated

# 生成物独立编译/检查
(cd .output/go && go build ./... && go vet ./...)

# 查看调用元数据与清单
cat .output/metadata/billing/v2.0.0.json
cat .output/.sdkgen/manifest.json
cat .output/.sdkgen/history.json
```

## 4. 构造一次“被拒绝”的破坏性升级

```bash
mkdir -p /tmp/sdkdemo/a/svc /tmp/sdkdemo/b/svc /tmp/sdkdemo/out
cat > /tmp/sdkdemo/a/svc/v1.json <<'JSON'
{"name":"svc","version":"v1.0.0","operations":[
  {"name":"get",
   "input":{"kind":"object","fields":[{"name":"id","type":{"kind":"string"}}]},
   "output":{"kind":"object","fields":[
     {"name":"name","type":{"kind":"string"}},
     {"name":"age","type":{"kind":"integer"}}]}}]}
JSON
cp /tmp/sdkdemo/a/svc/v1.json /tmp/sdkdemo/b/svc/v2.json
# 手工把 v2.json 的 version 改成 v2.0.0 并删除 output 的 age 字段
sed -i 's/v1.0.0/v2.0.0/; /"age"/d' /tmp/sdkdemo/b/svc/v2.json

go run ./cmd/sdkgen generate --in /tmp/sdkdemo/a --out /tmp/sdkdemo/out
echo "exit=$?"   # 0
go run ./cmd/sdkgen generate --in /tmp/sdkdemo/b --out /tmp/sdkdemo/out
echo "exit=$?"   # 3，输出包含受影响字段 fields.age 与原因
```

## 5. 运行期异常归一化验证（生成物内）

`generator.TestGeneratedSDKEndToEnd` 会在生成模块中放置测试并运行 `go test`，
覆盖：本地 handler 成功往返、`FailingTransport` 的厂商错误→`runtime_call_failed`、
后端 400→`input_error`。也可参考 README 的类型化调用片段自行接
`runtime.LocalTransport` 复现。
