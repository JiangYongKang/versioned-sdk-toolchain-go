# 版本与兼容性规则

## 版本号

`schema.ParseVersion` 接受 `vMAJOR[.MINOR[.PATCH]]`（前缀 `v` 可省略、大小写均可），
规范化为 `vMAJOR.MINOR.PATCH`。以下形式一律拒绝（`input_error`），不进入后续流程：

- 空串、单独的 `v`、负数、非数字段（`latest`、`v1-alpha`）；
- 段数超过 3（`v1.2.3.4`）、空段（`v1..0`）。

同一服务同一版本在同批或多文件中重复出现 = 冲突，直接报错。

## 比较模型

`compat.Compare(prev, next)` 逐结构比较**同一服务**的两个接口，输出有序的
`Change{Kind, Service, FromVer, ToVer, Op, Field, Detail}` 列表：

- 比较范围：命名类型集合、操作集合、对象字段、枚举成员、引用目标、数组/Map 元素。
- 顺序确定性：所有集合先排序再比较，输出与 map 遍历顺序无关。

### 兼容变化（`compatible`，允许继续生成）

| 变化 | 说明 |
| --- | --- |
| 新增对象**可选**字段 | `optional:true` 的全新字段 |
| 枚举扩容 | 仅**追加**新枚举值，且不改变既有顺序 |
| 新增命名类型 | 旧代码不引用即不受影响 |
| 新增操作 | 旧调用方不受影响 |
| 文档变化 | 不影响线上结构 |

### 破坏性变化（`breaking`，拒绝生成并说明影响面）

| 变化 | 影响说明 |
| --- | --- |
| 删除字段 | 旧调用方仍会产出/读取该字段 |
| 删除操作 | 旧调用方调用必然失败 |
| 删除命名类型 | 引用与使用全部失效 |
| 字段类型 kind 改变 | 如 `integer -> string`、`object -> array` |
| 引用目标改变 | `ref: Money -> OtherMoney` |
| 数组/Map 元素类型改变 | 线上载荷形状改变 |
| 删除枚举值 | 旧端可能仍产出该值 |
| 枚举重排 | 依赖序号的序列化语义改变 |
| 必填 <-> 可选 | 工具链保守地视为破坏；唯一例外是“新增字段且为可选” |
| 新增**必填**字段 | 老客户端无法提供该值 |

## 门禁在哪里生效

1. **同批输入内**：同一服务的多个版本按版本号排序，对相邻版本逐一比较。
2. **跨多次运行**：生成器把每个已接受版本写入 `.sdkgen/history.json`；
   新批次中某服务的**最低版本**会与历史中“严格低于它的最高版本”比较。
3. 任意一处出现 `breaking`，整体生成在渲染前中止（`version_incompatible`），
   reason 中枚举所有受影响的 `field/op` 与原因，且不会发布任何新文件。

## 重新生成相同版本

相同 `(service, version)` 若结构已被接受，再次提交**相同内容**走幂等复用；
若内容与历史不同（同版本号但形状改变），属于语义篡改：

- 当前实现下，相同版本号不会再与自己比较；但因整批输入指纹
  （manifest.input_sha256）变化，会重新进入规划与比较路径。
- 推荐实践：形状改变必须使用新版本号；不要原地修改已发布版本。

## 判定依据可观测

CLI `validate` 与 `generate` 都会打印每个版本对的结论与每条变化，例如：

```
- billing v1.0.0 -> v2.0.0: COMPATIBLE (3 change(s))
     compatible list_invoices/operations.list_invoices   new operation added
     compatible types.Currency.values.GBP                enum value added ...
     compatible types.Money.fields.memo                  new optional field added
```

单测（`compat/compat_test.go`、`generator/*_test.go`）在日志中打印
interface、版本对与判定依据，便于审计。
