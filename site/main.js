// The page's Chinese, and the latest release's download link.

const zh = {
  "nav.features": "功能",
  "nav.ha": "Home Assistant",
  "nav.install": "安装",
  "hero.eyebrow": "为 Home Assistant 打造 · macOS 13 及以上",
  "hero.title": "把你的 Mac，接入 Home\u00a0Assistant。",
  "hero.lede": "一个原生的 macOS 菜单栏应用：通过 MQTT 把 Mac 的状态上报给 Home Assistant，也让 Home Assistant 能控制它。",
  "hero.download": "下载 macOS 版",
  "hero.github": "在 GitHub 查看",
  "hero.fine": "免费开源 · Apple 芯片与 Intel",
  "features.eyebrow": "上报什么，控制什么",
  "features.title": "Mac 知道的一切，都变成实体。",
  "f.media.t": "正在播放",
  "f.media.d": "控制中心里显示的一切：音乐、Spotify、Chrome 里的视频，并能播放、暂停、上一首、下一首。",
  "f.sound.t": "音量与静音",
  "f.sound.d": "在 Home Assistant 中用滑块和开关控制，走 macOS 自身的音频设置。",
  "f.screen.t": "屏幕与显示器",
  "f.screen.d": "屏幕是否锁定、显示器是否唤醒，并提供锁屏、睡眠和唤醒按钮。",
  "f.camera.t": "摄像头与麦克风",
  "f.camera.d": "是否正在使用，两秒内上报：通话时点亮“会议中”灯。只上报你的 Mac 实际拥有的设备。",
  "f.notify.t": "通知与朗读",
  "f.notify.d": "Home Assistant 可以在 Mac 上弹出通知，或让它告诉你洗衣机洗好了。",
  "f.shortcuts.t": "快捷指令",
  "f.shortcuts.d": "在自动化中运行任意快捷指令，可附带文本输入。默认关闭，由你决定是否开启。",
  "f.power.t": "电源与网络",
  "f.power.d": "电源类型，笔记本的电量和充电状态，联网状态、空闲时长，以及 Mac 型号和 macOS 版本。",
  "f.reliable.t": "什么网络都稳",
  "f.reliable.d": "保留状态、遗嘱消息、批量发送、快速重连，即使走慢速 VPN 也稳定。还能自动更新。",
  "f.native.t": "小巧原生",
  "f.native.d": "常驻菜单栏的一个小巧二进制，中英双语，浅色深色，可登录时自动打开。",
  "ha.eyebrow": "为 Home Assistant 而生",
  "ha.title": "不用写 YAML，自动出现。",
  "ha.p1": "MQTT 自动发现会为你的 Mac 创建一个设备，以它命名，带上型号和 macOS 版本。按组选择要发布的内容，关闭的会立即从 Home Assistant 中移除。",
  "ha.l1": "真正的媒体播放器：通过 MQTT Media Player 集成，或使用应用为你生成的 universal 播放器配置。",
  "ha.l2": "按钮、开关和滑块，在 Mac 上即时生效。",
  "ha.l3": "只有 Mac 实际拥有的：Mac mini 没有电池实体，没有摄像头就不上报摄像头。",
  "auto.eyebrow": "自动化",
  "auto.title": "Mac 既是触发器，也是执行者。",
  "auto.p1": "通话时点亮门口的灯，全家离开时锁上 Mac，门铃响时暂停音乐，洗衣机洗完时听到提醒。",
  "install.eyebrow": "安装",
  "install.title": "三步，一分钟。",
  "s1.t": "下载",
  "s1.d": "打开磁盘映像，把 Mac2MQTT 拖到“应用程序”。首次打开时，右键点击它并选择“打开”。",
  "s2.t": "填写 Broker",
  "s2.d": "使用 Home Assistant 的 MQTT 集成所用的那个 Broker（开启自动发现）：地址、端口和账号。",
  "s3.t": "找到你的 Mac",
  "s3.d": "它会出现在 设置 → 设备与服务 → MQTT 中，可直接用于仪表盘和自动化。",
  "mygo.eyebrow": "基于 MyGo 构建",
  "mygo.title": "原生，用 Go 写成。",
  "mygo.p1": "Mac2MQTT 基于 EGOIST 的 MyGo 构建。这是一个 Go 桌面框架，界面全部自己绘制，连 Liquid Glass 都有：没有 WebView、不用 cgo、一个小巧的二进制，DMG 打包和签名自动更新开箱即用。",
  "mygo.link": "了解 MyGo",
  "footer.license": "MIT 许可证 · 与 Apple 及 Home Assistant 无关",
};

const english = {};
document.querySelectorAll("[data-i18n]").forEach((el) => {
  english[el.dataset.i18n] = el.textContent;
});

function setLanguage(lang) {
  const chinese = lang === "zh";
  document.documentElement.lang = chinese ? "zh-CN" : "en";
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    const key = el.dataset.i18n;
    el.textContent = chinese && zh[key] ? zh[key] : english[key];
  });
  // The screenshots in the page's language.
  document.querySelectorAll("picture[data-shot]").forEach((picture) => {
    const name = picture.dataset.shot;
    const suffix = chinese ? "zh" : "en";
    picture.querySelector("source").srcset = `assets/${name}-${suffix}-dark.png`;
    picture.querySelector("img").src = `assets/${name}-${suffix}-light.png`;
  });
  document.getElementById("lang").textContent = chinese ? "English" : "中文";
  try {
    localStorage.setItem("lang", lang);
  } catch {}
}

// ?lang=en or ?lang=zh, else the reader's last choice, else the browser's.
let lang = new URLSearchParams(location.search).get("lang");
if (lang !== "en" && lang !== "zh") {
  lang = "en";
  try {
    lang = localStorage.getItem("lang") || (navigator.language.startsWith("zh") ? "zh" : "en");
  } catch {}
}
if (lang === "zh") setLanguage("zh");
document.getElementById("lang").addEventListener("click", () => {
  lang = lang === "zh" ? "en" : "zh";
  setLanguage(lang);
});

// The universal disk image of the latest release, rather than the release
// page, when GitHub answers.
fetch("https://api.github.com/repos/FaintGhost/mac2mqtt/releases/latest")
  .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
  .then((release) => {
    const dmg = release.assets.find((a) => a.name.endsWith("-universal.dmg"));
    if (dmg) document.getElementById("download").href = dmg.browser_download_url;
    document.getElementById("version").textContent = release.tag_name;
  })
  .catch(() => {});
