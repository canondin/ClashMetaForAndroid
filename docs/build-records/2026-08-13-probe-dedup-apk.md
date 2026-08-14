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

**探测去重默认开启（300 秒窗口），无需任何配置。**

只要扩展脚本 `script.js` 把所有 url-test/fallback 组的 `url` 统一成同一个字符串（用户现有脚本已这么做），跨组延迟就会一致——不需要在配置顶层加任何字段。

可选调优（仅当需要改变默认行为时，在配置顶层加）：

```yaml
skip-recent-probe: 600    # 放大到 10 分钟窗口
skip-recent-probe: 1      # 实质关闭（1 秒窗口）
```

> 设计说明：dedup 默认开启是因为扩展脚本只能改 `proxies`/`proxy-groups`，改不了顶层 general 字段。让 `skip-recent-probe` 缺省即生效，用户在手机端只需维护 script.js 即可。

## 已知限制

- 探测去重仅对**同 URL**生效；不同 URL 的桶各自独立探测（设计如此）
- 此 APK 为 debug 版，会覆盖现有应用（profile 数据保留）
- submodule 补丁位于 `canondin/mihomo:feature/skip-recent-probe`，跟随上游需走 rebase 流程

## 签名与安装（重要）

本次构建的是 **alpha debug** 变体，使用本机 debug.keystore 签名：

- debug.keystore 路径：`~/.android/debug.keystore`（本机生成于 2026-05-28）
- APK 签名 SHA256：`A5:92:25:E0:E9:C3:E8:2B:30:36:5E:29:AC:5C:9D:A1:F1:C7:0D:11:2F:66:46:0F:BC:7B:BA:1C:19:BE:D0:45`

### 为何首次无法更新安装

Android 的 debug 签名是**每台机器各自生成**的（密码固定 `android`，但密钥对不同）。手机上若已装由**其他机器/CI** 构建的 debug 版，签名不一致 → Android 拒绝作为更新安装。

### 安装方式

- **全新安装**：先卸载手机现有版本，再装本 APK。无签名校验，必定成功。代价：profile / script.js 数据清除。
- **更新安装**：仅当手机现有版本也是由**同一台机器** debug.keystore 签名时可直接更新。

### 长期建议

- 始终在本机构建 debug → debug.keystore 固定 → 以后可直接更新安装。
- 若要跨机器一致签名，需用 `release.keystore` + `signing.properties`（`signing.properties` 被 gitignore，含 3 个密码；构建 release 变体）。
- APK 完整性已校验：源文件与 OneDrive 副本 SHA256 一致（`b5145a90...`），排除传输损坏。
