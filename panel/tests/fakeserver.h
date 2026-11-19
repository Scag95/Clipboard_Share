#pragma once

// Fake of the Go server's pairing control socket, shared by the panel's tests:
// one JSON command per line in, one JSON response per line out.

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocalServer>
#include <QLocalSocket>
#include <QTimer>
#include <QtTest>

namespace fake {

// Answers every command with the next scripted response and records commands.
class FakeServer : public QObject {
public:
    explicit FakeServer(const QString &path) {
        QLocalServer::removeServer(path);
        QVERIFY2(m_server.listen(path), "fake server could not listen");
        connect(&m_server, &QLocalServer::newConnection, this, [this] {
            auto *socket = m_server.nextPendingConnection();
            connect(socket, &QLocalSocket::readyRead, this, [this, socket] {
                while (socket->canReadLine()) {
                    const auto command = QJsonDocument::fromJson(socket->readLine()).object();
                    commands.append(command);
                    // A hung server: it accepted the connection and read the
                    // command, but never answers.
                    if (silent)
                        continue;
                    // The response is taken now, so a delayed list carries the
                    // state as it was when the command arrived (a stale list).
                    const QByteArray line = QJsonDocument(nextResponse(command)).toJson(QJsonDocument::Compact) + "\n";
                    const int delay = command.value("cmd").toString() == "list" ? listDelayMs : 0;
                    auto answer = [socket, line] {
                        socket->write(line);
                        socket->flush();
                    };
                    if (delay > 0)
                        QTimer::singleShot(delay, socket, answer);
                    else
                        answer();
                }
            });
            connect(socket, &QLocalSocket::disconnected, socket, &QObject::deleteLater);
        });
    }

    QJsonObject nextResponse(const QJsonObject &command) {
        const auto cmd = command.value("cmd").toString();
        if (cmd == "list") {
            QJsonObject list{{"ok", true}, {"requests", requests}, {"devices", devices}};
            // Like the Go server: the certificate's code and fingerprint, only on list.
            if (!code.isEmpty()) {
                list.insert("code", code);
                list.insert("fingerprint", fingerprint);
            }
            return list;
        }
        if (!actionError.isEmpty())
            return {{"ok", false}, {"error", actionError}, {"requests", QJsonValue::Null}, {"devices", QJsonValue::Null}};
        return {{"ok", true}, {"message", actionMessage}, {"requests", QJsonValue::Null}, {"devices", QJsonValue::Null}};
    }

    QJsonArray requests, devices;
    QString code, fingerprint;
    QString actionMessage = "Approved pair request.";
    QString actionError;
    bool silent = false;
    int listDelayMs = 0;
    QList<QJsonObject> commands;

private:
    QLocalServer m_server;
};

QJsonObject request(const QString &requestId, const QString &name, const QString &ip) {
    return {{"requestId", requestId}, {"id", "dev-" + requestId}, {"name", name}, {"ip", ip},
            {"requestedAt", "2026-05-17T18:00:00Z"}};
}

QJsonObject device(const QString &id, const QString &name, const QString &ip) {
    return {{"id", id}, {"name", name}, {"ip", ip}, {"firstApproved", "2026-05-17T18:00:00Z"}};
}

} // namespace fake
