#pragma once

#include <QString>

// Where the panel finds what it shares with the Nudge server.
namespace paths {

// The server's effective directory (state.json, ctl.sock), with the same rule
// as the server and the CLI: ~/.config/nudge if it exists, else the
// pre-rename ~/.config/clipboard-share if it exists, else ~/.config/nudge.
// Only the server moves the old directory; the panel never moves or creates
// either one, and resolves it again at each use so it follows the move.
QString configDir();

// NUDGE_CTL_SOCK, else the old CLIPBOARD_SHARE_CTL_SOCK, else ctl.sock in
// configDir(), resolved now.
QString controlSocket();

// The panel's own state: $XDG_STATE_HOME/nudge-panel, by default
// ~/.local/state/nudge-panel. Not created here.
QString panelStateDir();

} // namespace paths
