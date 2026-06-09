# 需求：代理组过滤与脚本引擎

## 背景

订阅返回的代理节点通常包含大量无关节点（信息节点、过期节点、非目标地区节点等），需要一种机制让用户自定义过滤和分组逻辑。

## 需求列表

### R1: 代理组脚本过滤引擎

**优先级**: P0 (已实现)

用户可在 Profile 属性页编写 JS 脚本（`script.js`），在配置加载时执行 `main(config)` 函数，对 `proxies` 和 `proxy-groups` 进行过滤、重组。

- 脚本接收 `{ proxies, proxy-groups }` 结构
- 脚本返回修改后的同结构对象
- 脚本执行有 5s 超时保护
- 脚本失败不崩溃，跳过并记录日志

### R2: 脚本编辑器 UI

**优先级**: P0 (已实现)

在 Profile 属性页提供扩展脚本编辑入口，支持查看/编辑/保存 `script.js`。

### R3: 脚本执行集成到配置处理流水线

**优先级**: P0 (已实现)

`patchScript` 作为 config processor 的一环，在 override 之后、DNS/TUN 之前执行。

### R4: 延迟测试 URL 一致性

**优先级**: P1 (已实现)

当前 `convertProxies` 查询延迟时，从 `ExtraDelayHistories()` map 中随机取 testURL，可能与实际测试 URL 不匹配，导致延迟显示为 0 或不更新。

修复：新增 `pickBestTestURL()` 函数，遍历所有 key 取最低非零延迟对应的 testURL，兜底使用 `C.DefaultTestURL`。

**修改文件**: `core/src/main/golang/native/tunnel/proxies.go`

### R5: 延迟测试并发优化

**优先级**: P2 (待实现)

当前并发限制为 10（`healthcheck.go:132`），代理数量多时测试耗时长。可考虑提升并发数或允许用户配置。

### R6: 其他上游功能（已合入 main）

- age 密钥解密支持 (#764)
- 依赖更新 (#758)
