#pragma once

#include <QString>

// Where the panel finds what it shares with the Nudge server.
namespace paths {

// ~/.config/nudge, the server's directory (state.json, ctl.sock). When only
// the pre-rename ~/.config/clipboard-share exists it is moved here once, so
// paired devices survive the rename; an existing ~/.config/nudge is never
// overwritten.
QString configDir();

// NUDGE_CTL_SOCK, else the old CLIPBOARD_SHARE_CTL_SOCK, else ctl.sock in
// configDir().
QString controlSocket();

} // namespace paths
