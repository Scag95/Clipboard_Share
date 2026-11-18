#include "paths.h"

#include <QDir>
#include <QFileInfo>

#include <cstdio>

namespace paths {

QString configDir() {
    const QString base = QDir::homePath() + "/.config";
    const QString current = base + "/nudge";
    const QString old = base + "/clipboard-share";
    // exists() follows symlinks, so a dangling link named nudge also blocks
    // the move: nothing at the new name is ever replaced.
    if (!QFileInfo::exists(current) && !QFileInfo(current).isSymLink() && QFileInfo(old).isDir()) {
        if (!QDir().rename(old, current))
            std::fprintf(stderr, "nudge-panel: cannot move %s to %s\n", qPrintable(old), qPrintable(current));
    }
    return current;
}

QString controlSocket() {
    for (const char *name : {"NUDGE_CTL_SOCK", "CLIPBOARD_SHARE_CTL_SOCK"}) {
        const QString override = qEnvironmentVariable(name);
        if (!override.isEmpty())
            return override;
    }
    return configDir() + "/ctl.sock";
}

} // namespace paths
