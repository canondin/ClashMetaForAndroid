# Plan: JavaScript 扩展脚本 — 节点过滤功能

## Context

当前分支 `feature/proxy-group-filter` 中有一个硬编码的 `patchProxyGroups` 函数，写死了只保留日本/新加坡流媒体节点。用户需要一个类似 Clash Verge 的 **JS 脚本扩展机制**，让用户可以自定义节点过滤逻辑，而不是每次改需求都要改代码重新编译。

核心需求：**JS 脚本形式，仅节点过滤**（proxy + proxy-groups 的筛选和重组）。

## 方案概述

在 Go 层嵌入 **goja**（纯 Go 的 JS 引擎），在配置处理管线中新增 `patchScript` 处理器。用户通过 Android UI 编写 JS 脚本，脚本在配置加载时执行，对 proxies 和 proxy-groups 进行过滤和重组。

参考 Clash Verge 的 API 设计，但简化为仅节点过滤场景。

## 技术选型

| 项目 | 选择 | 理由 |
|------|------|------|
| JS 引擎 | **goja** | 纯 Go、零 CGO、支持 ES6 子集（let/const/箭头/展开）、gomobile 直接打包 |
| 脚本 API | `function main(config)` | 与 Clash Verge 一致，降低用户学习成本 |
| 脚本存储 | 复用现有 override 机制 | `script.js` 文件存放在 profile 目录，与 override.json 同级 |

## 用户脚本 API 设计

用户编写的脚本遵循 Clash Verge 的约定：

```javascript
function main(config) {
  // config.proxies: [{name, type, server, port, ...}, ...]
  // config["proxy-groups"]: [{name, type, proxies, ...}, ...]

  // 示例：过滤日本节点
  config.proxies = config.proxies.filter(p => /日本|JP/i.test(p.name));

  return config;
}
```

**约束：**
- 脚本必须导出 `main` 函数
- `main` 接收完整 config 对象，必须返回修改后的 config
- 仅同步操作，不支持 Promise/async
- 执行超时 5 秒

## 实现步骤

### Step 1: Go 层 — 添加 goja 依赖

**文件**: `core/src/foss/golang/go.mod`

```bash
cd core/src/foss/golang
go get github.com/dop251/goja
```

### Step 2: Go 层 — 创建脚本执行引擎

**新文件**: `core/src/main/golang/native/config/script.go`

核心逻辑：
- `executeScript(script string, cfg *config.RawConfig) error`
- 初始化 goja VM
- 将 `cfg.Proxy` 和 `cfg.ProxyGroup` 序列化为 JSON 注入 VM
- 注入 `console.log`（路由到 Android logcat）
- 拼接执行代码：`JSON.stringify(main(config))`
- 反序列化结果回写 `cfg.Proxy` 和 `cfg.ProxyGroup`
- 超时保护：5 秒上限
- 脚本为空时直接跳过

### Step 3: Go 层 — 替换硬编码处理器

**文件**: `core/src/main/golang/native/config/process.go`

- 删除 `patchProxyGroups` 及其辅助函数（`collectStreamingJPSG`, `sortDirectFirst`, `toInterfaceSlice`）
- 新增 `patchScript` 处理器
- 从 profile 目录读取 `script.js` 文件
- 调用 `executeScript()` 执行

处理器插入位置（在 `patchProfile` 之后）：

```go
var processors = []processor{
    patchExternalController,
    patchOverride,
    patchGeneral,
    patchProfile,
    patchScript,        // ← 新增
    patchDns,
    patchTun,
    patchListeners,
    patchProviders,
    validConfig,
}
```

### Step 4: Go 层 — 暴露 JNI 接口

**文件**: `core/src/main/golang/native/main.go`

新增导出函数：

```go
//export readScript
func readScript(profilePath C.c_string) *C.char

//export writeScript
func writeScript(profilePath C.c_string, content C.c_string)
```

### Step 5: Kotlin 层 — Clash 核心桥接

**文件**: `core/src/main/java/com/github/kr328/clash/core/Clash.kt`

新增：

```kotlin
external fun nativeReadScript(profilePath: String): String
external fun nativeWriteScript(profilePath: String, content: String)

companion object {
    fun readScript(profilePath: String): String = nativeReadScript(profilePath)
    fun writeScript(profilePath: String, content: String) = nativeWriteScript(profilePath, content)
}
```

### Step 6: Kotlin 层 — 删除旧的硬代码文件

- 删除 `service/src/main/java/com/github/kr328/clash/service/util/ProxyFilter.kt`（未提交的文件，不再需要）

### Step 7: Android UI — 脚本编辑界面

**新文件**: `app/src/main/java/com/github/kr328/clash/ScriptActivity.kt`

功能：
- 简单的文本编辑界面（EditText + 语法高亮可后续再加）
- 加载/保存当前 profile 的 `script.js`
- 提供"示例脚本"按钮，预填充 Clash Verge 风格的模板
- 显示脚本执行日志（可选，后续迭代）

**新文件**: `design/src/main/java/com/github/kr328/clash/design/ScriptDesign.kt`

Design 层处理 UI 逻辑。

### Step 8: 入口集成

在 Profile 详情页（`PropertiesActivity` 或 `ProfilesActivity`）添加"扩展脚本"入口按钮。

## 关键文件清单

| 文件 | 操作 |
|------|------|
| `core/src/foss/golang/go.mod` | 修改 — 添加 goja 依赖 |
| `core/src/main/golang/native/config/script.go` | **新建** — JS 执行引擎 |
| `core/src/main/golang/native/config/process.go` | 修改 — 删 patchProxyGroups，加 patchScript |
| `core/src/main/golang/native/main.go` | 修改 — 添加 JNI 导出 |
| `core/src/main/java/com/github/kr328/clash/core/Clash.kt` | 修改 — 添加桥接方法 |
| `service/.../util/ProxyFilter.kt` | **删除** |
| `app/.../ScriptActivity.kt` | **新建** — 脚本编辑 UI |
| `design/.../ScriptDesign.kt` | **新建** — 脚本编辑 Design |

## 验证方式

1. 编写一个测试脚本（过滤日本节点），放入 profile 目录的 `script.js`
2. 启动 App，加载该 profile，确认 proxy 列表已被过滤
3. 删除 `script.js`，确认恢复原始行为
4. 编写一个有语法错误的脚本，确认 App 不崩溃，回退到原始配置
5. 完整构建 `./gradlew app:assembleAlphaRelease` 确认编译通过

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| goja 增加库体积 | goja 约 5-8 MB 编译后体积，相比 65 MB 的 APK 可接受 |
| JS 执行耗时 | 加 5 秒超时保护；节点过滤脚本通常毫秒级完成 |
| 脚本错误导致配置损坏 | 执行失败时跳过 patchScript，继续后续处理器 |
| goja ES6 子集不够用 | 节点过滤场景只需 filter/map/正则，完全覆盖 |

## 示例脚本

### 基础：按关键词过滤节点

```javascript
function main(config) {
  const keywords = ["日本", "新加坡", "香港"];
  config.proxies = config.proxies.filter(p =>
    keywords.some(kw => p.name.includes(kw))
  );
  return config;
}
```

### 进阶：过滤 + 重建 proxy-groups

```javascript
function main(config) {
  const all = config.proxies.map(p => p.name);
  const jp = all.filter(n => /日本|JP/i.test(n));
  const sg = all.filter(n => /新加坡|SG/i.test(n));
  const hk = all.filter(n => /香港|HK/i.test(n));

  config["proxy-groups"] = [
    { name: "Proxy", type: "select", proxies: ["Auto", "JP", "SG", "HK", "DIRECT"] },
    { name: "Auto", type: "url-test", proxies: all,
      url: "http://www.gstatic.com/generate_204", interval: 300 },
    { name: "JP", type: "url-test", proxies: jp,
      url: "http://www.gstatic.com/generate_204", interval: 300 },
    { name: "SG", type: "url-test", proxies: sg,
      url: "http://www.gstatic.com/generate_204", interval: 300 },
    { name: "HK", type: "url-test", proxies: hk,
      url: "http://www.gstatic.com/generate_204", interval: 300 },
  ];
  return config;
}
```

### 排除垃圾信息节点

```javascript
function main(config) {
  const junk = ["剩余流量", "套餐到期", "邀请好友", "官网", "网址", "TG群"];
  config.proxies = config.proxies.filter(p =>
    !junk.some(kw => p.name.includes(kw))
  );
  return config;
}
```
