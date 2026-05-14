# Scribe

> 多平台视频 → 本地 Whisper 转写 → LLM 校对 + Typeless 风格智能词表。
> 基于 Wails v2 的原生桌面 App，macOS / Windows。

Scribe 把音视频转成高质量文字稿。下载完自动跑本地 Whisper（可切云 API），配合 LLM 校对 + 智能词表，逐渐把你的常用术语"喂"给工具——Typeless 风格的增量学习。

**本期 (v0.3)** 同时支持微信视频号（内置 MITM 代理，下载按钮直接注入到微信客户端）和 yt-dlp 驱动的通用 URL 下载（YouTube / B 站 / X / 抖音 / TikTok 等 1700+ 站点），两个来源在 Downloads 页统一展示，转写、词表、校对都共用。

UI 视觉对齐 [autogame-17/prism](https://github.com/autogame-17/prism)：窄边栏 + 主内容 + 卡片网格，Tailwind + shadcn/ui + Radix + lucide。

![Scribe](docs/screenshots/hero.png)

---

## 为什么不是 downloader

前身是 `sph-downloader`——一个把视频号下载 CLI 重包成桌面应用的小玩意。真跑起来之后发现：**下载只是半成品**。视频号里大量内容本质是"通过视频承载的一段话"，真正有价值的是那段话本身——字面、可搜、可剪、可改。Scribe 是把"下载"降级为工具链里的一步，把终态产物从 MP4 改成"能读、能改、能导出"的文字稿。

## 工作流

```
 视频号 / YouTube / B 站 / X / 抖音 / TikTok / ...
   ↓ 下载（wx_channel MITM 或 yt-dlp，根据来源自动分流）
 mp4 / m4a
   ↓ ffmpeg 抽音轨
 wav (16 kHz mono)
   ↓ whisper.cpp 本地推理
 segments + timestamps
   ↓ 确定性词表替换（种子 40+ 条 + 个人累积）
   ↓ LLM 校对（Claude / Gemini，v0.2c）
   ↓ 用户 accept → 回流进个人词表
 成稿 (md / srt) + 原视频里的 .zh.srt
```

## 架构

```
scribe-studio/
├── backend/
│   ├── core/                     # git subtree: ltaoo/wx_channels_download
│   │   └── pkg/sphkit/           # overlay: Start/Stop/ListTasks（绕 internal 壁垒）
│   └── scribe/
│       ├── app.go                # Wails App struct
│       ├── runtime/              # AppSupport 路径 + 二进制定位
│       ├── media/                # ffmpeg 抽音轨
│       ├── transcribe/           # Provider 接口 + LocalWhisperCpp
│       ├── models/               # whisper 模型下载管理
│       ├── external/             # yt-dlp 集成（probe / download / 状态机）
│       ├── pipeline/             # watcher + queue + 持久化状态（含 SourceExternal）
│       ├── proofread/            # LLM 校对 + 词表
│       ├── logbus/               # 实时日志环形缓冲区 + Wails 广播
│       └── transcripts.go        # Wails 绑定
├── frontend/                     # React + Vite + TS + pnpm
│   └── src/
│       ├── components/layout/    # Sidebar + Topbar
│       ├── components/ui/        # shadcn 风格 Card/Button/Badge
│       └── pages/                # Dashboard / Downloads / Transcripts / Logs / Settings / About
├── resources/bin/                # ffmpeg + whisper-cli + yt-dlp (.gitignore)
└── scripts/
    ├── fetch-bins.sh             # dev：brew install + 软链到 resources/bin
    ├── scribesmoke/              # go run -tags scribesmoke
    └── realsmoke/                # go run -tags realsmoke <video>
```

## 开发

### 依赖

- macOS（目前 v0.2a 只跑 mac；Windows 走 v0.2d 再说）
- Go 1.23+
- Node 20+ & pnpm
- Wails v2 CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Homebrew（fetch-bins 脚本用）

### 一次性 setup

```bash
./scripts/fetch-bins.sh           # 默认 dev 模式：brew install ffmpeg + whisper-cpp，软链到 resources/bin/
./scripts/fetch-bins.sh --release # 编静态二进制（evermeet ffmpeg + 源码编 whisper-cli），给 CI / 本地打包用
```

Whisper 模型从 App 内一键下载（设置 → 转写），不需要手动 curl。

### 开发循环

```bash
wails dev                          # 热更新 + DevTools
wails build                        # dev 构建，走 resources/bin/ 的 brew symlink
./scripts/build-release.sh v0.2.0  # release 构建：注入 ldflags + bundle 静态二进制进 .app
```

### 跑 smoke

```bash
# 只测 Whisper Go wrapper
go run -tags scribesmoke ./scripts/scribesmoke/main.go

# 对真视频走 ffmpeg + whisper 完整链路，输出 SRT
go run -tags realsmoke ./scripts/realsmoke/main.go path/to/video.mp4 base
```

## Roadmap

| 版本 | 范围 | 状态 |
|---|---|---|
| v0.1 | 视频号下载桌面封装（sph-downloader） | ✓ 完成 |
| v0.2a | 改名 Scribe、下载完成自动转写、Transcripts 页 | ✓ 完成 |
| v0.2b | `@uiw/react-md-editor` 轻量编辑器 + 种子词表 + srt/md 导出 | ✓ 完成 |
| v0.2c | LLM 校对 + SuggestionChip + Typeless 回流词表 + AI Settings | ✓ 完成 |
| v0.2d | macOS ldflags 注入 + 静态二进制 bundle + 模型下载 UI + release CI | ✓ 完成 |
| v0.2e | sphkit 解析修复 + 代理停止时仍可看下载历史 + Scribe 品牌图标 | ✓ 完成 |
| v0.3 | yt-dlp 集成（YouTube / B 站 / X / 抖音 等 1700+ 站点）+ Downloads 页 MediaSource 抽象 + 实时日志面板 + 自动转写 toggle | ✓ 完成 |
| v0.4 | Apple notarization + Intel mac / Windows binaries；Settings 代理 / 下载 tab 接入；可选 Whisper 量化模型 | ⏳ |

## License

**GPL-3.0-or-later**。详见 [`LICENSE`](LICENSE)。

为什么是 GPL：Wails 桌面二进制静态链接了 vendored 进来的 [GopeedLab/gopeed](https://github.com/GopeedLab/gopeed) fork（在 `backend/core/pkg/gopeed/`，GPL-3.0）。按 GPL-3.0 §5，combined work 必须以 GPL-3.0（或兼容许可证）整体发行——所以 scribe-studio 整仓都走 GPL-3.0-or-later。

各第三方组件保留各自原始许可（见 [`NOTICE.md`](NOTICE.md)）。例如 `backend/core/` 下来自上游 `ltaoo/wx_channels_download` 的文件保留其 "Commons Clause" + MIT 上游声明（描述这些**单个文件**的授权），但合成进 Scribe 二进制后整体仍以 GPL-3.0 发行。

### 商业许可（dual licensing）

如果 GPL-3.0 的义务（衍生品源码公开等）与你的用途冲突——典型场景如闭源 SaaS、私有发行——`autogame-17` 作为 Scribe 原创部分的版权所有者，欢迎商谈另外的商业授权。注意：dual licensing 只能覆盖 Scribe 自有代码 + 已签署 CLA 的贡献者代码；vendored 的 GPL/MIT 第三方组件仍受其上游许可约束（也就是说商业版若想完全脱离 GPL，需要把 gopeed 这类 GPL 依赖剥离或替换）。

联系方式：通过 GitHub `@autogame-17` 私信，或在 issue 里留 contact 邮箱。

## Credits

第一致谢：[ltaoo/wx_channels_download](https://github.com/ltaoo/wx_channels_download) —— 没有这套视频号 MITM + 注入脚本，Scribe 的下载侧就不存在。详见 [NOTICE.md](NOTICE.md) 的完整清单。
