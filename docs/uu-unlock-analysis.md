# UU Remote lock-screen input analysis

## Target and method

Analyzed the installed UU Remote **4.46.0** helper, `/Applications/UURemote.app/Contents/Helpers/UURemoteServer`, using [REA](https://github.com/morluto/rea) **6.5.0**, Ghidra **12.1.4** and OpenJDK **21**. The universal executable was reduced to its arm64 slice with `lipo` because the provider requires a thin Mach-O.

- Original SHA-256: `3f834f06c648b0bfeab93a6d2c0d101dfbc8e3dc3993a8c0390b37d13c4effbc`.
- arm64 SHA-256: `232d239d77f164f8a6d17036c981ec759f4ff8e94381d66b83f07b6d8d32dce6`.

Analysis used `open_binary` followed by targeted `analyze_function` calls through REA's stdio MCP server. The Ghidra analysis completed after disabling the expensive `Decompiler Switch Analysis`, `Objective-C Message Analyzer` and `Demangler Swift` analyzers and allowing a 900-second request budget. Strings and arm64 disassembly were used to identify Swift async continuation functions; decompilation then established the native calls and constants.

This is **static evidence**, not a recording of UU successfully unlocking this machine. Swift async continuations and calling conventions leave imperfect pseudocode, so argument values and native calls carry more weight than recovered local variable names. This investigation covers lock-screen input, not UU's network protocol or credential storage.

## Findings

The observed function family implements this sequence:

1. Find `loginwindow` among running applications, then obtain its `AXFocusedUIElement` using `AXUIElementCreateApplication` and `AXUIElementCopyAttributeValue`.
2. Post Delete (`0x33`) and wait about 100 ms; reacquire the focused element.
3. Set `AXValue` to an empty string and wait for length zero, with a one-second budget. A further 200 ms delay precedes the password write. The clear-wait result is not visibly checked in the recovered continuation.
4. Set `AXValue` to the supplied password. Wait up to two seconds for the field's character count to equal Swift `String.count`. The value reader accepts a string or an attributed string and polls every 50 ms.
5. Only after that check succeeds, post Return (`0x24`). The key helper creates both down and up events and holds the key for about 30 ms.
6. Confirm the unlocked state, using a seven-second budget and a one-second polling interval. The confirmation code consults multiple state helpers and a cached lock flag; it does not equate a successful key post with successful authentication.

The decisive difference from Mac2MQTT's previous implementation is **AX value replacement plus input confirmation**, instead of a fixed 1.5-second pause followed by blind character input. Direct AX replacement also avoids depending on the active keyboard input method.

### Evidence ledger

Addresses below are preferred arm64 virtual addresses, not runtime addresses with ASLR applied. Full response records and pseudocode extracts from this session are in `/tmp/mac2mqtt-uu-analysis/`.

| Observation | Function addresses | REA Evidence ID |
| --- | --- | --- |
| Unlock entry invokes AX injection | `0x1000b281c` | `ev_94b4cca514f442149a7195f1d7f83f58be953dbc74808d76861429aeb1a44c29` |
| Lookup by bundle identifier / loginwindow name | `0x1000b507c` | `ev_e0c81565e2da08309e654f951b3011d3f59b53881881c6f4d9df81110dcf4bed` |
| Get loginwindow's focused AX element | `0x1000b5338` | `ev_ef999cbec1ff90976f5661c50720811323826b4ade885cdc32b176ac1cff554e` |
| Delete before AX injection | `0x1000b5df8` | `ev_6440be31358a4c704abc2b53c19b375032508b679823029826ffa7cb450c8688` |
| Clear AXValue and request zero-length wait | `0x1000b6280` | `ev_3cdea683656ea60526ada651b1f43ffa454aef7db492a8bba0bb886699acf5aa` |
| Set password and request character-count wait | `0x1000b63b8` | `ev_c8aff56b1495a56d3087dbf4e22127050b075e68e36d7fea0adaac90f36d3c6a` |
| Read string/attributed string and poll its count | `0x1000b57f0` | `ev_71a9f8b129294d39f0422cf50c3f629c7e8a47e53edd26d8c5066cd1d39719ce` |
| Submit Return only after input confirmation | `0x1000b60e4` | `ev_1dde92513d522b271683c341a3adac7f84975d0025113ddc08833d3584079255` |
| Post key down, then wait 30 ms | `0x1000b5558` | `ev_a4e2e81e2f7623e3d2f35ffd957b52f55da7323d5379da31ddc49032f0c62140` |
| Start confirmation with 7-second / 1-second arguments | `0x1000b29e8` | `ev_d27143f1a451b0a138e703052ebc3f2ece187e6e1b2f81d2454c1e9ece35f6df` |
| Confirm state independently of keyboard posting | `0x1000b2f2c`, `0x1000b3288` | `ev_b23b0e3ab29f43591fc0ce6fe979bef0bc3341eb8bbc1912a0463f6d0cb3a2a8`, `ev_099fc8c68221ae5451a6b567ee62f348de7ba5f1759b8032d2548a28ccf4ed36` |

## Mac2MQTT implementation

`internal/system/unlock.go` now waits for the focused password field, confirms clearing, writes the password and confirms the expected composed-character count before submitting once. A rejected direct AX write can fall back to select-all/delete and Unicode events; this path must pass the same focus and value checks. No failed authentication is retried automatically.

`internal/system/unlock_darwin.go` implements the native calls through purego, without cgo. It accepts only a secure text field owned by a discovered loginwindow application, retains that field's identity, bounds AX messaging waits and releases per-attempt native objects. It reads character counts without copying the field's text into Go. Both plain and attributed AX values are supported; composed-character counting handles combining characters and emoji. This is a length check, not proof of password equality or authentication.

Additional Mac2MQTT design choices, **not claims about UU**:

- Require the current user's session to remain on the console before input and throughout confirmation.
- Stop on cancellation, missing field, changed focus, unknown session state or unconfirmed input.
- Serialize attempts across controller instances.
- Use a private CGEventSource and explicit flags to avoid inheriting modifier state; allocate both events before posting either.
- Poll unlock confirmation every 250 ms for up to seven seconds.
- Give MQTT screen commands a 20-second overall budget to include field readiness, input confirmation and authentication.

## Validation and limits

Regression tests cover leftover input, delayed/missing fields, AX acknowledgments with no actual value change, Unicode replacement, cancellation, manual unlocking, session switches, posting errors and overlapping requests. Native tests construct real CGEvents while intercepting posting, and exercise CoreFoundation's composed-character counting. The opt-in desktop probe reads permission/session state and checks that an unlocked desktop is not mistaken for a password field; it does not type, lock the screen or access stored credentials.

Validation completed with `CGO_ENABLED=0`:

- `go test ./...`, `go vet ./...`, and `go vet -tags mygo_noinspector ./...`.
- Builds and vet for Linux and Windows; a darwin/amd64 build.
- `MAC2MQTT_TEST_NATIVE_UNLOCK=1 go test ./internal/system -run TestNativeUnlockDesktopProbe -count=1 -v`.
- A local arm64 application bundle built with `go tool mygo build -skip-dmg -sign -`, then checked with `codesign --verify --deep --strict`.

The stripped release executable grew from 17,838,050 to 17,871,778 bytes: 33,728 bytes (0.19%). The local bundle is `/tmp/mac2mqtt-uu-analysis/build/darwin-arm64/Mac2MQTT.app`; no installed app was replaced.

A real lock/unlock cycle remains to be validated on the target macOS installation, especially whether its loginwindow exposes a focused secure field and a readable value length. If those checks cannot be satisfied, this implementation stops without submitting. FileVault's pre-boot login remains outside the running application's scope.
