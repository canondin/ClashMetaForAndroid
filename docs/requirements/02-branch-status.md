# 分支状态：feature/proxy-group-filter

## 分支信息

- **分支**: `feature/proxy-group-filter`
- **基线**: `main`
- **领先 main**: 10 commits
- **核心功能**: 用户自定义 JS 脚本过滤代理节点和代理组

## 已实现功能 & 代码位置

### 1. Go 层 JS 脚本引擎 (goja)

**Commit**: `09157dca` feat(script): add JS-based proxy filter with goja engine

| 文件 | 行号 | 功能 |
|------|------|------|
| `core/src/main/golang/native/config/script.go` | 全文 | goja VM 执行用户脚本，5s 超时，console 路由到 logcat |
| `core/src/main/golang/native/config/script.go:21-85` | L21-85 | `executeScript()` — 构建 JS config 对象、执行 `main(config)`、解析返回值 |
| `core/src/main/golang/native/config/script.go:87-117` | L87-117 | `patchScript()` — 从 profileDir 读取 script.js 并执行，超时/错误优雅降级 |
| `core/src/main/golang/native/config/script.go:128-140` | L128-140 | `ReadScript()` / `WriteScript()` — JNI 暴露的读写接口 |
| `core/src/main/golang/native/config/script_test.go` | 全文 | 10 个测试用例：过滤、重组、错误处理、文件 I/O |

### 2. 脚本集成到配置处理流水线

**Commit**: `09157dca`

| 文件 | 行号 | 功能 |
|------|------|------|
| `core/src/main/golang/native/config/process.go:19-30` | L19-30 | `processors` 数组中插入 `patchScript`，位于 `patchOverride` 之后、`patchDns` 之前 |

### 3. JNI 桥接层

**Commit**: `09157dca`

| 文件 | 行号 | 功能 |
|------|------|------|
| `core/src/main/golang/native/config.go:64-72` | L64-72 | `readScript` / `writeScript` JNI 导出函数 |
| `core/src/main/java/com/github/kr328/clash/core/Clash.kt` | — | Kotlin 端新增 `readScript()` / `writeScript()` API |
| `core/src/main/java/com/github/kr328/clash/core/bridge/Bridge.kt` | — | Bridge 层声明 |

### 4. 脚本编辑器 UI

**Commit**: `9e103487` feat(ui): add extension script editor in profile properties

| 文件 | 行号 | 功能 |
|------|------|------|
| `design/src/main/res/layout/design_properties.xml` | L104-111 | 属性页「扩展脚本」入口按钮 |
| `design/src/main/res/layout/dialog_code_field.xml` | 全文 | 等宽字体多行代码编辑对话框布局 |
| `design/src/main/java/com/github/kr328/clash/design/PropertiesDesign.kt` | — | 新增 `Request.SaveScript`、`script` 属性 |
| `design/src/main/java/com/github/kr328/clash/design/dialog/Input.kt` | — | `requestModelCodeInput()` 对话框函数 |
| `design/src/main/res/drawable/ic_baseline_key.xml` | — | 密钥图标 |
| `design/src/main/res/values/strings.xml` | — | 新增字符串资源（多语言） |

### 5. Profile 属性页脚本加载/保存

**Commit**: `9e103487`

| 文件 | 行号 | 功能 |
|------|------|------|
| `app/src/main/java/com/github/kr328/clash/PropertiesActivity.kt:33-34` | L33-34 | 加载 `script.js` 到 design |
| `app/src/main/java/com/github/kr328/clash/PropertiesActivity.kt:71-76` | L71-76 | 处理 `SaveScript` 请求，写入 `script.js` |
| `app/src/main/java/com/github/kr328/clash/PropertiesActivity.kt:128-130` | L128-130 | `getProfileDir()` 获取 profile 文件目录 |

### 6. 硬编码过滤（早期版本，已被脚本引擎替代）

**Commit**: `7db03c04` feat: filter proxy-groups for JP/SG streaming nodes

| 文件 | 功能 |
|------|------|
| `service/src/main/java/com/github/kr328/clash/service/ProfileProcessor.kt` | 硬编码过滤逻辑（已被脚本引擎替代，文件中的变更主要是 age 密钥相关） |
| `core/src/main/java/com/github/kr328/clash/core/model/Proxy.kt` | Proxy 模型新增字段 |

> 注：最初的硬编码过滤方式已被 goja 脚本引擎替代，用户可通过 `script.js` 完全自定义过滤逻辑。

### 7. 其他变更（来自 main 合入）

| Commit | 功能 |
|--------|------|
| `a69dd69a` | feat(profile): support age secret keys (#764) |
| `7f63b750` | Update Dependencies (#758) |
| `e49dd652` | ci: add fork build workflow |
| `7eee2b3a` | build: restrict to arm64-v8a only |

## 依赖变更

- `go.mod`: 新增 `github.com/dop251/goja`（纯 Go JS 引擎）
- `build.gradle.kts`: 构建配置调整

## 待办事项

1. **R4**: 延迟测试 URL 一致性修复 — `core/src/main/golang/native/tunnel/proxies.go:185-199`
2. **R5**: 延迟测试并发优化 — `core/src/foss/golang/clash/adapter/provider/healthcheck.go:132`
