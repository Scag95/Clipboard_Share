# Arch packaging

Builds the Nudge server and CLI (`nudge`) and the desktop panel
(`nudge-panel`) from this repository checkout. No network sources are used:
`prepare()` copies the sources from the parent directory into `src/`, so the
build leaves no artifacts in the repository tree.

The package was called `clipboard-share` before; `nudge` replaces, conflicts
with and provides it, so `pacman -U` swaps one for the other.

## Build

```sh
cd pkgbuild && makepkg -f
```

If Go is only available through mise, run
`mise exec go@1.24 -- makepkg -f` instead.

## Install

```sh
sudo pacman -U nudge-*.pkg.tar.zst
```

Installed files:

- `/usr/bin/nudge`, `/usr/bin/nudge-panel`
- `/usr/lib/systemd/user/nudge.service`
- `/usr/lib/udev/rules.d/99-nudge-uinput.rules`
- `/usr/share/applications/nudge.desktop`
- `/etc/xdg/autostart/nudge-panel.desktop` (panel starts hidden in the tray)
- `/usr/share/icons/hicolor/scalable/apps/nudge.svg`

## Starting at login

Both are on after installing, and each one can be turned off in the panel,
under **Ajustes**:

- **Iniciar el servidor al iniciar sesión** runs
  `systemctl --user enable nudge.service` or `disable`. The panel enables it
  once, on its first launch; after that it never turns it back on by itself.
- **Abrir el panel al iniciar sesión** uses XDG autostart: turning it off
  writes `~/.config/autostart/nudge-panel.desktop` with `Hidden=true`, which
  overrides the packaged entry; turning it on removes that file.

Opening the panel always starts the server if it is not running
(`systemctl --user start nudge.service`), whatever the first switch says.

By hand, without the panel:

```sh
systemctl --user enable --now nudge
```

The service starts with the graphical session and restarts if it fails.
Set `NUDGE_PORT` (default `8765`) with `systemctl --user edit nudge` if
needed; the old `CLIPBOARD_SHARE_*` variable names still work as a fallback.

## Upgrading from clipboard-share

The first run of the server, the CLI or the panel moves
`~/.config/clipboard-share` to `~/.config/nudge` when only the old directory
exists, so paired phones keep working. An existing `~/.config/nudge` is never
overwritten. The old unit is removed by the upgrade, so stop and disable it
first, before `pacman -U`, to leave no running process or dangling link:

```sh
systemctl --user disable --now clipboard-share.service
```

## Remote input (/dev/uinput)

The udev rule grants the logged-in session user access to `/dev/uinput`
(`uaccess`; see the note below). Apply it once after installing:

```sh
sudo udevadm control --reload && sudo udevadm trigger /dev/uinput
```

Without access to `/dev/uinput` the server still runs, but remote input is
disabled.

Note on `uaccess`: the rule is meant to give the active local session user access
to `/dev/uinput` without the `input` group. That was not verified on the
development machine: after installing the package and reloading udev the ACL on
`/dev/uinput` had no entry for the user, and access worked because the user is in
the `input` group (the node is `root:input 0660` by default). If remote input
runs in stub mode, add yourself to the `input` group and log in again.
