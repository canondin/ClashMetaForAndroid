# 延迟测试问题分析

## 问题描述

在代理 Tab 点击闪电按钮进行延迟测试后，延迟结果没有正确更新到对应节点和节点组上。

## 根因分析

### 问题一：testURL 查询与实际测试 URL 不匹配

**位置**: `core/src/main/golang/native/tunnel/proxies.go:185-199`

```go
testURL := "https://www.gstatic.com/generate_204"  // 硬编码默认值
for k := range p.ExtraDelayHistories() {            // Go map 遍历，顺序随机
    if len(k) > 0 {
        testURL = k
        break
    }
}
Delay: int(p.LastDelayForTestUrl(testURL))
```

当用户脚本配置了自定义测试 URL（如 `http://1.1.1.1/generate_204`）时：
- 健康检查用 `http://1.1.1.1/generate_204` 测试，结果存入 `ExtraDelayHistories["http://1.1.1.1/generate_204"]`
- 查询时从 map 随机取一个 key，可能取到 `https://www.gstatic.com/generate_204`
- 该 key 从未被测试过，`LastDelayForTestUrl()` 返回 0
- UI 显示延迟为 0 或空

### 问题二：并发限制导致测试慢

**位置**: `core/src/foss/golang/clash/adapter/provider/healthcheck.go:132`

```go
b := new(errgroup.Group)
b.SetLimit(10)  // 最大并发 10
```

- Provider 间并发（`connectivity.go:30-38`）
- Provider 内代理并发限制 10
- 50 个代理需要 5 轮，每轮含网络 I/O

## 测试流程全链路

```
闪电按钮点击
  └─ ProxyDesign.requestUrlTesting()
      └─ Request.UrlTest(currentItem)           // 只测当前 tab
          └─ ProxyActivity: healthCheck(names[index])
              └─ Go: tunnel.HealthCheck(groupName)
                  └─ 对 group 的所有 Provider 并发执行
                      └─ provider.HealthCheck()
                          └─ HealthCheck.check()
                              └─ errgroup 并发 10 执行 URLTest
                                  └─ proxy.URLTest(ctx, url)
                                      └─ 存入 ExtraDelayHistories[url]
              └─ Request.Reload(index)
                  └─ queryProxyGroup()
                      └─ convertProxies()
                          └─ 从 ExtraDelayHistories 随机取 key 查延迟 ← BUG
                      └─ updateGroup() → UI 刷新
```

## 修复方案

### 方案 A（推荐，最小改动）

修改 `convertProxies`，遍历所有 key 取有非零延迟记录的 testURL：

```go
testURL := ""
bestDelay := uint(0)
for k := range p.ExtraDelayHistories() {
    if len(k) > 0 {
        d := p.LastDelayForTestUrl(k)
        if d > 0 && (testURL == "" || d < bestDelay) {
            testURL = k
            bestDelay = d
        }
    }
}
if testURL == "" {
    testURL = C.DefaultTestURL
}
```

### 方案 B（更彻底）

在 Provider/ProxyGroup 层面记住「最近一次健康检查使用的 URL」，查询时直接用这个 URL，避免猜测。

改动较大，需要修改多个接口。

## 测试验证

1. 编写脚本使用自定义 `http://1.1.1.1/generate_204` 作为健康检查 URL
2. 点击闪电按钮测试
3. 验证所有代理节点延迟值正确显示（非 0 且非空）
4. 切换不同 tab 测试，验证各组延迟独立正确
