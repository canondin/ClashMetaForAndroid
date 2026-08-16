# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **必读**:构建、安装、同步操作前先阅读仓库根目录的 `AGENTS.md`(已提交,跨机器通用),
> 其中包含 release 构建命令的正确写法、cmake/mihomo 强推问题修复、签名一致性与 adb 无线安装流程。

## Build Commands

```bash
# Debug build (alpha flavor, arm64 only)
./gradlew app:assembleAlphaDebug

# Release build — MUST use the app: prefix; the unscoped task fails on
# library resource verification (hideapi cannot resolve @string/launch_name_alpha)
./gradlew app:assembleAlphaRelease

# Build specific module
./gradlew :core:assembleDebug

# Clean build
./gradlew clean app:assembleAlphaDebug
```

Build requires Java 21 (Temurin); system Go >= 1.22 is enough (mihomo go.mod requires 1.20). The Gradle build automatically compiles Go code via the `golang-android` plugin and downloads geo files (geoip, geosite, ASN).

## Project Architecture

ClashMetaForAndroid integrates the Clash.Meta (mihomo) Go core into an Android VPN app. The architecture is multi-process: the main app process handles UI, and a `:background` process runs VPN/Clash services.

### Module Responsibilities

- **app** — Activities, Application class, UI entry points
- **core** — Go↔Kotlin bridge (JNI via C), Clash core wrapper (`Clash.kt`, `Bridge.kt`)
- **service** — Background services (VPN, profile management, Room database, AIDL IPC)
- **design** — UI components, data binding layouts, dialog helpers
- **common** — Shared utilities, Android compatibility
- **hideapi** — Access to hidden Android system APIs

### Communication Flow

```
Kotlin Activities → design module (UI) → service module (AIDL IPC) → ClashService → core module (JNI) → C bridge → Go exports → mihomo core
```

### Go/Kotlin Bridge

- **Go exports** (`core/src/main/golang/native/*.go`) are annotated with `//export` and compiled to `libclash.so`
- **C layer** (`core/src/main/golang/cpp/*.c`) provides JNI bindings
- **Kotlin** (`core/.../bridge/Bridge.kt`) declares `external fun` counterparts
- Go submodules: mihomo core at `core/src/foss/golang/clash` (Alpha branch)

### Key Services

- `ClashService` — Manages Clash runtime lifecycle
- `TunService` — Android VPN service (extends VpnService)
- `RemoteService` — AIDL-based IPC between main and background processes
- `ProfileWorker` — Periodic profile updates

### Build Flavors

Two flavors: `alpha` (default) and `meta`. Both use the same Go source with tags: `foss`, `with_gvisor`, `cmfa`.

## Code Conventions

- Code style configured for Android Studio/IntelliJ (see CONTRIBUTING.md)
- Kotlin for all Android code, Go for core logic, C for JNI bridge only
- Go code uses `//export` comments for JNI-exported functions
- Data binding is used extensively in the design module

## Intent API

The app supports external control via intent actions: `TOGGLE_CLASH`, `START_CLASH`, `STOP_CLASH`, and URL schemes `clash://install-config?url=...` and `clashmeta://install-config?url=...`.

## Upstream Sync

Original repo: `https://github.com/MetaCubeX/ClashMetaForAndroid.git` (remote: `upstream`). To sync main:
```bash
git fetch upstream && git checkout main && git reset --hard upstream/main && git push --force origin main
```
