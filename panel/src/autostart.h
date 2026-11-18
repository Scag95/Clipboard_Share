#pragma once

#include <QObject>
#include <QStringList>

#include <functional>

class Backend;

// The panel's two login switches, independent of each other:
// - server: the systemd user unit nudge.service, enabled or disabled with
//   `systemctl --user`;
// - panel: the packaged XDG autostart entry nudge-panel.desktop, turned off by
//   a user override in $XDG_CONFIG_HOME/autostart whose [Desktop Entry] says
//   Hidden=true (or X-GNOME-Autostart-enabled=false), and turned on again by
//   removing that override.
class Autostart : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool serverEnabled READ serverEnabled NOTIFY changed)
    Q_PROPERTY(bool panelEnabled READ panelEnabled NOTIFY changed)
    Q_PROPERTY(QString error READ error NOTIFY changed)

public:
    struct Result {
        int exitCode = -1;
        QString output;
    };
    // Runs `systemctl --user <args>`; injectable so tests never touch the
    // user's real units. An injected runner is called synchronously, also for
    // the startup calls.
    using Runner = std::function<Result(const QStringList &args)>;

    static constexpr const char *Unit = "nudge.service";

    // panelStateDir holds the first-run marker (default
    // paths::panelStateDir()); xdgConfigHome holds autostart/. Empty values
    // mean the real ones; an empty runner runs the real systemctl, and then
    // the startup calls (is-enabled, the first-run enable, start) run
    // asynchronously so they never block the window.
    explicit Autostart(const QString &panelStateDir = {}, const QString &xdgConfigHome = {}, Runner runner = {},
                       QObject *parent = nullptr);

    bool serverEnabled() const { return m_serverEnabled; }
    bool panelEnabled() const;
    QString error() const { return m_error; }
    QString overridePath() const;
    QString markerPath() const;

    Q_INVOKABLE bool setServerEnabled(bool enabled);
    Q_INVOKABLE bool setPanelEnabled(bool enabled);

    // Until it succeeds once, each launch turns the server's autostart on;
    // the marker is written only after the enable succeeded, so afterwards
    // the user's choice stands, even when it is off. A failure stays shown.
    void applyFirstRunDefaults();

    // Asks systemd to start the server once, if the backend's first answer
    // says the server is not running. Independent of serverEnabled().
    void startServerWhenOffline(Backend *backend);

signals:
    void changed();

private:
    using Done = std::function<void(const Result &)>;
    Result run(const QStringList &args) const;
    void runAsync(const QStringList &args, Done done);
    void setError(const QString &error);
    // Applies an enable/disable outcome; returns whether it succeeded.
    bool applySwitchResult(bool enabled, const Result &result);

    QString m_stateDir, m_xdgConfigHome;
    Runner m_runner;
    bool m_serverEnabled = false;
    // Bumped by every enable/disable outcome, so a late is-enabled answer
    // never overrides a newer one.
    int m_switchGeneration = 0;
    QString m_error;
};
