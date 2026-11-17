#include "backend.h"

#include <QDir>
#include <QHostInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocalSocket>

namespace {

// A server that accepts the connection but never answers must not freeze the
// panel's polling.
constexpr int kExchangeTimeoutMs = 3000;

QString defaultSocketPath() {
    const QString override = qEnvironmentVariable("CLIPBOARD_SHARE_CTL_SOCK");
    return override.isEmpty() ? QDir::homePath() + "/.config/clipboard-share/ctl.sock" : override;
}

} // namespace

Backend::Backend(const QString &socketPath, int pollMs, QObject *parent)
    : QObject(parent), m_socketPath(socketPath.isEmpty() ? defaultSocketPath() : socketPath) {
    if (pollMs > 0) {
        m_poll.setInterval(pollMs);
        connect(&m_poll, &QTimer::timeout, this, &Backend::refresh);
        m_poll.start();
        refresh();
    }
}

QString Backend::hostName() const {
    return QHostInfo::localHostName();
}

void Backend::refresh() {
    // Skip while the previous poll is still waiting, so a slow server never
    // piles up connections.
    if (m_listInFlight)
        return;
    m_listInFlight = true;
    send({{"cmd", "list"}}, false);
}

void Backend::approve(const QString &requestId) {
    send({{"cmd", "approve"}, {"request", requestId}}, true);
}

void Backend::deny(const QString &requestId) {
    send({{"cmd", "deny"}, {"request", requestId}}, true);
}

void Backend::revoke(const QString &deviceId) {
    send({{"cmd", "revoke"}, {"device", deviceId}}, true);
}

void Backend::approveLast() {
    send({{"cmd", "approve-last"}}, true);
}

void Backend::send(const QJsonObject &command, bool refreshAfter) {
    const bool isList = command.value("cmd").toString() == "list";
    auto *socket = new QLocalSocket(this);
    auto *timeout = new QTimer(socket);
    timeout->setSingleShot(true);
    timeout->setInterval(kExchangeTimeoutMs);

    auto finish = [this, socket, isList] {
        if (isList)
            m_listInFlight = false;
        socket->disconnect(this);
        socket->deleteLater();
    };

    connect(timeout, &QTimer::timeout, this, [this, socket, finish] {
        socket->abort();
        setOffline(tr("The server at %1 did not answer").arg(m_socketPath));
        finish();
    });
    connect(socket, &QLocalSocket::connected, this, [socket, command, timeout] {
        timeout->start();
        socket->write(QJsonDocument(command).toJson(QJsonDocument::Compact) + "\n");
    });
    connect(socket, &QLocalSocket::errorOccurred, this, [this, socket, finish](QLocalSocket::LocalSocketError) {
        // The control socket belongs to the server process, so an unreachable
        // socket means the server is not running (or runs under another user).
        setOffline(tr("Server not running (%1)").arg(m_socketPath));
        finish();
    });
    connect(socket, &QLocalSocket::readyRead, this, [this, socket, isList, refreshAfter, finish] {
        if (!socket->canReadLine())
            return;
        const auto response = QJsonDocument::fromJson(socket->readLine()).object();
        const bool ok = response.value("ok").toBool();
        m_online = true;
        if (!isList) {
            // The outcome of the user's last action stays visible until the next
            // action; the refresh that follows must not wipe it a moment later.
            m_message = ok ? response.value("message").toString() : QString();
            m_error = ok ? QString() : response.value("error").toString(tr("The server rejected the command"));
            m_errorFromAction = !ok;
        } else if (!m_errorFromAction) {
            m_error = ok ? QString() : response.value("error").toString(tr("The server rejected the command"));
        }
        if (isList && ok)
            applyList(response);
        emit changed();
        finish();
        if (refreshAfter)
            refresh();
    });
    socket->connectToServer(m_socketPath);
}

void Backend::applyList(const QJsonObject &response) {
    m_requests.clear();
    m_devices.clear();
    QSet<QString> present;
    QList<QPair<QString, QString>> fresh;
    for (const auto &value : response.value("requests").toArray()) {
        const auto object = value.toObject();
        m_requests.append(object.toVariantMap());
        const QString id = object.value("requestId").toString();
        present.insert(id);
        if (!m_seenRequests.contains(id))
            fresh.append({object.value("name").toString(), object.value("ip").toString()});
    }
    for (const auto &value : response.value("devices").toArray())
        m_devices.append(value.toObject().toVariantMap());
    // Forget resolved requests so the set cannot grow without bound.
    m_seenRequests = present;
    // Announced after the state is consistent, so a listener can read it.
    for (const auto &entry : fresh)
        emit newRequest(entry.first, entry.second);
}

void Backend::setOffline(const QString &reason) {
    m_online = false;
    m_error = reason;
    m_errorFromAction = false;
    m_requests.clear();
    m_devices.clear();
    m_seenRequests.clear();
    emit changed();
}
