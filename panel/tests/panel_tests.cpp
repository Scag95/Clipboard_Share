// Tests for Backend, the panel's client of the server's pairing control socket.
// A fake server on a temporary socket plays the Go server: one JSON command per
// line in, one JSON response per line out. Everything is asserted through the
// public properties, signals and invokables, never through internals.

#include <QJsonDocument>
#include <QJsonObject>
#include <QLocalServer>
#include <QLocalSocket>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QtTest>

#include "backend.h"
#include "fakeserver.h"

using namespace fake;

namespace {

// Waits until the backend has finished a round trip: its changed() signal fires.
bool waitChanged(QSignalSpy &spy, int count = 1) {
    return spy.count() >= count || spy.wait(2000) || spy.count() >= count;
}

} // namespace

class PanelTests : public QObject {
    Q_OBJECT

private slots:
    void init() { QVERIFY(m_dir.isValid()); m_path = m_dir.filePath("ctl.sock"); }

    void listPopulatesRequestsAndDevices() {
        FakeServer server(m_path);
        server.requests = {request("r1", "Pixel", "192.168.0.13")};
        server.devices = {device("d1", "Galaxy", "192.168.0.20")};
        Backend backend(m_path, 0);
        QSignalSpy changed(&backend, &Backend::changed);

        backend.refresh();
        QVERIFY(waitChanged(changed));

        QVERIFY(backend.online());
        QCOMPARE(backend.error(), QString());
        QCOMPARE(backend.requests().size(), 1);
        QCOMPARE(backend.requests().first().toMap().value("name").toString(), QString("Pixel"));
        QCOMPARE(backend.requests().first().toMap().value("requestId").toString(), QString("r1"));
        QCOMPARE(backend.devices().size(), 1);
        QCOMPARE(backend.devices().first().toMap().value("name").toString(), QString("Galaxy"));
        QCOMPARE(server.commands.size(), 1);
        QCOMPARE(server.commands.first().value("cmd").toString(), QString("list"));
    }

    void missingSocketReportsOfflineWithTheSocketPath() {
        Backend backend(m_dir.filePath("nope.sock"), 0);
        QSignalSpy changed(&backend, &Backend::changed);

        backend.refresh();
        QVERIFY(waitChanged(changed));

        QVERIFY(!backend.online());
        QVERIFY2(backend.error().contains("nope.sock"), qPrintable(backend.error()));
        QVERIFY(backend.requests().isEmpty());
        QVERIFY(backend.devices().isEmpty());
    }

    void approveSendsTheRequestIdThenRefreshesAndShowsTheMessage() {
        FakeServer server(m_path);
        server.requests = {request("r1", "Pixel", "192.168.0.13")};
        Backend backend(m_path, 0);
        QSignalSpy changed(&backend, &Backend::changed);

        backend.approve("r1");
        QTRY_VERIFY_WITH_TIMEOUT(server.commands.size() >= 2, 2000);

        QCOMPARE(server.commands.at(0).value("cmd").toString(), QString("approve"));
        QCOMPARE(server.commands.at(0).value("request").toString(), QString("r1"));
        QCOMPARE(server.commands.at(1).value("cmd").toString(), QString("list"));
        QTRY_COMPARE_WITH_TIMEOUT(backend.message(), QString("Approved pair request."), 2000);
    }

    void denyAndRevokeAndApproveLastUseTheirCommands() {
        FakeServer server(m_path);
        Backend backend(m_path, 0);

        backend.deny("r9");
        QTRY_VERIFY_WITH_TIMEOUT(server.commands.size() >= 2, 2000);
        backend.revoke("d7");
        QTRY_VERIFY_WITH_TIMEOUT(server.commands.size() >= 4, 2000);
        backend.approveLast();
        QTRY_VERIFY_WITH_TIMEOUT(server.commands.size() >= 6, 2000);

        QCOMPARE(server.commands.at(0).value("cmd").toString(), QString("deny"));
        QCOMPARE(server.commands.at(0).value("request").toString(), QString("r9"));
        QCOMPARE(server.commands.at(2).value("cmd").toString(), QString("revoke"));
        QCOMPARE(server.commands.at(2).value("device").toString(), QString("d7"));
        QCOMPARE(server.commands.at(4).value("cmd").toString(), QString("approve-last"));
    }

    void serverErrorIsShownAndKeepsTheListAndTheServerOnline() {
        FakeServer server(m_path);
        server.requests = {request("r1", "Pixel", "192.168.0.13")};
        Backend backend(m_path, 0);
        QSignalSpy changed(&backend, &Backend::changed);
        backend.refresh();
        QVERIFY(waitChanged(changed));

        server.actionError = "pair: request expired";
        backend.approve("r1");
        QTRY_COMPARE_WITH_TIMEOUT(backend.error(), QString("pair: request expired"), 2000);

        QVERIFY(backend.online());
        QCOMPARE(backend.requests().size(), 1);
    }

    void newRequestIsAnnouncedOnceNotOnEveryPoll() {
        FakeServer server(m_path);
        Backend backend(m_path, 0);
        QSignalSpy announced(&backend, &Backend::newRequest);
        QSignalSpy changed(&backend, &Backend::changed);
        backend.refresh();
        QVERIFY(waitChanged(changed));
        QCOMPARE(announced.count(), 0);

        server.requests = {request("r1", "Pixel", "192.168.0.13")};
        backend.refresh();
        QVERIFY(announced.wait(2000));
        QCOMPARE(announced.count(), 1);
        QCOMPARE(announced.first().at(0).toString(), QString("Pixel"));
        QCOMPARE(announced.first().at(1).toString(), QString("192.168.0.13"));

        const int commandsBefore = server.commands.size();
        backend.refresh();
        QTRY_VERIFY_WITH_TIMEOUT(server.commands.size() > commandsBefore, 2000);
        QTest::qWait(100);
        QCOMPARE(announced.count(), 1);
    }

    void socketPathHonorsTheEnvironmentOverride() {
        qputenv("CLIPBOARD_SHARE_CTL_SOCK", "/tmp/custom-ctl.sock");
        Backend backend;
        qunsetenv("CLIPBOARD_SHARE_CTL_SOCK");
        QCOMPARE(backend.socketPath(), QString("/tmp/custom-ctl.sock"));
    }

private:
    QTemporaryDir m_dir;
    QString m_path;
};

QTEST_MAIN(PanelTests)
#include "panel_tests.moc"
