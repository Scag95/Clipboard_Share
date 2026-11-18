#include "paths.h"

#include <QDir>
#include <QFileInfo>

namespace paths {

namespace {

// Anything at the path counts, a dangling symlink too, like the server's
// Lstat: the server never moves the old directory over it either.
bool present(const QString &path) {
    const QFileInfo info(path);
    return info.exists() || info.isSymLink();
}

} // namespace

QString configDir() {
    const QString base = QDir::homePath() + "/.config";
    const QString current = base + "/nudge";
    const QString old = base + "/clipboard-share";
    if (!present(current) && present(old))
        return old;
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

QString panelStateDir() {
    // The XDG spec ignores a relative value.
    QString base = qEnvironmentVariable("XDG_STATE_HOME");
    if (base.isEmpty() || QDir::isRelativePath(base))
        base = QDir::homePath() + "/.local/state";
    return base + "/nudge-panel";
}

} // namespace paths
