# Mac2MQTT

[![CI](https://github.com/FaintGhost/mac2mqtt/actions/workflows/ci.yml/badge.svg)](https://github.com/FaintGhost/mac2mqtt/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/FaintGhost/mac2mqtt)](https://github.com/FaintGhost/mac2mqtt/releases/latest)

[English](README.md) | 简体中文

一个原生的 macOS 菜单栏应用，通过 MQTT 把你的 Mac 接入 Home Assistant：网络、
电源、锁屏、声音和正在播放的内容，并可在 HA 中控制音量、静音和播放。借助 MQTT
自动发现，Home Assistant 会自动创建设备和实体。

<p align="center">
  <img src="docs/screenshots/overview-zh.png" width="49%" alt="概览">
  <img src="docs/screenshots/entities-zh-dark.png" width="49%" alt="实体（深色模式）">
</p>

基于 [MyGo](https://github.com/egoist/mygo) 构建：界面由应用自己绘制（包括
Liquid Glass），没有 WebView，只有一个小巧的二进制文件。

## 功能

- **Home Assistant 自动发现**：以你的 Mac 命名的设备，显示型号和 macOS 版本；
  实体按组自由选择是否发布。
- **正在播放**：控制中心里显示的一切（音乐、Spotify、Safari 或 Chrome 中的视频），
  包括标题、艺人、应用、进度和状态，并提供播放/暂停、上一首、下一首按钮。
- **真正的媒体播放器**：HA 的 MQTT 集成不支持 `media_player`，因此 Mac2MQTT 按
  [MQTT Media Player](https://github.com/bkbilly/mqtt_media_player) 集成的格式发布，
  并能为不使用 HACS 的用户生成 `universal` 媒体播放器配置。
- **声音**：在 HA 中设置音量和静音。
- **屏幕与会话**：屏幕是否锁定，以及空闲时长。
- **电源与网络**：电源类型；笔记本还有电量和充电状态（Mac mini、Mac Studio 不会创建电池实体）；联网状态。
- **可靠的 MQTT**：保留状态、带遗嘱的可用性、指数退避重连。关闭某组或修改 Topic 时，
  会清理 HA 中不该再显示的内容。
- **地道的 Mac 应用**：常驻菜单栏，可登录时打开，跟随系统的浅色/深色外观和语言
  （English、简体中文、繁體中文）。

## 安装

从 [最新 Release](https://github.com/FaintGhost/mac2mqtt/releases/latest) 下载磁盘映像：

| 文件 | 适用于 |
|---|---|
| `Mac2MQTT-<版本>-universal.dmg` | 所有 Mac |
| `Mac2MQTT-<版本>-arm64.dmg` | Apple 芯片（M1 及以后） |
| `Mac2MQTT-<版本>-amd64.dmg` | Intel |

打开后把 Mac2MQTT 拖到“应用程序”。需要 macOS 13 或更高版本。

应用暂未经过公证，首次打开时 macOS 会阻止：在“应用程序”中右键点击它并选择
**打开**，或在 **系统设置 → 隐私与安全性** 中允许。也可以清除隔离标记：

```sh
xattr -dr com.apple.quarantine /Applications/Mac2MQTT.app
```

## 配置步骤

1. 在 Home Assistant 中添加 **MQTT** 集成，保持自动发现开启（前缀 `homeassistant`）。
2. 从菜单栏打开 Mac2MQTT，进入 **设置**，填写 Broker 的地址、端口和账号，保存。
3. 设备会出现在 HA 的 **设置 → 设备与服务 → MQTT** 中。

在设置中开启 **登录时打开**，重启后也会继续上报。如果 macOS 提示，请在
**系统设置 → 通用 → 登录项** 中允许 Mac2MQTT。

## 实体

每一组都可以在 **实体** 页关闭，关闭后其实体会从 HA 中移除。

| 组 | 实体 |
|---|---|
| 网络 | 网络连接（二元传感器） |
| 电源 | 电源类型；笔记本另有电量和充电状态 |
| 屏幕与会话 | 屏幕锁定（二元传感器）、空闲时长 |
| 声音 | 音量（数值）、静音（开关） |
| 正在播放 | 播放状态、正在播放（标题，艺人/专辑/应用/进度作为属性）、播放/暂停、下一首、上一首按钮、媒体播放器 |
| 诊断 | 最后更新、运行时长、应用版本、主机名 |

另有一个始终发布的“刷新”按钮，用于立即上报状态。

### 媒体播放器

二选一：

- **使用 HACS**：安装 [MQTT Media Player](https://github.com/bkbilly/mqtt_media_player)，
  媒体播放器会自动出现在这台 Mac 的设备下，支持播放控制、进度、音量和静音。
- **不使用 HACS**：在 **实体 → Home Assistant 媒体播放器** 中点击 **复制 YAML**，
  粘贴到 `configuration.yaml` 后重启 HA。它使用 HA 自带的
  [universal 媒体播放器](https://www.home-assistant.io/integrations/universal/)。
  YAML 中的实体 ID 按 HA 默认规则生成（如 `sensor.mac_mini_now_playing`），如果你改过实体名，请相应修改。

## MQTT Topic

Topic 列表、状态文档格式和配置项说明见 [英文 README](README.md#mqtt-topics)。

## 工作原理

- **正在播放** 来自 MediaRemote，即控制中心背后的私有框架。从 macOS 15.4 起它只响应
  Apple 签名的进程，因此 Mac2MQTT 通过 Apple 签名的 `osascript` 运行一小段
  JavaScript for Automation 脚本来读取信息和发送控制命令。
- **锁屏状态** 启动时从窗口服务器的会话信息（`ioreg`）读取，之后由锁屏/解锁通知更新。
- **音量和静音** 使用 macOS 自身的音频设置。

## 开发

需要 macOS 和 Go 1.27.3 或更高版本。

```sh
go test ./...                 # 单元测试：使用模拟的 Broker 和命令，不会改动真实音量或播放
go tool mygo dev              # 实时重载运行
go tool mygo build            # 生成 build/darwin-arm64/Mac2MQTT.app 和 DMG
go tool mygo vet .            # MyGo 的检查
go tool mygo i18n check       # 检查翻译目录
```

修改 `locales/*.json` 后运行 `go tool mygo generate` 更新访问函数。

### 发版

修改 `mygo.json` 中的 `version`，提交后推送同名 tag：

```sh
git tag v0.2.0 && git push origin v0.2.0
```

Release 工作流会检查 tag 与 `mygo.json` 是否一致，构建 arm64、amd64 和 universal
三种 DMG，并连同 SHA-256 校验和一起发布。

## 限制

- 仅支持 macOS 13 及以上。
- 仅做了临时签名（ad hoc），未经公证（见 [安装](#安装)）。
- MediaRemote 是私有框架，未来的 macOS 可能会改变它。
- 暂不发布专辑封面：MediaRemote 只通过 `osascript` 无法调用的接口提供封面。
- 不支持 TLS 客户端证书。

## 许可证

[MIT](LICENSE)
