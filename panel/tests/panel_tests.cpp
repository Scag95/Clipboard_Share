// Tests for Backend, the panel's client of the server's pairing control socket.
// A fake server on a temporary socket plays the Go server: one JSON command per
// line in, one JSON response per line out. Everything is asserted through the
// public properties, signals and invokables, never through internals.

#include <QDir>
#include <QElapsedTimer>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocalServer>
#include <QLocalSocket>
#include <QLockFile>
#include <QProcess>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QtTest>

#include <memory>

#include "backend.h"
#include "fakeserver.h"

using namespace fake;

namespace {

// Waits until the backend's changed() signal has fired `count` times in total.
bool waitChanged(QSignalSpy &spy, int count = 1) {
    while (spy.count() < count)
        if (!spy.wait(2000))
            return false;
    return true;
}

int commandCount(const FakeServer &server, const QString &cmd) {
    int n = 0;
    for (const auto &command : server.commands)
        n += command.value("cmd").toString() == cmd;
    return n;
}

// Lets the backend read and apply an answer the fake server already sent.
void settle() {
    QTest::qWait(200);
}

} // namespace

class PanelTests : public QObject {
    Q_OBJECT

private slots:
    void init() {
        QVERIFY(m_dir.isValid());
        m_path = m_dir.filePath("ctl.sock");
        m_sandbox = std::make_unique<QTemporaryDir>();
        QVERIFY(m_sandbox->isValid());
    }

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

        // The action is followed by a list that succeeds; the action's error must
        // survive it.
        QTRY_COMPARE_WITH_TIMEOUT(commandCount(server, "list"), 2, 2000);
        settle();
        QCOMPARE(backend.error(), QString("pair: request expired"));
        QVERIFY(backend.online());
        QCOMPARE(backend.requests().size(), 1);
    }

    // The Repeaters rebuild their delegates on every changed(): a poll that
    // brings the same state must not fire it, or a click can land on a
    // delegate that is being destroyed.
    void changedFiresOnlyWhenTheStateChanges() {
        FakeServer server(m_path);
        server.requests = {request("r1", "Pixel", "192.168.0.13")};
        server.devices = {device("d1", "Galaxy", "192.168.0.20")};
        Backend backend(m_path, 0);
        QSignalSpy changed(&backend, &Backend::changed);
        backend.refresh();
        QVERIFY(waitChanged(changed));
        QCOMPARE(changed.count(), 1);

        backend.refresh();
        QTRY_COMPARE_WITH_TIMEOUT(commandCount(server, "list"), 2, 2000);
        settle();
        QCOMPARE(changed.count(), 1);

        server.devices = {};
        backend.refresh();
        QVERIFY(waitChanged(changed, 2));
        QVERIFY(backend.devices().isEmpty());
        QCOMPARE(backend.requests().size(), 1);

        // An action always reports, even when its message repeats the last one.
        backend.approve("r1");
        QVERIFY(waitChanged(changed, 3));
        QCOMPARE(backend.message(), QString("Approved pair request."));
        QTRY_COMPARE_WITH_TIMEOUT(commandCount(server, "list"), 4, 2000);
        settle();
        const int afterFirstApprove = changed.count();
        backend.approve("r1");
        QVERIFY(waitChanged(changed, afterFirstApprove + 1));
    }

    void repeatedOutagesFireChangedOnce() {
        Backend backend(m_dir.filePath("nope.sock"), 0);
        QSignalSpy changed(&backend, &Backend::changed);
        backend.refresh();
        QVERIFY(waitChanged(changed));
        QVERIFY(!backend.online());

        backend.refresh();
        backend.refresh();
        QTest::qWait(300);
        QCOMPARE(changed.count(), 1);
    }

    // A transient outage must not announce every pending request again.
    void anOutageDoesNotAnnounceSeenRequestsAgain() {
        auto server = std::make_unique<FakeServer>(m_path);
        server->requests = {request("r1", "Pixel", "192.168.0.13")};
        Backend backend(m_path, 0);
        QSignalSpy announced(&backend, &Backend::newRequest);
        backend.refresh();
        QVERIFY(announced.wait(2000));
        QCOMPARE(announced.count(), 1);

        server.reset();
        backend.refresh();
        QTRY_VERIFY_WITH_TIMEOUT(!backend.online(), 2000);

        server = std::make_unique<FakeServer>(m_path);
        server->requests = {request("r1", "Pixel", "192.168.0.13"), request("r2", "Galaxy", "192.168.0.20")};
        backend.refresh();
        QTRY_VERIFY_WITH_TIMEOUT(backend.online(), 2000);
        settle();
        QCOMPARE(backend.requests().size(), 2);
        QCOMPARE(announced.count(), 2);
        QCOMPARE(announced.last().at(0).toString(), QString("Galaxy"));
    }

    // An action that finishes while a poll is in flight must not be undone by
    // that poll's older list: a fresh list follows it.
    void anActionDuringAPollIsFollowedByAFreshList() {
        FakeServer server(m_path);
        server.requests = {request("r1", "Pixel", "192.168.0.13")};
        server.listDelayMs = 400;
        Backend backend(m_path, 0);

        backend.refresh();
        QTRY_COMPARE_WITH_TIMEOUT(commandCount(server, "list"), 1, 2000);
        // The poll's answer still lists r1; the approval removes it.
        server.requests = {};
        server.listDelayMs = 0;
        backend.approve("r1");

        QTRY_COMPARE_WITH_TIMEOUT(commandCount(server, "list"), 2, 2000);
        QTRY_VERIFY_WITH_TIMEOUT(backend.requests().isEmpty(), 2000);
        settle();
        QVERIFY(backend.requests().isEmpty());
        QCOMPARE(backend.message(), QString("Approved pair request."));
        QCOMPARE(server.commands.at(1).value("cmd").toString(), QString("approve"));
    }

    // A server that accepts and never answers is offline after the exchange
    // timeout, and polling picks it up again once it answers.
    void aSilentServerGoesOfflineAndPollingRecovers() {
        FakeServer server(m_path);
        server.devices = {device("d1", "Galaxy", "192.168.0.20")};
        Backend backend(m_path, 200);
        QTRY_VERIFY_WITH_TIMEOUT(backend.online(), 2000);

        server.silent = true;
        QElapsedTimer clock;
        clock.start();
        QTRY_VERIFY_WITH_TIMEOUT(!backend.online(), 4500);
        QVERIFY2(clock.elapsed() < 4500, qPrintable(QString::number(clock.elapsed())));
        QVERIFY2(backend.error().contains("did not answer"), qPrintable(backend.error()));
        QVERIFY(backend.devices().isEmpty());

        server.silent = false;
        QTRY_VERIFY_WITH_TIMEOUT(backend.online(), 5000);
        QCOMPARE(backend.error(), QString());
        QCOMPARE(backend.devices().size(), 1);
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

    // Single instance, driven through the built panel binary, offscreen and
    // isolated: its own runtime, temp and home directories, no session bus, and
    // a control socket nobody serves.

    void theActivationSocketLivesInTheRuntimeDirectory() {
        QProcess *first = startPanel();
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(activationPath()), 10000);
        QVERIFY(first->state() == QProcess::Running);
        QVERIFY2(!QFileInfo::exists(sandbox("tmp/clipboard-share-panel")),
                 "activation socket left in the shared temp directory");

        QLocalSocket socket;
        socket.connectToServer(activationPath());
        QVERIFY(socket.waitForConnected(2000));
    }

    void aSecondLaunchShowsTheFirstAndExits() {
        QProcess *first = startPanel();
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(activationPath()), 10000);

        QProcess *second = startPanel();
        QVERIFY(second->waitForFinished(10000));
        QCOMPARE(second->exitCode(), 0);
        QVERIFY(first->state() == QProcess::Running);
        QVERIFY(QFileInfo::exists(lockPath()));
    }

    // A second launch while the first is still loading must wait for it, not
    // start a second panel.
    void aSecondLaunchDuringStartupDoesNotStartAnotherPanel() {
        QProcess *first = startPanel();
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(lockPath()), 10000);

        QProcess *second = startPanel();
        QVERIFY2(second->waitForFinished(10000), "a second panel kept running");
        QCOMPARE(second->exitCode(), 0);
        QVERIFY(first->state() == QProcess::Running);
    }

    // A lock whose holder is alive but does not answer is never removed.
    void aLiveLockHolderIsNeverOverridden() {
        QVERIFY(QDir().mkpath(sandbox("runtime")));
        QLockFile held(lockPath());
        QVERIFY(held.tryLock(0));

        QElapsedTimer clock;
        clock.start();
        QProcess *second = startPanel();
        QVERIFY2(second->waitForFinished(10000), "started a second panel over a live lock");
        QVERIFY2(clock.elapsed() >= 1500, "gave up activating too early");
        QCOMPARE(second->exitCode(), 1);
        QVERIFY2(second->readAllStandardError().contains(QByteArray::number(QCoreApplication::applicationPid())),
                 "the error does not name the running panel");

        qint64 pid = 0;
        QString host, app;
        QVERIFY(held.getLockInfo(&pid, &host, &app));
        QCOMPARE(pid, QCoreApplication::applicationPid());
    }

    // A panel that died without cleanup leaves its lock and socket behind; the
    // next launch takes over.
    void aDeadPanelsLockIsTakenOver() {
        QProcess *dead = startPanel();
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(activationPath()), 10000);
        dead->kill();
        QVERIFY(dead->waitForFinished(5000));
        QVERIFY(QFileInfo::exists(lockPath()));

        QProcess *next = startPanel();
        QTest::qWait(3000);
        QVERIFY2(next->state() == QProcess::Running, next->readAllStandardError().constData());
        QLocalSocket socket;
        socket.connectToServer(activationPath());
        QVERIFY(socket.waitForConnected(2000));
    }

    void cleanup() {
        for (QProcess *process : std::as_const(m_panels)) {
            if (process->state() != QProcess::NotRunning) {
                process->terminate();
                if (!process->waitForFinished(3000)) {
                    process->kill();
                    process->waitForFinished(3000);
                }
            }
            delete process;
        }
        m_panels.clear();
    }

private:
    QString sandbox(const QString &name) const { return m_sandbox->filePath(name); }
    QString activationPath() const { return sandbox("runtime/clipboard-share-panel.socket"); }
    QString lockPath() const { return sandbox("runtime/clipboard-share-panel.lock"); }

    QProcess *startPanel() {
        const QString binary = QStringLiteral(PANEL_BINARY);
        if (!QFileInfo::exists(binary))
            qFatal("panel binary %s is missing; bin/test builds it", qPrintable(binary));
        for (const char *dir : {"runtime", "tmp", "home"})
            QDir().mkpath(sandbox(dir));
        QFile::setPermissions(sandbox("runtime"), QFile::ReadOwner | QFile::WriteOwner | QFile::ExeOwner);

        auto env = QProcessEnvironment::systemEnvironment();
        for (const char *name : {"WAYLAND_DISPLAY", "DISPLAY", "HYPRLAND_INSTANCE_SIGNATURE", "QT_QPA_PLATFORMTHEME",
                                 "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"})
            env.remove(name);
        env.insert("QT_QPA_PLATFORM", "offscreen");
        env.insert("XDG_RUNTIME_DIR", sandbox("runtime"));
        env.insert("TMPDIR", sandbox("tmp"));
        env.insert("HOME", sandbox("home"));
        env.insert("DBUS_SESSION_BUS_ADDRESS", "unix:path=" + sandbox("no-bus"));
        env.insert("CLIPBOARD_SHARE_CTL_SOCK", sandbox("absent-ctl.sock"));

        auto *process = new QProcess;
        process->setProcessEnvironment(env);
        process->start(binary, {"--background"});
        if (!process->waitForStarted(5000))
            qFatal("could not start %s", qPrintable(binary));
        m_panels.append(process);
        return process;
    }

    QTemporaryDir m_dir;
    QString m_path;
    // A fresh runtime, temp and home directory per test for the panel processes.
    std::unique_ptr<QTemporaryDir> m_sandbox;
    QList<QProcess *> m_panels;
};

QTEST_MAIN(PanelTests)
#include "panel_tests.moc"
