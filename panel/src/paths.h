#pragma once

#include <QString>

// Where the panel finds what it shares with the Nudge server.
namespace paths {

// The server's directory (state.json, ctl.sock): ~/.config/nudge, the same
// as the server and the CLI. The panel never creates it and resolves it again
// at each use.
QString configDir();

// NUDGE_CTL_SOCK, else ctl.sock in configDir(), resolved now.
QString controlSocket();

// The panel's own state: $XDG_STATE_HOME/nudge-panel, by default
// ~/.local/state/nudge-panel. Not created here.
QString panelStateDir();

} // namespace paths
