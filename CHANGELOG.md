# Changelog

## 0.4.2

- Signed with a Developer ID and notarized by Apple: Mac2MQTT opens like
  any app, and macOS keeps its permissions, such as Local Network, across
  updates.
- When macOS's Local Network privacy keeps Mac2MQTT from a broker on the
  local network, which it reported as "no route to host", the window says
  what to allow, with a button to the setting.

## 0.4.1

- Lighter: idle in the menu bar, Mac2MQTT takes about a fifth of the
  memory it did, and starts no processes every few seconds. Volume, mute
  and the screen's lock are read without one, Now Playing by a single
  watcher, and closing the window frees what it held.
- Now Playing shows the playing app's icon.
- The menu bar's icon is the app icon's house.
- Home Assistant names the entities in the app's language, with English
  entity IDs that automations can rely on.
- The window opens at the top of its page, and each page keeps its own
  scroll.
- Install with Homebrew: brew install --cask faintghost/tap/mac2mqtt.

## 0.4.0

- A flat icon: a house in one white line on ink, with a signal inside.
- A new website, as flat: faintghost.github.io/mac2mqtt.

## 0.3.3

- A new icon: a house broadcasting, for a Mac reporting to its home.

## 0.3.2

- One instance at a time: opening Mac2MQTT again shows the running one's
  window, rather than starting another that would disconnect it at the
  broker.

## 0.3.1

- The tiles of This Mac share two equal columns: a last tile alone on its
  row, such as Display, was wider than the others.

## 0.3.0

- Mac2MQTT updates itself: it checks for new versions in the background and
  offers to install them, with **Check for Updates…** in the menus and the
  choice to update automatically in Settings.
- Next and previous show as unavailable, in the window and in Home
  Assistant, while what plays does not offer them, as a video without a
  playlist: they used to do nothing.
- Camera In Use and Microphone In Use are published only when the Mac has
  a camera or a microphone, built in or plugged in: a Mac mini without them
  has neither entity, and connecting one adds it at once.
- A page's title sits on the window with nothing behind it; content that
  scrolls under it fades out, with no edge.

## 0.2.2

- Stays connected over slow and flaky links: messages go out in batches,
  discovery is published again only when it changes or Home Assistant
  restarts, a failed report no longer drops the connection, and waking or a
  network change reconnects at once.

## 0.2.1

- Fixes a data race when a media command republished Now Playing.

## 0.2.0

- Camera and microphone in use, and whether the display is awake.
- Lock Screen, Sleep Display, Wake Display and Sleep buttons.
- Notifications and speech from Home Assistant, and running Shortcuts
  (off by default).
- Home Assistant's device names the Mac's model and macOS version.

## 0.1.0

- The first release: network, power, screen lock, sound and Now Playing,
  with volume, mute and playback controls, a media player, and a native
  window with Liquid Glass.
