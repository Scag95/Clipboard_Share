#pragma once

#include <QObject>
#include <QStringList>

#include <functional>

class Backend;

// The panel's two login switches, independent of each other:
// - server: the systemd user unit nudge.service, enabled or disabled with
//   `systemctl --user`;
// - panel: the packaged XDG autostart entry nudge-panel.desktop, turned off by
//   a user override with Hidden=true in $XDG_CONFIG_HOME/autostart and turned
//   on again by removing that override.
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
    // user's real units.
    using Runner = std::function<Result(const QStringList &args)>;

    static constexpr const char *Unit = "nudge.service";

    // nudgeConfigDir holds the first-run marker; xdgConfigHome holds
    // autostart/. Empty values mean the real ones; an empty runner runs the
    // real systemctl.
    explicit Autostart(const QString &nudgeConfigDir = {}, const QString &xdgConfigHome = {}, Runner runner = {},
                       QObject *parent = nullptr);

    bool serverEnabled() const { return m_serverEnabled; }
    bool panelEnabled() const;
    QString error() const { return m_error; }
    QString overridePath() const;

    Q_INVOKABLE bool setServerEnabled(bool enabled);
    Q_INVOKABLE bool setPanelEnabled(bool enabled);

    // The first launch turns the server's autostart on, once; afterwards the
    // user's choice stands, even when it is off.
    void applyFirstRunDefaults();

    // Asks systemd to start the server once, if the backend's first answer
    // says the server is not running. Independent of serverEnabled().
    void startServerWhenOffline(Backend *backend);

signals:
    void changed();

private:
    Result run(const QStringList &args) const;
    void setError(const QString &error);

    QString m_configDir, m_xdgConfigHome;
    Runner m_runner;
    bool m_serverEnabled = false;
    QString m_error;
};
