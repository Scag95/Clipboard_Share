#include "autostart.h"

#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QProcess>
#include <QSaveFile>
#include <QStandardPaths>
#include <QTimer>

#include <memory>

#include "backend.h"
#include "paths.h"

namespace {

// Records that the first-run defaults were applied, so they never are again.
constexpr const char *kMarker = "autostart-defaults-applied";

constexpr int kStartTimeoutMs = 5000;
constexpr int kFinishTimeoutMs = 15000;

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
    if (!process.waitForStarted(kStartTimeoutMs))
        return {-1, QStringLiteral("systemctl: %1").arg(process.errorString())};
    if (!process.waitForFinished(kFinishTimeoutMs)) {
        process.kill();
        process.waitForFinished(1000);
        return {-1, QStringLiteral("systemctl --user %1: no answer").arg(args.join(' '))};
    }
    const int code = process.exitStatus() == QProcess::NormalExit ? process.exitCode() : -1;
    return {code, QString::fromLocal8Bit(process.readAll()).trimmed()};
}

// Whether the [Desktop Entry] group of the override turns the entry off:
// Hidden=true or X-GNOME-Autostart-enabled=false. Other groups (actions) and
// comments do not count.
bool hiddenOverride(const QString &path) {
    QFile file(path);
    if (!file.open(QIODevice::ReadOnly))
        return false;
    bool inEntry = false;
    for (const QString &raw : QString::fromUtf8(file.readAll()).split('\n')) {
        const QString line = raw.trimmed();
        if (line.isEmpty() || line.startsWith('#'))
            continue;
        if (line.startsWith('[')) {
            inEntry = line == QLatin1String("[Desktop Entry]");
            continue;
        }
        if (!inEntry)
            continue;
        const int equals = line.indexOf('=');
        if (equals < 0)
            continue;
        const QString key = line.left(equals).trimmed();
        const QString value = line.mid(equals + 1).trimmed();
        if (key == QLatin1String("Hidden") && value.compare("true", Qt::CaseInsensitive) == 0)
            return true;
        if (key == QLatin1String("X-GNOME-Autostart-enabled") && value.compare("false", Qt::CaseInsensitive) == 0)
            return true;
    }
    return false;
}

QString reasonOf(const Autostart::Result &result) {
    return result.output.trimmed().isEmpty()
               ? QObject::tr("systemctl terminó con %1").arg(result.exitCode)
               : result.output.trimmed();
}

} // namespace

Autostart::Autostart(const QString &panelStateDir, const QString &xdgConfigHome, Runner runner, QObject *parent)
    : QObject(parent),
      m_stateDir(panelStateDir.isEmpty() ? paths::panelStateDir() : panelStateDir),
      m_xdgConfigHome(xdgConfigHome.isEmpty()
                          ? QStandardPaths::writableLocation(QStandardPaths::GenericConfigLocation)
                          : xdgConfigHome),
      m_runner(std::move(runner)) {
    const int generation = m_switchGeneration;
    runAsync({"is-enabled", Unit}, [this, generation](const Result &result) {
        if (generation != m_switchGeneration)
            return;
        m_serverEnabled = result.exitCode == 0;
        emit changed();
    });
}

QString Autostart::overridePath() const {
    return m_xdgConfigHome + "/autostart/nudge-panel.desktop";
}

QString Autostart::markerPath() const {
    return m_stateDir + "/" + kMarker;
}

bool Autostart::panelEnabled() const {
    return !hiddenOverride(overridePath());
}

Autostart::Result Autostart::run(const QStringList &args) const {
    return m_runner ? m_runner(args) : runSystemctl(args);
}

void Autostart::runAsync(const QStringList &args, Done done) {
    if (m_runner) {
        done(m_runner(args));
        return;
    }
    auto *process = new QProcess(this);
    process->setProcessChannelMode(QProcess::MergedChannels);
    auto *timeout = new QTimer(process);
    timeout->setSingleShot(true);
    timeout->setInterval(kStartTimeoutMs + kFinishTimeoutMs);
    auto reported = std::make_shared<bool>(false);
    auto finish = [process, timeout, done, reported](const Result &result) {
        // Reported once, whichever signal comes first.
        if (*reported)
            return;
        *reported = true;
        timeout->stop();
        QObject::disconnect(process, nullptr, nullptr, nullptr);
        process->deleteLater();
        done(result);
    };
    connect(process, &QProcess::finished, this, [process, finish](int code, QProcess::ExitStatus status) {
        finish({status == QProcess::NormalExit ? code : -1, QString::fromLocal8Bit(process->readAll()).trimmed()});
    });
    connect(process, &QProcess::errorOccurred, this, [process, finish](QProcess::ProcessError error) {
        if (error == QProcess::FailedToStart)
            finish({-1, QStringLiteral("systemctl: %1").arg(process->errorString())});
    });
    connect(timeout, &QTimer::timeout, this, [process, finish, args] {
        finish({-1, QStringLiteral("systemctl --user %1: no answer").arg(args.join(' '))});
        process->kill();
    });
    timeout->start();
    process->start("systemctl", QStringList{"--user"} + args);
}

void Autostart::setError(const QString &error) {
    m_error = error;
    emit changed();
}

bool Autostart::applySwitchResult(bool enabled, const Result &result) {
    ++m_switchGeneration;
    if (result.exitCode != 0) {
        setError((enabled ? tr("No se pudo activar el inicio del servidor: %1")
                          : tr("No se pudo desactivar el inicio del servidor: %1"))
                     .arg(reasonOf(result)));
        return false;
    }
    m_serverEnabled = enabled;
    setError({});
    return true;
}

bool Autostart::setServerEnabled(bool enabled) {
    return applySwitchResult(enabled, run({enabled ? "enable" : "disable", Unit}));
}

bool Autostart::setPanelEnabled(bool enabled) {
    const QString path = overridePath();
    if (enabled) {
        // Only an override that turns the panel off is removed; any other
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
    const QString marker = markerPath();
    if (QFileInfo::exists(marker))
        return;
    // The panel's autostart is on by default through the packaged entry; the
    // server's needs enabling. A failed enable writes no marker, so the next
    // launch tries again, and its error stays shown.
    const int generation = m_switchGeneration;
    runAsync({"enable", Unit}, [this, generation, marker](const Result &result) {
        if (generation != m_switchGeneration) {
            // The user flipped the switch meanwhile: their choice stands, and
            // the defaults count as applied once systemd took the enable.
            if (result.exitCode != 0)
                return;
        } else if (!applySwitchResult(true, result)) {
            return;
        }
        QFile file(marker);
        if (!QDir().mkpath(m_stateDir) || !file.open(QIODevice::WriteOnly))
            setError(tr("No se pudo escribir %1").arg(marker));
    });
}

void Autostart::startServerWhenOffline(Backend *backend) {
    auto startIfOffline = [this, backend] {
        if (backend->online())
            return;
        runAsync({"start", Unit}, [this](const Result &result) {
            if (result.exitCode != 0)
                setError(tr("No se pudo iniciar el servidor: %1").arg(reasonOf(result)));
        });
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
