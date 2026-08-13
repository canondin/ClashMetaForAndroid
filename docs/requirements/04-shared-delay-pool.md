# 需求：同节点跨组延迟一致性（共享延迟池）

## 背景

用户反馈：**同一个代理节点在不同代理组内显示的延迟时间不一致**。例：

- 节点 `新加坡｜直连-01` 在 `延迟最低` 组显示 `85ms`
- 同一节点在 `新加坡` 组显示 `120ms`
- 同一节点在 `自动选择` 组显示 `95ms`

这会导致：

1. **节点选择判断失真**：用户切换组别时，看到同一节点延迟跳变，误以为是节点不稳定。
2. **测速资源浪费**：每个 url-test 组独立探测一次，N 个组 × M 个节点 = N×M 次探测；其中 (N-1)×M 次是冗余的。
3. **脚本配置难以调优**：用户无法判断到底哪次测速的延迟才是可信的。

## 现状分析

### 根因（已从代码层确认）

#### 1. 每个 url-test / fallback / load-balance 组都会创建独立的 `CompatibleProvider` + `HealthCheck`

`core/src/foss/golang/clash/adapter/outboundgroup/parser.go:155-185`：

- 当 url-test 组使用 `proxies:`（节点名列表）而不是 `use:` 时：
  - 第 176 行 `provider.NewHealthCheck(ps, groupOption.URL, ...)` 为这个组独立创建 `HealthCheck`
  - 第 178 行 `provider.NewCompatibleProvider(groupName, ps, hc)` 创建独立的 `CompatibleProvider`
- 每个 Provider 启动独立 goroutine 跑 `process()` ticker（`provider.go:65-69` + `healthcheck.go:44-62`），按各自 `interval` 触发 `check()`
- 因此 N 个 url-test 组 = N 套 `HealthCheck`，每套独立探测整个 `proxies` 列表

#### 2. URLTest 结果按 URL 维度分散存储在节点 `extra` map

`core/src/foss/golang/clash/adapter/adapter.go:38, 166-198`：

- 节点上有 `extra xsync.Map[string, *internalProxyState]`
- **key 是测速 URL 字符串**（trim 过），value 是 `ProxyState{Alive, History}`
- 默认 URL 的延迟存在 `p.history`（环形队列），额外 URL 的延迟存在 `p.extra[url].history`
- `URLTest` 一次探测会**同时写入默认桶 + 指定 URL 桶**（`adapter.go:176-198`）
- 节点上保留每个 URL 最近 10 条历史（`defaultHistoriesNum = 10`，`adapter.go:26`）

#### 3. url-test 组选节点时只看自己组的 testUrl

`core/src/foss/golang/clash/adapter/outboundgroup/urltest.go:108, 119, 127, 131`：

- 调用 `proxy.AliveForTestUrl(testUrl)` 和 `proxy.LastDelayForTestUrl(testUrl)`
- 组的 `testUrl` 来自 YAML 配置里该组的 `url:` 字段（默认 `C.DefaultTestURL`）
- 即两个组用不同 URL，节点上就存在两份独立的 `DelayHistory`，URL-test 选节点时只看自己那份 → **同一节点显示不同延迟**

#### 4. UI 侧已有缓解，但不够

`core/src/main/golang/native/tunnel/proxies.go:168-190`：

- 新增 `pickBestTestURL(p)`：遍历 `ExtraDelayHistories()`，找非零、非 0xffff 的最小延迟对应的 URL
- `convertProxies` (`proxies.go:210-218`) 用这个最优 URL 来展示延迟
- **副作用**：UI 显示的是"最低延迟"而不是"当前测试 URL 的延迟"，用户切组时以为延迟变了

#### 5. 关于"共享 Provider"的伪方案

调研结果澄清一个误区：

- 当多个组都 `use:` 同一个 Provider 时，确实只跑一份默认 URL 探测
- 但当一个组用 `url:` 指定额外 URL 时，会通过 `RegisterHealthCheckTask`（`parser.go:150` → `provider.go:99`）把额外 URL 写入 Provider 的 `hc.extra` map
- `healthcheck.go:139-143` 在同一次 `check()` 内同时遍历 `hc.url`（默认）+ `hc.extra`（额外），并发上限共享 `b.SetLimit(10)`（`healthcheck.go:132`）
- **结论**：即使 `use:` 共享 Provider，每个独立 URL 仍会独立探测一次；只是探测任务被合并到同一个 `HealthCheck` 实例里

#### 6. 关于 mihomo 上游 script 类型组（不可用）

调研澄清：

- mihomo 上游的 `script` 类型 ProxyGroup + JS 沙箱 API（`api.proxies`、`api.proxies[i].history`）**未合入本仓库 fork**
- 本仓库 `parser.go:187-220` 的 switch 只支持 `url-test / select / fallback / load-balance` 四种类型
- `component/` 目录下没有 `script/` 子包
- 因此"用 script 类型组读共享池 history 排序"的方案在本仓库**不可行**

## 目标

让**同一个节点在任何代理组内显示的延迟值是同一份历史数据**。

### G1: 单一延迟来源
同一节点上同一 URL 的延迟值只来自一次探测的写入。

### G2: 节点选择按区域隔离
- `新加坡` 组只在该区域子集里选最低
- `日本` 组只在该区域子集里选最低
- `延迟最低` 组在 sg + jp 子集里选最低

### G3: 减少冗余探测
N 个 url-test 组引用 M 个节点时，探测次数接近 M 而不是 N×M。

### G4: 不破坏现有脚本行为
- `unified-delay = true`
- `tcp-concurrent = true`
- 自定义 `direct rules` prepend
- `lazy = true`（手机端节流）

## 可行方案

| 方案 | 跨组一致性 | 探测次数 | 改动量 | 可行 |
|---|---|---|---|---|
| 当前（每组独立 url-test） | ❌ 无 | N×M | 0 | ✅ 现状 |
| A. 共享池 + url-test fallback | ⚠️ 部分 | ≈ N+M | 脚本 | ✅ 仅脚本侧 |
| B. 脚本侧 url 全部归一 | ✅ 同 URL 一致 | ≈ N+M | 脚本 | ✅ 仅脚本侧 |
| C. 内核：URL 归一 + 探测去重 | ✅ 彻底 | ≈ M | 中等 | ✅ 内核改 |
| D. 内核：UI 显示用本组 testURL | ⚠️ 显式区分 | ≈ N×M | 小 | ✅ 内核改 |
| ~~E. mihomo script 类型组~~ | ✅ 完全 | ≈ M | 大 | ❌ 上游未合入 |

### 方案 A：脚本侧共享延迟池

**原理**：构造一个隐藏的 `__SHARED_LATENCY__` url-test 组（带唯一 URL），所有需要延迟的组 `proxies` 数组首位放共享池，并保留子集作为 fallback。

**核心问题**：Clash Meta 对 `proxies:` 数组里**非池节点**仍会探测一次（`parser.go:176`），所以探测倍数仍是 N+M。

**只能减少部分冗余**，且无法跨组统一延迟值。

### 方案 B：脚本侧 URL 归一（推荐脚本侧方案）

**原理**：在 `script.js` 里把所有 url-test 组强制使用同一个 URL（共享 URL 字符串），让 `HealthCheck` 复用同一个 `extra[url]` 桶。

- `healthcheck.go:139-143` 会在一次 `check()` 内把 `hc.url` 和所有 `hc.extra[url]` 都探测
- 如果所有组的 URL 都是同一个字符串，**两次探测在节点 `extra[url]` 桶里写入的就是同一份延迟历史**
- 但是每个组仍有自己的 `HealthCheck` 实例，触发时间是独立的（不同 ticker）
- **结论**：延迟值会一致（因为写入同一个 bucket），但探测次数不变

```js
const SHARED_URL = "http://www.gstatic.com/generate_204";

function applyHealthCheck(group) {
  group.url = SHARED_URL;          // 强制统一 URL
  group.interval = ...;
  group.timeout = ...;
  group.lazy = ...;
  group.tolerance = ...;
}
```

**优点**：纯脚本侧改动，零内核改动，跨组延迟值完全一致。

**缺点**：探测次数不变（每个组仍独立探测一次）。

### 方案 C：内核侧 URL 归一 + Provider 间探测去重（推荐内核侧方案）

**改动点 1：URL 归一**

`core/src/foss/golang/clash/adapter/outboundgroup/parser.go:165-167`：

```go
// 当前：每个组用自己的 URL
if groupOption.URL == "" {
    groupOption.URL = C.DefaultTestURL
}

// 改为：归一为全局共享 URL（开启 unified-delay 时）
if config["unified-delay"] == true {
    groupOption.URL = SharedTestURL  // 全局变量，由 main 初始化时设置
}
```

或更精细：在 parser 里维护 `urlToHealthCheck map[string]*HealthCheck`，相同 URL 复用同一 HealthCheck。

**改动点 2：Provider 维度探测去重**

`core/src/foss/golang/clash/adapter/provider/healthcheck.go:128-148`：

```go
func (hc *HealthCheck) check() {
    if len(hc.proxies) == 0 { return }
    
    _, _, _ = hc.singleDo.Do(func() (struct{}, error) {
        id := utils.NewUUIDV4().String()
        b := new(errgroup.Group)
        b.SetLimit(10)
        
        // 新增：探测去重 —— 同一 (proxyName, url) 在 N 分钟内已被其他 Provider 测过则跳过
        skipRecent := config["skip-recent-probe"] != nil  // 例如 600s
        
        option := &extraOption{filters: nil, expectedStatus: hc.expectedStatus}
        hc.executeWithSkip(b, hc.url, id, option, skipRecent)
        
        if len(hc.extra) != 0 {
            for url, option := range hc.extra {
                hc.executeWithSkip(b, url, id, option, skipRecent)
            }
        }
        _ = b.Wait()
        return struct{}{}, nil
    })
}
```

需要在 `proxyProvider` 之上引入一个共享层（包级 `xsync.Map[proxyName+url]time.Time`）记录最近探测时间。

**优点**：
- 跨组延迟值完全一致（同一 URL 桶）
- 探测次数降到 ≈ M（同一 URL 只测一次）
- 改动可控，复用现有 `URLTest` 路径

**缺点**：
- 需要修改内核 Go 代码
- 需要新增全局配置项（`unified-delay` 已有，再加 `skip-recent-probe`、`shared-test-url`）

### 方案 D：UI 显式区分"本组延迟" vs "全局最优延迟"

**改动点**：`core/src/main/golang/native/tunnel/proxies.go:192-223`

把当前 `pickBestTestURL` 逻辑改为可选：

```go
func convertProxies(proxies []C.Proxy, groupTestURL string, uiSubtitlePattern *regexp2.Regexp) []*Proxy {
    ...
    // 优先用本组 testURL；找不到再降级到 ExtraDelayHistories 最优
    testURL := groupTestURL
    if testURL == "" {
        testURL = pickBestTestURL(p)
    }
    delay := p.LastDelayForTestUrl(testURL)
    if delay == 0 || delay == 0xffff {
        // 降级：尝试其他 URL
        alt := pickBestTestURL(p)
        if alt != "" {
            delay = p.LastDelayForTestUrl(alt)
        }
    }
    ...
}
```

调用方传入 `groupTestURL = provider.HealthCheckURL()`。

**优点**：
- 用户切换组时看到的就是"本组用的 URL 测出的延迟"，符合直觉
- 与脚本侧 URL 归一（方案 B）天然兼容
- 改动集中在 `convertProxies`，风险低

**缺点**：
- 不解决"探测次数 N×M"问题

### ~~方案 E：mihomo script 类型组~~（不可行）

本仓库 fork 不含 script 类型组实现。需要：
- 从上游 `MetaCubeX/mihomo` 合入 `component/script/` 包
- 在 `parser.go:187-220` 增加 `case "script":`
- 增加 JS 运行时（goja / starlark）

风险大、不在本期范围。

## 推荐组合方案

**内核层（方案 C + D）+ 脚本层（方案 B）**

### 内核改动

1. **URL 归一**（`outboundgroup/parser.go:165-167`）：开启 `unified-delay` 时归一为同一 URL
2. **Provider 探测去重**（`provider/healthcheck.go:128-148`）：同一 `(proxyName, url)` 在 N 秒内已被测过则跳过
3. **UI 显式本组 URL**（`tunnel/proxies.go:192-223`）：传入 `groupTestURL`，找不到再降级到 ExtraDelayHistories 最优

### 脚本改动

1. **强制所有 url-test 组用同一 URL 字符串**（即方案 B）
2. **构造隐藏的共享延迟池组**（即使内核未改，脚本侧也能减少一组探测）

## 实现拆解

### S1: 脚本侧实现（用户 script.js）

修改 `main(config)`：

```js
const SHARED_URL = "http://www.gstatic.com/generate_204";

function applyHealthCheck(group, withTolerance) {
  group.url = SHARED_URL;  // 强制统一 URL，跨组延迟值会落到同一桶
  group.interval = ...;
  group.timeout = ...;
  group.lazy = ...;
  if (withTolerance) group.tolerance = ...;
}

// 在 main() 里：
// 1. 收集 sgDirect / jpDirect / usDirect / krNodes，去重
// 2. 推入隐藏的 __SHARED_LATENCY__ url-test 组（仅一份）
// 3. 所有 url-test / fallback / load-balance 组都强制 SHARED_URL
```

### I1: 内核侧 URL 归一

**改动文件**：`core/src/foss/golang/clash/adapter/outboundgroup/parser.go:165-167`

```go
// 旧
if groupOption.URL == "" {
    groupOption.URL = C.DefaultTestURL
}

// 新
if config.UnifiedDelay {
    groupOption.URL = SharedTestURL  // 全局共享 URL
} else if groupOption.URL == "" {
    groupOption.URL = C.DefaultTestURL
}
```

`SharedTestURL` 在 `config` 初始化时设置（默认值 `C.DefaultTestURL`），用户可通过配置覆盖。

### I2: 内核侧 Provider 探测去重

**改动文件**：`core/src/foss/golang/clash/adapter/provider/healthcheck.go`

新增包级变量：

```go
var (
    probeCache     xsync.Map[string, time.Time]  // key: proxyName+url
    skipRecentSecs uint32                         // 默认 600（10 分钟）
)
```

`check()` 内对每个 `(proxy, url)` 探测前查缓存：

```go
func shouldSkip(proxyName, url string) bool {
    if skipRecentSecs == 0 { return false }
    key := proxyName + "|" + url
    if t, ok := probeCache.Load(key); ok {
        return time.Since(t) < time.Duration(skipRecentSecs)*time.Second
    }
    return false
}
```

探测成功后写入 `probeCache.Store(key, time.Now())`。

### I3: UI 显式本组 testURL

**改动文件**：`core/src/main/golang/native/tunnel/proxies.go:192-223`

```go
func convertProxies(proxies []C.Proxy, groupTestURL string, uiSubtitlePattern *regexp2.Regexp) []*Proxy {
    result := make([]*Proxy, 0, 128)
    for _, p := range proxies {
        ...
        testURL := groupTestURL
        if testURL == "" {
            testURL = pickBestTestURL(p)
        }
        delay := p.LastDelayForTestUrl(testURL)
        // 降级：本组 URL 没数据时尝试其他 URL
        if delay == 0 || delay == 0xffff {
            if alt := pickBestTestURL(p); alt != "" {
                delay = p.LastDelayForTestUrl(alt)
            }
        }
        ...
    }
}
```

调用方：

```go
// QueryProxyGroup
groupTestURL := provider.HealthCheckURL()  // 已经是 Provider 接口方法
result := convertProxies(proxies, groupTestURL, uiSubtitlePattern)
```

## 验收标准

### 功能验收

- [ ] 切换不同 tab 查看同一节点，延迟值不变化（方案 B 或 C+D 生效）
- [ ] `延迟最低` 组内排序按 sg+jp 区域子集延迟排序
- [ ] `新加坡` 组只在该区域子集内选最低
- [ ] UI 显示的延迟值与本组 `healthcheck.url` 探测结果一致（方案 D 生效）

### 性能验收

- [ ] 开启 `unified-delay: true` 后，logcat 中 `URLTest` 调用次数降为接近 4 次（去重后）
- [ ] 共享池探测间隔生效，手机端 4h、桌面端 2h
- [ ] lazy 生效，无流量时不探测
- [ ] `skip-recent-probe` 生效，10 分钟内同节点不重复探测

### 兼容性验收

- [ ] 移动端（mihomo Android）方案 B 脚本侧生效
- [ ] 内核方案 C 升级到 clash-meta-for-android 后生效
- [ ] 老配置（无 `unified-delay`）行为不变

## 待讨论

1. **方案 C 是否需要新增 `skip-recent-probe` 配置**？还是硬编码 600s？
2. **`SharedTestURL` 默认值**？`C.DefaultTestURL`？还是允许用户脚本注入？
3. **方案 D 是否保留 `pickBestTestURL` 降级**？还是只用本组 URL，找不到就显示 0？
4. **方案 C 的探测去重 cache 是否需要持久化**？目前进程内即可，订阅刷新后 cache 失效。
5. **mihomo 上游 script 类型组何时合入**？本仓库是否需要单独 cherry-pick？

## 核心修改链路（第二步实施前复核）

本节用于先判断“改完后能否达到目的”。本次第二步不改变用户脚本，也不把 `script` 引入内核；核心链路是：

```text
配置文件
  └─ config.parseProxies
       └─ 读取 skip-recent-probe
            └─ provider.SetProbeDedupWindow(window)

用户点击组测速
  └─ Clash.healthCheck(group)
       └─ tunnel.HealthCheck(group)
            └─ provider.HealthCheck()
                 └─ HealthCheck.check()
                      ├─ execute(default URL)
                      └─ execute(extra URLs)
                           └─ 对每个 (proxy, url, expectedStatus)
                                ├─ probeCache 未命中 → 真正调用 p.URLTest()
                                │                    └─ 写入 p.extra[url]
                                └─ probeCache 命中 → 跳过探测，直接复用已有延迟历史
```

### 1. 现有链路：为什么同节点跨组会得到不同结果

```text
节点 P1 被两个组引用

  Group A: 新加坡
    URL = u1
      └─ Provider A.HealthCheck.execute(u1, [P1, P2])
           └─ p1.URLTest(u1) → P1.extra[u1] = 85ms

  Group B: 日本
    URL = u2
      └─ Provider B.HealthCheck.execute(u2, [P1, P3])
           └─ p1.URLTest(u2) → P1.extra[u2] = 120ms

  UI 查询 Group A
    └─ pickBestTestURL(P1) / 或组 URL 查询
         └─ 显示 P1.extra[u1] = 85ms

  UI 查询 Group B
    └─ 查询 P1.extra[u2] = 120ms
         └─ 显示 P1 = 120ms
```

因此，**不同组使用不同 `url` 时，延迟不一致是由 URL 分桶本身决定的**，不是 UI 排序造成的。相同节点相同 URL 时，理论上会共享同一桶；不同 URL 不可能通过 Provider 去重变成同一份数值。

### 2. 探测去重后的链路

`skip-recent-probe: 600` 生效时，cache key 为：

```text
proxy.Name + "\x00" + url + "\x00" + expectedStatus
```

```text
第一次：Group A / u1
  P1: cache miss
    └─ URLTest(P1, u1) → extra[u1] = 85ms → cache 记录 t0

  P2: cache miss
    └─ URLTest(P2, u1) → extra[u1] = 92ms → cache 记录 t0

第二次：Group A / u1（t0 + 300 秒）
  P1: cache hit，且 t0 + 300 < t0 + 600
    └─ skip，不产生新的 URLTest

  P2: cache hit，且 t0 + 300 < t0 + 600
    └─ skip，不产生新的 URLTest

第三次：Group A / u1（t0 + 601 秒）
  P1: cache miss
    └─ URLTest(P1, u1) → extra[u1] = 最新值 → cache 记录 t1
```

### 3. 本次能解决什么、不能解决什么

| 问题 | 探测去重是否解决 | 原因 |
|---|---:|---|
| 同组重复 ticker 在窗口内重复探测 | ✅ | 同 `(proxy,url,expectedStatus)` 命中 cache |
| 多组恰好使用相同 URL 时重复探测 | ✅ | 包级 cache 跨 `HealthCheck` 实例共享 |
| 不同 `url` 桶被重复探测 | ❌ | key 不同，必须分别测速 |
| 不同组因 URL 不同显示不同延迟 | ❌ | UI 查询的是不同 `extra[url]` |
| 不同 `expectedStatus` 的错误探测 | ❌ | key 包含 expectedStatus，不复用结果 |
| 探测结果在不同组之间同步 | ⚠️ 仅相同 URL/状态码 | 底层延迟历史按 URL 分桶 |

### 4. 要达到“所有组同一节点数值完全一致”，还需要的必要链路

必须在后续方案中再增加“统一测速来源”：

```text
所有组（延迟最低 / 新加坡 / 日本 / 自动选择）
  └─ 统一指向同一个测速 URL
       └─ 同一 HealthCheck 或共享 probe cache
            └─ p1.URLTest(u_shared)
                 └─ P1.extra[u_shared]
                      └─ 所有组通过 u_shared 查询延迟
```

可选实施方式：

1. **配置层 URL 归一（Layer 1）**：开启统一开关后，让各组使用同一个 `shared-test-url`；实现简单，但要明确它会改变用户显式 `url` 的语义。
2. **真正的共享 HealthCheck**：构造一个全局 provider/URLTest 池，各组只引用池，不在 `proxies` 中重复列节点；这是最符合“单次测速、更新所有组”的模型。
3. **持久化延迟快照**：由一个统一测速任务完成后把同一 `DelayHistory` 复制到各组使用的桶；需要改变现有数据模型，复杂度最高。

本次先实施的是第 2 步中的**Provider 间探测去重**，它解决“测得太多、不同时间窗口造成抖动”，但不宣称单独解决不同 URL 桶的显示差异。

### 5. 关键代码位置

| 链路环节 | 文件/位置 | 作用 |
|---|---|---|
| 配置文件解析 | `core/src/foss/golang/clash/config/config.go` | 读取 `skip-recent-probe` |
| 真正发起探测 | `core/src/foss/golang/clash/adapter/provider/healthcheck.go:150-189` | 每个 proxy/URL 调 `URLTest` |
| 同实例防重入 | `healthcheck.go:128-147` | 1 秒内阻止同一 HealthCheck 并发重复执行 |
| 延迟结果分桶 | `core/src/foss/golang/clash/adapter/adapter.go:38,166-198` | 按 URL 保存 `extra[url]` |
| 组内最低节点选择 | `core/src/foss/golang/clash/adapter/outboundgroup/urltest.go:103-149` | 读取组 URL 的 alive/delay |
| UI 延迟显示 | `core/src/main/golang/native/tunnel/proxies.go:168-223` | 选择历史桶并回传 UI |

### 6. 配置建议

```yaml
# 开启 10 分钟窗口的同节点同 URL 探测去重
skip-recent-probe: 600

# 手机端建议结合组自身 interval，避免测速过于频繁
# url-test 组继续保留自己的 interval/lazy/tolerance
```

默认值必须为 `0`，这样未配置该字段的用户行为完全不变；只有明确设置 `skip-recent-probe` 后才启用去重窗口。

### 7. 验收链路（可据此判断目标是否达成）

```text
A. 开启 skip-recent-probe=600
B. 构造两个同 URL、不同 HealthCheck 的组
C. 分别触发两次 healthCheck
D. 统计每个 proxy 的 URLTest 调用次数
E. 预期第二次（窗口内）调用次数不再增加
F. 读取 P1.LastDelayForTestUrl(u)
G. 预期得到同一历史桶的最近有效延迟
```

必须额外验证：

- 同 URL：去重生效，结果来自同一 `extra[url]` 桶。
- 不同 URL：允许各自探测，不承诺数值相同。
- 不同 expected status：允许各自探测，不复用结果。
- `skip-recent-probe=0`：不改变原有测速行为。
- cache 清理后：能够重新执行探测并刷新延迟历史。

## 实施记录

### 第一阶段：Provider 探测去重（已完成）

落地位置：mihomo submodule（`core/src/foss/golang/clash/`）

**新增文件**

- `adapter/provider/healthcheck_dedup.go`
  - `probeCache`：包级 `xsync.Map[string, time.Time]`，跨所有 HealthCheck 共享
  - `probeDedupWindow`：`atomic.Int64`（纳秒），0 表示关闭
  - `SetProbeDedupWindow(d)` / `ResetProbeCache()`：对外开关与重置
  - `shouldSkipProbe(name, url, expectedStatus)` / `markProbeDone(...)`：判定与回写
  - key 形态：`name + "\x00" + url + "\x00" + expectedStatus.String()`

- `adapter/provider/healthcheck_dedup_test.go`
  - 9 个测试，覆盖：默认关闭、窗口内跳过、跨实例去重、不同 url/proxy/status 不跳过、窗口过期、cache 重置、execute() 端到端

**修改文件**

- `adapter/provider/healthcheck.go`：`execute()` 在 `b.Go` 前调 `shouldSkipProbe`（命中则 continue），在 `b.Go` 内 `URLTest` 后调 `markProbeDone`；跳过时打 debug 日志
- `config/config.go`：`RawConfig` 新增 `SkipRecentProbe uint16 yaml:"skip-recent-probe"`；`parseProxies` 末尾 `ResetProbeCache()` + `SetProbeDedupWindow(cfg.SkipRecentProbe * time.Second)`

**行为说明（实测确认）**

- 探测无论成功或失败都会 `markProbeDone`：死节点在窗口内不会被多个组反复重测（符合"减少冗余探测"目标）
- 同一 HealthCheck 实例的 `check()` 受 `singleDo`（1 秒）额外约束，与探测去重是两层独立机制
- `parseProxies` 每次加载都会 `ResetProbeCache()`，避免旧节点的 cache 抑制新同名节点

**验证**

```
go test ./adapter/provider/...  PASS (9/9，含 -race)
go test ./adapter/...           PASS
go build ./config/...           PASS
go vet ./adapter/provider/...   PASS
:core:externalGolangBuildAlphaDebugArm64V8a   BUILD SUCCESSFUL
:core:assembleAlphaDebug                       BUILD SUCCESSFUL
```

**配置用法**

```yaml
# 0 或缺省：保持原行为
skip-recent-probe: 0

# 10 分钟窗口（桌面端推荐）
skip-recent-probe: 600

# 4 小时窗口（手机端节流）
skip-recent-probe: 14400
```

**待办**

- ~~submodule 改动当前为本地 detached 提交（`12119ace`），未推送。~~ **已完成**：fork 到 `canondin/mihomo`，分支 `feature/skip-recent-probe`，submodule 已切到该分支。
- 若后续要达成"不同 URL 也数值一致"，仍需第二阶段 URL 归一（Layer 1）或共享 HealthCheck，本阶段不涉及。

### submodule fork 工作流（mihomo 本地补丁）

submodule 保留两个 remote：

| remote | 仓库 | 用途 |
|---|---|---|
| `origin` | `MetaCubeX/mihomo`（上游，只读） | 拉取最新 Alpha，跟随上游 |
| `fork` | `canondin/mihomo`（个人 fork） | 推送本地补丁分支 |

补丁分支：`feature/skip-recent-probe`（基于上游 `origin/Alpha`，含 `12119ace`）

**跟随上游同步 Alpha 的流程**

```bash
cd core/src/foss/golang/clash
git fetch origin
git rebase origin/Alpha              # 把补丁 rebase 到最新 Alpha
go test ./adapter/provider/...        # 验证
git push fork feature/skip-recent-probe --force-with-lease
cd -                                  # 回到项目根
# foss / main 各跑一次 go mod tidy
git add core/src/foss/golang/clash core/src/foss/golang/go.mod core/src/foss/golang/go.sum core/src/main/golang/go.mod core/src/main/golang/go.sum
git commit -m "build(core): sync mihomo Alpha to <commit>"
```

注意：`git submodule update --remote` 会用 `.gitmodules` 的 `branch = Alpha` 从 `origin`（上游）取最新，会**脱离补丁分支**。本仓库不要对该 submodule 用 `--remote`，改用上面的 rebase 流程。

## 相关文件

- 用户脚本：`script.js`（Profile 配置目录）
- 现有需求文档：`docs/requirements/01-proxy-group-filter.md`
- 现有测速分析：`docs/requirements/03-delay-test-analysis.md`
- 内核 ProxyGroup 实现：
  - `core/src/foss/golang/clash/adapter/provider/healthcheck.go`（`check()` 在 line 123-148，`b.SetLimit(10)` 在 line 132）
  - `core/src/foss/golang/clash/adapter/provider/provider.go`（`baseProvider.Initial()` 在 line 65-69）
- 内核 parser：
  - `core/src/foss/golang/clash/adapter/outboundgroup/parser.go`（创建 `HealthCheck` 在 line 176）
- 内核 adapter（节点级延迟存储）：
  - `core/src/foss/golang/clash/adapter/adapter.go`（`extra` map 在 line 38，`URLTest` 在 line 166-198）
- 内核常量定义：
  - `core/src/foss/golang/clash/constant/adapters.go:152-172`（`ProxyState`、`DelayHistory`、`C.Proxy` 接口）
- 内核 Provider 接口：
  - `core/src/foss/golang/clash/constant/provider/interface.go:66-87`（`HealthCheckURL()` 在 line 86）
- UI 侧延迟查询：
  - `core/src/main/golang/native/tunnel/proxies.go:168-190`（`pickBestTestURL`）与 `192-223`（`convertProxies`）