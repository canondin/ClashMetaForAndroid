# 构建记录：2026-08-13 probe-dedup APK

## 概要

| 项 | 值 |
|---|---|
| 日期 | 2026-08-13 |
| 构建目标 | `:app:assembleAlphaDebug` |
| 构建耗时 | 44s |
| 可执行任务 | 152（执行 109，up-to-date 43） |
| 输出 APK | 56M |
| 变体 | alpha debug（arm64-v8a） |
| JDK | openjdk@21（`/opt/homebrew/opt/openjdk@21`） |
| Gradle | 8.10.2 |

## 产物

```
app/build/outputs/apk/alpha/debug/cmfa-2.11.32-alpha-debug.apk
```

已复制到 OneDrive 同步目录：

```
~/Library/CloudStorage/OneDrive-个人/cmfa-2.11.32-alpha-debug-probe-dedup-20260813.apk
```

- 大小：56M
- SHA256：`b5145a90f104ec46cd25760fb7e2f6fec4e2e5ad7c45ac7d80f9d486015218df`

## 本次构建包含的改动

基于 `944b4bf9`（fork 既有 tip）之上的 5 个提交：

```
2c571c19 docs(delay): record mihomo fork workflow for probe-dedup patch
9913f636 feat(delay): cross-group probe deduplication (skip-recent-probe)
01b9e09b docs(delay): document cross-group probe deduplication flow
c66bf737 build(core): sync mihomo Alpha to 7ee0b05b
7b1466f6 docs(requirements): add shared delay pool design for cross-group latency consistency
```

### 核心特性

- **mihomo submodule 升级**：`e26714a1` → `7ee0b05b`（Alpha，75 个上游提交，含 ZeroTier/mipstack/gVisor/QUIC 改进，测速相关路径零改动）
- **probeCache 探测去重**（submodule commit `12119ace`）：
  - 同 `(proxy, url, expectedStatus)` 在 `skip-recent-probe` 窗口内只测一次
  - 跨所有 HealthCheck 共享缓存
  - 默认 `0` 关闭，保持原行为
- **新增配置字段**：`skip-recent-probe`（秒）

## 前置构建验证（均已通过）

| 步骤 | 耗时 | 结果 |
|---|---|---|
| `go test ./adapter/provider/...`（9 测试，含 -race） | ~1.6s | PASS |
| `go test ./adapter/...` | ~1.4s | PASS |
| `go build ./config/...` | <1s | PASS |
| `go vet ./adapter/provider/...` | <1s | PASS |
| `:core:externalGolangBuildAlphaDebugArm64V8a` | 13s | BUILD SUCCESSFUL |
| `:core:externalGolangBuildMetaDebugArm64V8a` | 38s | BUILD SUCCESSFUL |
| `:core:assembleAlphaDebug` | 11s | BUILD SUCCESSFUL |
| `:core:assembleMetaDebug` | 11s | BUILD SUCCESSFUL |
| `:app:assembleAlphaDebug` | 44s | BUILD SUCCESSFUL |

## 使用方法

profile 配置顶层加：

```yaml
skip-recent-probe: 600    # 10 分钟去重窗口（桌面端）
skip-recent-probe: 14400  # 4 小时（手机端节流）
skip-recent-probe: 0      # 缺省，保持原行为
```

## 已知限制

- 探测去重仅对**同 URL**生效；不同 URL 的桶各自独立探测（设计如此）
- 此 APK 为 debug 版，会覆盖现有应用（profile 数据保留）
- submodule 补丁位于 `canondin/mihomo:feature/skip-recent-probe`，跟随上游需走 rebase 流程
