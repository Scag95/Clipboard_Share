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

#include "autostart.h"
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

QByteArray readFile(const QString &path) {
    QFile file(path);
    return file.open(QIODevice::ReadOnly) ? file.readAll() : QByteArray();
}

bool writeFile(const QString &path, const QByteArray &data) {
    QFile file(path);
    return file.open(QIODevice::WriteOnly) && file.write(data) == data.size();
}

// The value of `key=` in a desktop entry.
QString desktopValue(const QByteArray &entry, const QString &key) {
    for (const auto &line : QString::fromUtf8(entry).split('\n'))
        if (line.startsWith(key + "="))
            return line.mid(key.size() + 1).trimmed();
    return {};
}

// Plays systemctl: records every call and answers from a script.
struct FakeSystemctl {
    QList<QStringList> calls;
    bool enabled = true;
    int failCode = 0;
    QString failOutput;

    Autostart::Runner runner() {
        return [this](const QStringList &args) -> Autostart::Result {
            calls.append(args);
            if (args.value(0) == "is-enabled")
                return {enabled ? 0 : 1, enabled ? "enabled\n" : "disabled\n"};
            if (failCode != 0)
                return {failCode, failOutput};
            if (args.value(0) == "enable")
                enabled = true;
            if (args.value(0) == "disable")
                enabled = false;
            return {0, {}};
        };
    }

    int count(const QStringList &args) const { return int(calls.count(args)); }
};

// Sets or clears an environment variable for one scope, then restores it.
class ScopedEnv {
public:
    ScopedEnv(const char *name, const QByteArray &value) : m_name(name), m_had(qEnvironmentVariableIsSet(name)),
                                                         m_old(qgetenv(name)) {
        if (value.isNull())
            qunsetenv(name);
        else
            qputenv(name, value);
    }
    ~ScopedEnv() {
        if (m_had)
            qputenv(m_name, m_old);
        else
            qunsetenv(m_name);
    }

private:
    const char *m_name;
    bool m_had;
    QByteArray m_old;
};

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
        ScopedEnv fresh("NUDGE_CTL_SOCK", "/tmp/nudge-ctl.sock");
        ScopedEnv old("CLIPBOARD_SHARE_CTL_SOCK", "/tmp/custom-ctl.sock");
        Backend backend;
        QCOMPARE(backend.socketPath(), QString("/tmp/nudge-ctl.sock"));
    }

    void socketPathFallsBackToTheOldEnvironmentName() {
        ScopedEnv fresh("NUDGE_CTL_SOCK", QByteArray());
        ScopedEnv old("CLIPBOARD_SHARE_CTL_SOCK", "/tmp/custom-ctl.sock");
        Backend backend;
        QCOMPARE(backend.socketPath(), QString("/tmp/custom-ctl.sock"));
    }

    // S3 on the panel's side: the old directory moves to the new name once.
    void theOldConfigDirectoryMovesToNudge() {
        ScopedEnv home("HOME", sandbox("home").toUtf8());
        ScopedEnv fresh("NUDGE_CTL_SOCK", QByteArray());
        ScopedEnv old("CLIPBOARD_SHARE_CTL_SOCK", QByteArray());
        QVERIFY(QDir().mkpath(sandbox("home/.config/clipboard-share")));
        QVERIFY(writeFile(sandbox("home/.config/clipboard-share/state.json"), "{\"devices\":[1]}"));

        Backend backend;

        QCOMPARE(backend.socketPath(), sandbox("home/.config/nudge/ctl.sock"));
        QCOMPARE(readFile(sandbox("home/.config/nudge/state.json")), QByteArray("{\"devices\":[1]}"));
        QVERIFY(!QFileInfo::exists(sandbox("home/.config/clipboard-share")));
    }

    void anExistingNudgeDirectoryIsNeverOverwritten() {
        ScopedEnv home("HOME", sandbox("home").toUtf8());
        ScopedEnv fresh("NUDGE_CTL_SOCK", QByteArray());
        ScopedEnv old("CLIPBOARD_SHARE_CTL_SOCK", QByteArray());
        QVERIFY(QDir().mkpath(sandbox("home/.config/clipboard-share")));
        QVERIFY(QDir().mkpath(sandbox("home/.config/nudge")));
        QVERIFY(writeFile(sandbox("home/.config/clipboard-share/state.json"), "old"));
        QVERIFY(writeFile(sandbox("home/.config/nudge/state.json"), "new"));

        Backend backend;

        QCOMPARE(backend.socketPath(), sandbox("home/.config/nudge/ctl.sock"));
        QCOMPARE(readFile(sandbox("home/.config/nudge/state.json")), QByteArray("new"));
        QCOMPARE(readFile(sandbox("home/.config/clipboard-share/state.json")), QByteArray("old"));
    }

    // Autostart: the panel's two login switches (S4, S5).

    void disablingThePanelWritesAHiddenOverrideLikeThePackagedEntry() {
        FakeSystemctl systemctl;
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        QSignalSpy changed(&autostart, &Autostart::changed);
        QVERIFY(autostart.panelEnabled());

        QVERIFY(autostart.setPanelEnabled(false));

        QVERIFY(!autostart.panelEnabled());
        QCOMPARE(autostart.error(), QString());
        QVERIFY(changed.count() >= 1);
        QCOMPARE(autostart.overridePath(), sandbox("xdg/autostart/nudge-panel.desktop"));
        const QByteArray written = readFile(sandbox("xdg/autostart/nudge-panel.desktop"));
        const QByteArray packaged = readFile(QStringLiteral(PACKAGED_AUTOSTART));
        QVERIFY2(!packaged.isEmpty(), PACKAGED_AUTOSTART);
        QVERIFY2(written.startsWith("[Desktop Entry]\n"), written.constData());
        QCOMPARE(desktopValue(written, "Hidden"), QString("true"));
        QCOMPARE(desktopValue(written, "Name"), desktopValue(packaged, "Name"));
        QCOMPARE(desktopValue(written, "Exec"), desktopValue(packaged, "Exec"));
        QCOMPARE(desktopValue(packaged, "Name"), QString("Nudge"));
        QCOMPARE(desktopValue(packaged, "Exec"), QString("nudge-panel --background"));
        QVERIFY(systemctl.count({"enable", Autostart::Unit}) == 0 && systemctl.count({"disable", Autostart::Unit}) == 0);
    }

    void enablingThePanelRemovesTheOverrideAndNothingElse() {
        FakeSystemctl systemctl;
        QVERIFY(QDir().mkpath(sandbox("xdg/autostart")));
        QVERIFY(writeFile(sandbox("xdg/autostart/other.desktop"), "[Desktop Entry]\nName=Other\nHidden=true\n"));
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());

        QVERIFY(autostart.setPanelEnabled(false));
        QVERIFY(autostart.setPanelEnabled(true));

        QVERIFY(autostart.panelEnabled());
        QCOMPARE(autostart.error(), QString());
        QVERIFY(!QFileInfo::exists(sandbox("xdg/autostart/nudge-panel.desktop")));
        QCOMPARE(QDir(sandbox("xdg/autostart")).entryList(QDir::Files), QStringList{"other.desktop"});
        QCOMPARE(readFile(sandbox("xdg/autostart/other.desktop")), QByteArray("[Desktop Entry]\nName=Other\nHidden=true\n"));
    }

    // A user override without Hidden=true does not disable the panel, and
    // turning the switch on leaves it alone.
    void aUserOverrideWithoutHiddenCountsAsEnabledAndIsKept() {
        FakeSystemctl systemctl;
        QVERIFY(QDir().mkpath(sandbox("xdg/autostart")));
        QVERIFY(writeFile(sandbox("xdg/autostart/nudge-panel.desktop"), "[Desktop Entry]\nExec=nudge-panel\n"));
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        QVERIFY(autostart.panelEnabled());

        QVERIFY(autostart.setPanelEnabled(true));

        QCOMPARE(readFile(sandbox("xdg/autostart/nudge-panel.desktop")), QByteArray("[Desktop Entry]\nExec=nudge-panel\n"));
    }

    void aPanelOverrideThatCannotBeWrittenIsReported() {
        FakeSystemctl systemctl;
        QVERIFY(QDir().mkpath(sandbox("xdg")));
        // A file where the autostart directory should be.
        QVERIFY(writeFile(sandbox("xdg/autostart"), "not a directory"));
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        QSignalSpy changed(&autostart, &Autostart::changed);

        QVERIFY(!autostart.setPanelEnabled(false));

        QVERIFY(autostart.panelEnabled());
        QVERIFY2(autostart.error().contains(sandbox("xdg/autostart")), qPrintable(autostart.error()));
        QVERIFY(changed.count() >= 1);
        QCOMPARE(readFile(sandbox("xdg/autostart")), QByteArray("not a directory"));
    }

    void theServerSwitchEnablesAndDisablesTheUserUnit() {
        FakeSystemctl systemctl;
        systemctl.enabled = true;
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        QVERIFY(autostart.serverEnabled());
        QCOMPARE(systemctl.count({"is-enabled", Autostart::Unit}), 1);

        QVERIFY(autostart.setServerEnabled(false));
        QVERIFY(!autostart.serverEnabled());
        QCOMPARE(systemctl.calls.last(), (QStringList{"disable", Autostart::Unit}));

        QVERIFY(autostart.setServerEnabled(true));
        QVERIFY(autostart.serverEnabled());
        QCOMPARE(systemctl.calls.last(), (QStringList{"enable", Autostart::Unit}));
        QCOMPARE(autostart.error(), QString());
        // The server switch never touches the panel's autostart.
        QVERIFY(!QFileInfo::exists(sandbox("xdg/autostart")));
    }

    void aSystemctlFailureIsReportedAndKeepsTheSwitch() {
        FakeSystemctl systemctl;
        systemctl.enabled = true;
        systemctl.failCode = 1;
        systemctl.failOutput = "Failed to disable unit: Access denied\n";
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        QSignalSpy changed(&autostart, &Autostart::changed);

        QVERIFY(!autostart.setServerEnabled(false));

        QVERIFY(autostart.serverEnabled());
        QVERIFY2(autostart.error().contains("Failed to disable unit: Access denied"), qPrintable(autostart.error()));
        QVERIFY(changed.count() >= 1);

        // The next success clears it.
        systemctl.failCode = 0;
        QVERIFY(autostart.setServerEnabled(false));
        QCOMPARE(autostart.error(), QString());
    }

    void theFirstRunEnablesTheServerOnceAndNeverAgain() {
        FakeSystemctl systemctl;
        systemctl.enabled = false;
        {
            Autostart first(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
            QVERIFY(!first.serverEnabled());
            first.applyFirstRunDefaults();
            QVERIFY(first.serverEnabled());
            first.applyFirstRunDefaults();
            QCOMPARE(systemctl.count({"enable", Autostart::Unit}), 1);
            // The user turns it off.
            QVERIFY(first.setServerEnabled(false));
        }
        Autostart later(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        later.applyFirstRunDefaults();

        QVERIFY(!later.serverEnabled());
        QCOMPARE(systemctl.count({"enable", Autostart::Unit}), 1);
        QVERIFY(later.panelEnabled());
        QVERIFY(!QFileInfo::exists(sandbox("xdg/autostart")));
    }

    void thePanelStartsAnOfflineServerOnce() {
        FakeSystemctl systemctl;
        systemctl.enabled = false;
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        Backend backend(m_dir.filePath("nope.sock"), 0);
        QSignalSpy changed(&backend, &Backend::changed);
        autostart.startServerWhenOffline(&backend);

        backend.refresh();
        QVERIFY(waitChanged(changed));
        QTRY_COMPARE_WITH_TIMEOUT(systemctl.count({"start", Autostart::Unit}), 1, 2000);
        backend.refresh();
        settle();
        QCOMPARE(systemctl.count({"start", Autostart::Unit}), 1);
        // Starting it now says nothing about logging in.
        QCOMPARE(systemctl.count({"enable", Autostart::Unit}), 0);
    }

    // The backend may already know the server is down when the panel asks.
    void aServerAlreadyKnownOfflineIsStarted() {
        FakeSystemctl systemctl;
        systemctl.failCode = 5;
        systemctl.failOutput = "Unit nudge.service not found.";
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        Backend backend(m_dir.filePath("nope.sock"), 0);
        QSignalSpy changed(&backend, &Backend::changed);
        backend.refresh();
        QVERIFY(waitChanged(changed));

        autostart.startServerWhenOffline(&backend);

        QCOMPARE(systemctl.count({"start", Autostart::Unit}), 1);
        QVERIFY2(autostart.error().contains("Unit nudge.service not found."), qPrintable(autostart.error()));
    }

    void thePanelLeavesARunningServerAlone() {
        FakeServer server(m_path);
        FakeSystemctl systemctl;
        Autostart autostart(sandbox("nudge"), sandbox("xdg"), systemctl.runner());
        Backend backend(m_path, 0);
        QSignalSpy changed(&backend, &Backend::changed);
        autostart.startServerWhenOffline(&backend);

        backend.refresh();
        QVERIFY(waitChanged(changed));
        settle();
        QVERIFY(backend.online());
        QCOMPARE(systemctl.count({"start", Autostart::Unit}), 0);
    }

    // The real panel binary, with a fake systemctl first on PATH.
    void aLaunchedPanelStartsTheServerAndEnablesItOnTheFirstRunOnly() {
        QProcess *first = startPanel();
        QTRY_VERIFY_WITH_TIMEOUT(systemctlLog().contains("--user start nudge.service"), 10000);
        QVERIFY(first->state() == QProcess::Running);
        QCOMPARE(systemctlLog().count("--user enable nudge.service"), 1);
        QVERIFY(QFileInfo::exists(sandbox("home/.config/nudge")));
        cleanup();

        QProcess *second = startPanel();
        QTRY_VERIFY_WITH_TIMEOUT(systemctlLog().count("--user start nudge.service") == 2, 10000);
        QVERIFY(second->state() == QProcess::Running);
        QCOMPARE(systemctlLog().count("--user enable nudge.service"), 1);
    }

    // Single instance, driven through the built panel binary, offscreen and
    // isolated: its own runtime, temp and home directories, no session bus, and
    // a control socket nobody serves.

    void theActivationSocketLivesInTheRuntimeDirectory() {
        QProcess *first = startPanel();
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(activationPath()), 10000);
        QVERIFY(first->state() == QProcess::Running);
        QVERIFY2(!QFileInfo::exists(sandbox("tmp/nudge-panel")),
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
    QString activationPath() const { return sandbox("runtime/nudge-panel.socket"); }
    QString lockPath() const { return sandbox("runtime/nudge-panel.lock"); }
    QString systemctlLog() const { return QString::fromUtf8(readFile(sandbox("systemctl.log"))); }

    QProcess *startPanel() {
        const QString binary = QStringLiteral(PANEL_BINARY);
        if (!QFileInfo::exists(binary))
            qFatal("panel binary %s is missing; bin/test builds it", qPrintable(binary));
        for (const char *dir : {"runtime", "tmp", "home", "bin"})
            QDir().mkpath(sandbox(dir));
        QFile::setPermissions(sandbox("runtime"), QFile::ReadOwner | QFile::WriteOwner | QFile::ExeOwner);
        // The panel runs systemctl; this one only records the call, so the
        // tests never reach the user's real units.
        const QString fake = sandbox("bin/systemctl");
        if (!QFileInfo::exists(fake)) {
            const QByteArray script = "#!/bin/sh\necho \"$*\" >> '" + sandbox("systemctl.log").toUtf8() +
                                      "'\ncase \"$2\" in is-enabled) echo disabled; exit 1;; esac\nexit 0\n";
            if (!writeFile(fake, script))
                qFatal("could not write %s", qPrintable(fake));
            QFile::setPermissions(fake, QFile::ReadOwner | QFile::WriteOwner | QFile::ExeOwner);
        }

        auto env = QProcessEnvironment::systemEnvironment();
        for (const char *name : {"WAYLAND_DISPLAY", "DISPLAY", "HYPRLAND_INSTANCE_SIGNATURE", "QT_QPA_PLATFORMTHEME",
                                 "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"})
            env.remove(name);
        env.insert("QT_QPA_PLATFORM", "offscreen");
        env.insert("XDG_RUNTIME_DIR", sandbox("runtime"));
        env.insert("TMPDIR", sandbox("tmp"));
        env.insert("HOME", sandbox("home"));
        env.insert("DBUS_SESSION_BUS_ADDRESS", "unix:path=" + sandbox("no-bus"));
        env.insert("NUDGE_CTL_SOCK", sandbox("absent-ctl.sock"));
        env.remove("CLIPBOARD_SHARE_CTL_SOCK");
        env.insert("PATH", sandbox("bin") + ":" + env.value("PATH"));

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
