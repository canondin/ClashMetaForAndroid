# AGENTS.md — CMFA fork 构建与部署须知(AI agent 必读)

> 面向所有机器上的 AI agent。执行构建 / 安装 / 同步操作前必读。
> 沉淀自 2026-08-16 release 构建与无线安装会话;历史背景见 `docs/build-records/`。

## 构建命令(最重要)

release 构建**必须带 `app:` 模块前缀**(与上游 CI 一致):

```bash
./gradlew app:assembleAlphaRelease   # release(非 debuggable,不被系统杀进程)
./gradlew app:assembleAlphaDebug    # debug
```

裸 `./gradlew assembleAlphaRelease` 会失败:Gradle 会对全部 6 个子模块执行 assemble,
`:hideapi:verifyAlphaReleaseResources` 等库模块资源校验任务被触发,而 flavor 的
`@string/launch_name_alpha` 只定义在 design 模块,hideapi 无依赖解析不到 → AAPT 报错。
debug 变体没有该校验任务,所以 debug 构建一直正常,问题被掩盖。

产物:`app/build/outputs/apk/alpha/release/cmfa-<version>-alpha-release.apk`(仅 arm64-v8a)。

## 已知坑与修复

### 1. cmake 版本脚本崩溃(mihomo 强推后)

- 症状:`:core` 的 externalNativeBuild 配置阶段 cmake exit 1(Windows 上常表现为长栈但无明确错误行)。
- 原因:`core/src/main/cpp/CMakeLists.txt` 用 `git submodule foreach git branch -r --contains <commit>`
  取分支名拼版本号;mihomo Alpha 分支会被强推/重建,锁定的子模块提交一旦不在任何
  `refs/remotes/origin/*` 上,列表为空 → cmake `list(GET ... 1)` 越界。
- 修复(每台机器一次,仅本地子模块仓库,不改任何项目文件):

```bash
SUB=$(git submodule status | awk '{print $1}' | tr -d ' +-')
git -C core/src/foss/golang/clash update-ref refs/remotes/origin/cmfa-pin "$SUB"
```

### 2. 签名一致性(跨机器关键)

- 仓库已提交 `release.keystore`;但 `signing.properties`(含 `keystore.password` /
  `key.alias` / `key.password` 三项)被 gitignore 且**每台机器要手动放一份**。
- 缺失时 release 构建回退 **debug 签名**(本机 debug keystore)。不同机器的 debug 签名互不兼容,
  跨机器装包必然 `INSTALL_FAILED_UPDATE_INCOMPATIBLE`,只能卸载重装 → 丢失应用内配置。
- 结论:每台构建机器放一份相同的 `signing.properties`,之后任何机器的构建都能直接覆盖更新。
  (旧版若是 debug 构建,卸载前可用 run-as 备份订阅,见下文;release 版不可备份,别走到那步。)

## 环境

- Java 21;mihomo `go.mod` 仅要求 go 1.20(系统 go ≥1.22 即可);NDK 29.0.14206865;
  flavor 默认 alpha;ABI 仅 arm64-v8a;包名 `com.github.metacubex.clash.alpha`。
- 切换提交 / 同步后先 `git submodule update --init --recursive`。

## 代码同步注意

`feature/proxy-group-filter` 的远程(origin)是权威,rebase 更新过。本地与远程分叉时
先 `git cherry -v origin/<branch> HEAD` 确认本地提交是否只是远程已含内容的旧基线版本,
是则直接 `git reset --hard origin/<branch>`;不要 merge / rebase(会产生重复补丁冲突)。

## adb 无线安装流程

1. 首次配对:手机 开发者选项 → 无线调试 → 使用配对码配对设备,然后
   `adb pair <ip>:<配对端口> <6位码>`(配对端口与连接端口不同,且每次打开弹窗都会变)。
2. 连接端口也会随无线调试服务重启变化;不要手动扫端口,用 mDNS 自动发现:
   `adb mdns services` → 输出形如 `adb-<serial> _adb-tls-connect._tcp <ip>:<port>`,
   之后可直接用该 mDNS 序列名作 `adb -s` 参数,断线重连也由 mDNS 兜底。
   注意:局域网里 5555 端口开放的不一定是目标手机(可能是别的 Android 设备的旧式 tcpip adb)。
3. 签名不兼容需卸载重装时,若旧版是 **debug 构建**(versionName 带 `.debug`),先备份订阅再卸载:

```bash
# 订阅 URL 明文存在 profiles 库(最新数据常在 -wal 里)
adb exec-out run-as com.github.metacubex.clash.alpha cat databases/profiles-wal \
  | grep -aoE "https?://[A-Za-z0-9./?&=%:_~-]+" | sort -u
```

4. 装完新版后用深链自动恢复订阅(URL 需整体百分号编码):

```bash
adb shell am start -a android.intent.action.VIEW \
  -d "clash://install-config?url=<URL编码后的订阅链接>"
```
