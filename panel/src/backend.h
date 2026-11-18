#pragma once

#include <QHash>
#include <QObject>
#include <QSet>
#include <QTimer>
#include <QVariantList>

class QJsonObject;

// The panel's client of the server's pairing control socket: a Unix socket that
// takes one JSON command per line and answers one JSON line. The server never
// pushes, so the panel polls `list`. Each exchange uses its own short-lived
// connection, like `nudge pair` does, so a dead server never leaves a
// half-open socket behind.
class Backend : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool online READ online NOTIFY changed)
    Q_PROPERTY(QString error READ error NOTIFY changed)
    Q_PROPERTY(QString message READ message NOTIFY changed)
    Q_PROPERTY(QVariantList requests READ requests NOTIFY changed)
    Q_PROPERTY(QVariantList devices READ devices NOTIFY changed)
    Q_PROPERTY(QString hostName READ hostName CONSTANT)
    Q_PROPERTY(QString socketPath READ socketPath NOTIFY changed)

public:
    // pollMs <= 0 disables the periodic refresh (tests drive refresh() by hand).
    // An empty socketPath means paths::controlSocket(), resolved again for
    // every exchange so the panel follows the server's config-dir migration.
    explicit Backend(const QString &socketPath = {}, int pollMs = 1000, QObject *parent = nullptr);

    bool online() const { return m_online; }
    QString error() const { return m_error; }
    QString message() const { return m_message; }
    QVariantList requests() const { return m_requests; }
    QVariantList devices() const { return m_devices; }
    QString hostName() const;
    QString socketPath() const;

    Q_INVOKABLE void refresh();
    Q_INVOKABLE void approve(const QString &requestId);
    Q_INVOKABLE void deny(const QString &requestId);
    Q_INVOKABLE void revoke(const QString &deviceId);
    Q_INVOKABLE void approveLast();

signals:
    void changed();
    // Emitted once per pending request the first time the panel sees it.
    void newRequest(const QString &name, const QString &ip);

private:
    void send(const QJsonObject &command, bool refreshAfter);
    void applyList(const QJsonObject &response);
    void setOffline(const QString &reason);
    QString errorOf(const QJsonObject &response) const;

    // What changed() announces; compared so an unchanged poll stays silent.
    struct State {
        bool online;
        QString error, message;
        QVariantList requests, devices;
        bool operator!=(const State &other) const {
            return online != other.online || error != other.error || message != other.message ||
                   requests != other.requests || devices != other.devices;
        }
    };
    State state() const { return {m_online, m_error, m_message, m_requests, m_devices}; }

    // Empty when the path is resolved at each use.
    QString m_fixedSocketPath;
    QTimer m_poll;
    bool m_listInFlight = false;
    // An action finished while a list was in flight: that list may predate the
    // action, so another one runs as soon as it completes.
    bool m_refreshPending = false;
    bool m_online = false;
    QString m_error;
    bool m_errorFromAction = false;
    QString m_message;
    QVariantList m_requests, m_devices;
    QSet<QString> m_seenRequests;
};
