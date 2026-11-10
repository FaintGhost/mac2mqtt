# Changelog

## Unreleased

- The welcome finds the broker on your network: Home Assistant's host,
  where its Mosquitto add-on listens, and brokers that announce
  themselves. One click puts it into Settings.
- Test Connection in Settings tries the settings as edited, beside the
  reporting connection, and says what went wrong.
- An Audio Output select chooses where sound plays, the speakers or the
  headphones, as the Sound settings do; the Volume tile names it.
- Home Assistant knows at once when the Mac goes to sleep: it says it is
  offline as it sleeps, and online again as it wakes, rather than after
  the broker gives up on it.
- A Displays sensor counts the displays connected, so that one switched
  off at the plug tells from one asleep; the Display tile says which, and
  how many are connected.

## 0.4.10

- A calmer window: the light behind the glass is blues of one family,
  the accent's, rather than blue, teal and pink.
- Motion: the sidebar's selection slides to the page chosen, and the
  page rises in; numbers roll to their new values, states and icons fade
  across, and rows that come and go open and close. With Reduce Motion
  on, things fade in place.
- The first launch opens on a welcome that explains the setup and leads
  to the broker's settings, rather than on Settings.
- Settings says "yesterday" for an update check a day ago, not "30 hours
  ago".
- Clearer text: the welcome's last step says settings save on their own,
  About lists what the app reports, and Display's tile no longer cuts off.
- Updates download only what changed since the installed version, when
  it is one of the last 3: usually a small part of the whole app.

## 0.4.9

- Reports keep their interval while the window is closed: macOS no longer
  puts the app into App Nap while it reports, and the Mac still sleeps
  when idle.
- Checking for updates asks again when the server fails for a moment.

## 0.4.8

- The menu bar's menu shows the connection's state over the broker's
  address, and the song over its artist, each on two lines.
- Settings says when updates were last checked as "5 minutes ago" or
  "yesterday", in the app's language.
- Updating removes the files the new version no longer has.

## 0.4.7

- The display reads off when its monitor has no power, as one on a smart
  plug that is switched off, not only when it sleeps.
- Screen Lock replaces Screen Locked in Home Assistant: it reads Locked or
  Unlocked, with a lock that closes and opens, where Screen Locked read On
  while the screen was locked. Its entity is new
  (binary_sensor.<device>_screen_lock, on while unlocked): update the
  automations that used Screen Locked.

## 0.4.6

- A brighter, more even look: badges and states in bright colors of one
  lightness, flat badges, a quieter sidebar, page titles level with the
  app's name, and This Mac in three columns.
- Overview's status and Now Playing are Liquid Glass over soft light, and
  pages fade out under their title as they scroll.
- Entities opens on a summary: how many entities Home Assistant has from
  this Mac, and how many groups are on and off.
- Settings save as you change them, as macOS's do: a field saves when you
  leave it, and there is no Save button to press.
- When an update is available, the menu bar's menu, Settings and About say
  which version, with a button to update to it.
- The app icon's body has a faint depth.

## 0.4.5

- Home Assistant gets the Mac's uptime, how long ago it started, sleep
  included, beside the app's own.

## 0.4.4

- Every item of the menu bar's menu has its icon, so that their labels
  line up in one column.

## 0.4.3

- The menu bar's menu shows icons beside its items, as macOS 26's menus
  do, and the status with a dot in its color.
- A Control-click on the menu bar's icon opens its menu, as a right-click
  does.
- Updates replace the app once it has quit, so the Dock and Login Items
  keep it; a check in the background shows its window as Sparkle does.
- Install from Terminal, with the update's signature checked:
  curl -fsSL https://github.com/FaintGhost/mac2mqtt/releases/latest/download/install.sh | sh

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
