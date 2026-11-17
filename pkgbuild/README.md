# Arch packaging

Builds the Clipboard Share server (`clipboard-share`) and the desktop panel
(`clipboard-share-panel`) from this repository checkout. No network sources
are used: `prepare()` copies the sources from the parent directory into
`src/`, so the build leaves no artifacts in the repository tree.

## Build

```sh
cd pkgbuild && makepkg -f
```

If Go is only available through mise, run
`mise exec go@1.24 -- makepkg -f` instead.

## Install

```sh
sudo pacman -U clipboard-share-*.pkg.tar.zst
```

Installed files:

- `/usr/bin/clipboard-share`, `/usr/bin/clipboard-share-panel`
- `/usr/lib/systemd/user/clipboard-share.service`
- `/usr/lib/udev/rules.d/99-clipboard-share-uinput.rules`
- `/usr/share/applications/clipboard-share.desktop`
- `/etc/xdg/autostart/clipboard-share-panel.desktop` (panel starts hidden in the tray)
- `/usr/share/icons/hicolor/scalable/apps/clipboard-share.svg`

## Enable the server

```sh
systemctl --user enable --now clipboard-share
```

The service starts with the graphical session and restarts if it fails.
Set `CLIPBOARD_SHARE_PORT` (default `8765`) with
`systemctl --user edit clipboard-share` if needed.

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
