# Feature: input-control (mouse + keyboard remote from phone)

New feature on branch `feat/input-control`: control the PC's mouse and keyboard
from the Android phone over the LAN. Touchpad + keyboard, WebSocket transport,
PIN pairing auth, separate Android screen. Clipboard sync stays as-is.

## Specs

- **S1** — Original request, verbatim: "Crea una nueva rama del proyecto. vamos
  a trabajar en un función de poder controlar el mouse y teclado del ordenador
  con el telefono." Branch `feat/input-control` created from `main`. Scope:
  phone → PC mouse + keyboard control, in addition to existing clipboard sync.
- **S2** — Scope decision, verbatim: "Touchpad + teclado (recomendado)".
  Behavior: one finger = relative pointer move; tap = left click; long press or
  two-finger tap = right click; two-finger drag / pinch = scroll; plus a text
  bar to send typed text and shortcut keys (Ctrl+C, Ctrl+V, Enter, Esc, arrows).
- **S3** — Channel decision, verbatim: "WebSocket (recomendado)". Events travel
  as a continuous JSON stream over one WebSocket connection (gorilla/websocket
  on server, OkHttp WebSocket on Android). No HTTP polling for input.
- **S4** — Security decision, verbatim: "Pairing con PIN (recomendado)".
  Server generates a 6-digit PIN at startup and prints it to the console.
  `POST /pair {"pin":"<6 digits>"}` returns a session token; correct PIN → 200 +
  token, wrong or spent PIN → 401, clipboard data unchanged either way. The
  input WebSocket requires a valid token (query param `token`); missing/invalid
  token → HTTP 401 before upgrade. Clipboard endpoints stay unauthenticated for
  now (unchanged behavior).
- **S5** — UI decision, verbatim: "Pantalla separada (recomendado)". A new
  `RemoteControlActivity` ("Control remoto") in Android; MainActivity keeps its
  clipboard behavior and gains an entry point to it.
- **S6** — Wire protocol (contract, JSON objects over the WS connection):
  `{"type":"move","dx":int,"dy":int}` relative pointer delta;
  `{"type":"click","button":"left"|"right","mode":"tap"|"down"|"up"}`;
  `{"type":"scroll","dx":int,"dy":int}` wheel notches;
  `{"type":"key","key":string,"mods":["ctrl"|"alt"|"shift"|"win"]?}` named key
  press+release; `{"type":"text","chars":string}` unicode text (sent via
  KEYEVENTF_UNICODE). Unknown type, non-numeric or out-of-range fields →
  `{"error":"..."}` frame (connection stays open); invalid input is never
  injected. Rates are clamped server-side (max 120 events/s per connection).
- **S7** — Platform contract: injection uses Windows `SendInput` (user32.dll)
  via the standard `syscall` package (`NewLazyDLL`/`SyscallN`), no cgo, behind
  build tag `windows`. A non-Windows stub injector (`Recorder`) records events
  so the whole pipeline compiles and is testable on Linux/macOS.
  `GOOS=windows go build ./...` must pass.

## Tasks

| ID | Specs | Route | Commit |
|----|-------|-------|--------|
| T1 | S4 | parent inline: `internal/pair` PIN/token store + tests | `71a7de7` |
| T2 | S6, S7 | parent inline: `internal/input` events, Injector, stub + Windows SendInput + tests | `1b307e4` |
| T3 | S3, S4, S6 | parent inline: WebSocket `/ws/input` handler with token gate + rate clamp + tests | `1980e0f` |
| T4 | S1, S4 | parent inline: wire `cmd/server/main.go`, deps, README + PLAN updates | `a543431` |
| T5 | S3, S4 | parent inline: Android OkHttp WS client + pairing flow (PIN entry, token persistence) | `e272e68` |
| T6 | S2, S5 | parent inline: Android `RemoteControlActivity` touchpad + text/shortcuts, manifest, entry point | — |

## Log

- L1 — User, verbatim: "Crea una nueva rama del proyecto. vamos a trabajar en
  un función de poder controlar el mouse y teclado del ordenador con el
  telefono."
- L2 — User decisions (batch answer, verbatim): scope "Touchpad + teclado
  (recomendado)"; channel "WebSocket (recomendado)"; security "Pairing con PIN
  (recomendado)"; UI "Pantalla separada (recomendado)".
- L3 — Branch `feat/input-control` created from `main` at `31ed989` (working
  tree clean, ahead of origin/main by 1 commit, not pushed).
- L4 — Tooling: `go` shim needs explicit version on this box; used
  `/home/scag/.local/share/mise/installs/go/1.24.13/bin/go`. Slice counter
  counts from `origin/main` and includes pre-existing main commit `31ed989`;
  real branch slices: T1 = 378 lines.
- L5 — S7 amended: original draft said purego; purego exposes no Dlopen on
  Windows and its SyscallN differs, so the Windows path uses the standard
  `syscall` package instead. Contract preserved: SendInput, no cgo, build-tag
  isolated, Linux unaffected.
- L6 — Delivery strategy for accumulated slices: reviewable work-unit commits
  on `feat/input-control` (real branch lines: T1 378, T2 ~550, T3 ~260, T4
  ~120 docs+wire, T5 ~180, T6 ~600 incl. layout). The harness slice counter
  measures from origin/main and includes pre-existing main commit 31ed989,
  so it overstates branch size. Push/PR remains a user decision.
- L7 — Incident 21:37:34 +0100: an external `git checkout main` (reflog
  HEAD@{0}, not issued by this session) moved the working tree off the
  feature branch, hiding branch-only files and dropping the uncommitted doc
  edits from L6's first attempt. Recovery was non-destructive: `git checkout
  feat/input-control` carried the uncommitted Android work back; all commits
  verified intact via reflog; doc lines re-recorded here.
