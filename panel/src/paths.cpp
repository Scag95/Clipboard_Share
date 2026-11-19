#include "paths.h"

#include <QDir>

namespace paths {

QString configDir() {
    return QDir::homePath() + "/.config/nudge";
}

QString controlSocket() {
    const QString override = qEnvironmentVariable("NUDGE_CTL_SOCK");
    if (!override.isEmpty())
        return override;
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
