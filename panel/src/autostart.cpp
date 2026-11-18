#include "autostart.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QProcess>
#include <QRegularExpression>
#include <QSaveFile>
#include <QStandardPaths>

#include <memory>

#include "backend.h"
#include "paths.h"

namespace {

// Records that the first-run defaults were applied, so they never are again.
constexpr const char *kMarker = "autostart-defaults-applied";

// Same Name and Exec as the packaged /etc/xdg/autostart/nudge-panel.desktop;
// Hidden=true tells the session not to start it.
constexpr const char *kHiddenOverride =
    "[Desktop Entry]\n"
    "Type=Application\n"
    "Name=Nudge\n"
    "Exec=nudge-panel --background\n"
    "Hidden=true\n";

Autostart::Result runSystemctl(const QStringList &args) {
    QProcess process;
    process.setProcessChannelMode(QProcess::MergedChannels);
    process.start("systemctl", QStringList{"--user"} + args);
    if (!process.waitForStarted(5000))
        return {-1, QStringLiteral("systemctl: %1").arg(process.errorString())};
    if (!process.waitForFinished(15000)) {
        process.kill();
        process.waitForFinished(1000);
        return {-1, QStringLiteral("systemctl --user %1: no answer").arg(args.join(' '))};
    }
    const int code = process.exitStatus() == QProcess::NormalExit ? process.exitCode() : -1;
    return {code, QString::fromLocal8Bit(process.readAll()).trimmed()};
}

bool hiddenOverride(const QString &path) {
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return false;
    static const QRegularExpression hidden(R"re(^\s*Hidden\s*=\s*true\s*$)re",
                                           QRegularExpression::MultilineOption |
                                               QRegularExpression::CaseInsensitiveOption);
    return hidden.match(QString::fromUtf8(file.readAll())).hasMatch();
}

} // namespace

Autostart::Autostart(const QString &nudgeConfigDir, const QString &xdgConfigHome, Runner runner, QObject *parent)
    : QObject(parent),
      m_configDir(nudgeConfigDir.isEmpty() ? paths::configDir() : nudgeConfigDir),
      m_xdgConfigHome(xdgConfigHome.isEmpty()
                          ? QStandardPaths::writableLocation(QStandardPaths::GenericConfigLocation)
                          : xdgConfigHome),
      m_runner(runner ? std::move(runner) : Runner(runSystemctl)) {
    m_serverEnabled = run({"is-enabled", Unit}).exitCode == 0;
}

QString Autostart::overridePath() const {
    return m_xdgConfigHome + "/autostart/nudge-panel.desktop";
}

bool Autostart::panelEnabled() const {
    return !hiddenOverride(overridePath());
}

Autostart::Result Autostart::run(const QStringList &args) const {
    return m_runner(args);
}

void Autostart::setError(const QString &error) {
    m_error = error;
    emit changed();
}

bool Autostart::setServerEnabled(bool enabled) {
    const Result result = run({enabled ? "enable" : "disable", Unit});
    if (result.exitCode != 0) {
        setError((enabled ? tr("No se pudo activar el inicio del servidor: %1")
                          : tr("No se pudo desactivar el inicio del servidor: %1"))
                     .arg(result.output.isEmpty() ? tr("systemctl terminó con %1").arg(result.exitCode)
                                                  : result.output.trimmed()));
        return false;
    }
    m_serverEnabled = enabled;
    setError({});
    return true;
}

bool Autostart::setPanelEnabled(bool enabled) {
    const QString path = overridePath();
    if (enabled) {
        // Only our own kind of override is removed; one without Hidden=true
        // already lets the panel start and belongs to the user.
        if (hiddenOverride(path) && !QFile::remove(path)) {
            setError(tr("No se pudo borrar %1").arg(path));
            return false;
        }
        setError({});
        return true;
    }
    const QString directory = QFileInfo(path).absolutePath();
    QSaveFile file(path);
    if (!QDir().mkpath(directory) || !file.open(QIODevice::WriteOnly) || file.write(kHiddenOverride) < 0 ||
        !file.commit()) {
        setError(tr("No se pudo escribir %1").arg(path));
        return false;
    }
    setError({});
    return true;
}

void Autostart::applyFirstRunDefaults() {
    const QString marker = m_configDir + "/" + kMarker;
    if (QFileInfo::exists(marker))
        return;
    // The panel's autostart is on by default through the packaged entry; the
    // server's needs enabling. Tried once: a failure is shown, and the user's
    // later choice is never overridden.
    setServerEnabled(true);
    QFile file(marker);
    if (!QDir().mkpath(m_configDir) || !file.open(QIODevice::WriteOnly))
        setError(tr("No se pudo escribir %1").arg(marker));
}

void Autostart::startServerWhenOffline(Backend *backend) {
    auto startIfOffline = [this, backend] {
        if (backend->online())
            return;
        const Result result = run({"start", Unit});
        if (result.exitCode != 0)
            setError(tr("No se pudo iniciar el servidor: %1")
                         .arg(result.output.isEmpty() ? tr("systemctl terminó con %1").arg(result.exitCode)
                                                      : result.output.trimmed()));
    };
    // A missing socket is reported at once, possibly while the backend was
    // still being constructed; otherwise the first answer is still to come.
    if (backend->online() || !backend->error().isEmpty()) {
        startIfOffline();
        return;
    }
    auto connection = std::make_shared<QMetaObject::Connection>();
    *connection = connect(backend, &Backend::changed, this, [startIfOffline, connection] {
        QObject::disconnect(*connection);
        startIfOffline();
    });
}
