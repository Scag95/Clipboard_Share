// nudge-panel - approve phones and manage paired devices for the Nudge
// server, from a window and a tray icon.

#include <QCommandLineParser>
#include <QDir>
#include <QElapsedTimer>
#include <QGuiApplication>
#include <QIcon>
#include <QLocalServer>
#include <QLocalSocket>
#include <QLockFile>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QStandardPaths>
#include <QSysInfo>
#include <QThread>
#include <QUrl>

#include <cerrno>
#include <csignal>
#include <cstdio>

#include "autostart.h"
#include "backend.h"
#include "theme.h"

namespace {

// The user's runtime directory, private to them, like the lock beside it.
QString runtimeDirectory() {
    const QString runtime = QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
    return runtime.isEmpty() ? QDir::tempPath() : runtime;
}

// Asks the already-running panel to show its window. A panel that is still
// starting listens moments after taking the lock, so keep trying for a while.
// Returns false when nobody answers in that time.
bool activateRunningInstance(const QString &path) {
    QElapsedTimer clock;
    clock.start();
    while (clock.elapsed() < 2000) {
        QLocalSocket socket;
        socket.connectToServer(path);
        if (socket.waitForConnected(200)) {
            socket.write("show\n");
            socket.waitForBytesWritten(500);
            return true;
        }
        QThread::msleep(100);
    }
    return false;
}

// Whether the process that holds the lock still runs. A lock from another
// host cannot be checked, so it counts as alive.
bool lockHolderAlive(qint64 pid, const QString &host) {
    if (host != QSysInfo::machineHostName())
        return true;
    return pid > 0 && (::kill(pid_t(pid), 0) == 0 || errno == EPERM);
}

} // namespace

int main(int argc, char *argv[]) {
    QGuiApplication app(argc, argv);
    app.setApplicationName("nudge-panel");
    app.setApplicationVersion("0.1.0");

    // Matches the window to nudge.desktop, so the compositor (Wayland app_id)
    // and the launcher pick up the installed icon.
    app.setDesktopFileName("nudge");
    app.setWindowIcon(QIcon::fromTheme("nudge", QIcon(":/nudge.svg")));

    QCommandLineParser parser;
    parser.setApplicationDescription("Nudge: approve phones, manage paired devices and choose what starts at login.");
    parser.addHelpOption();
    parser.addVersionOption();
    QCommandLineOption background("background", "Start hidden in the tray (for autostart).");
    parser.addOption(background);
    parser.process(app);

    // One panel per session: a second launch only raises the first.
    const QString runtime = runtimeDirectory();
    const QString activationPath = runtime + "/nudge-panel.socket";
    QLockFile lock(runtime + "/nudge-panel.lock");
    lock.setStaleLockTime(0);
    if (!lock.tryLock(0)) {
        if (activateRunningInstance(activationPath))
            return 0;
        // Only a lock whose owner is gone may be removed; a live panel that does
        // not answer keeps it, rather than two panels running side by side.
        qint64 pid = 0;
        QString host, owner;
        if (lock.getLockInfo(&pid, &host, &owner) && lockHolderAlive(pid, host)) {
            std::fprintf(stderr, "nudge-panel: another panel (pid %lld) holds %s but does not answer\n",
                         static_cast<long long>(pid), qPrintable(lock.fileName()));
            return 1;
        }
        lock.removeStaleLockFile();
        if (!lock.tryLock(0)) {
            std::fprintf(stderr, "nudge-panel: cannot take the lock %s\n", qPrintable(lock.fileName()));
            return 1;
        }
    }

    // Listen for "show" from a second launch right after taking the lock, before
    // the slow QML load, so a launch in between finds the socket. A socket left
    // by a panel that died is ours to replace: we hold the lock.
    QQuickWindow *window = nullptr;
    bool showRequested = false;
    QLocalServer activation;
    activation.setSocketOptions(QLocalServer::UserAccessOption);
    QObject::connect(&activation, &QLocalServer::newConnection, &app, [&] {
        while (auto *client = activation.nextPendingConnection()) {
            client->deleteLater();
            if (window)
                QMetaObject::invokeMethod(window, "showPanel");
            else
                showRequested = true;
        }
    });
    QLocalServer::removeServer(activationPath);
    if (!activation.listen(activationPath))
        std::fprintf(stderr, "nudge-panel: cannot listen on %s (%s); a second launch will not raise this one\n",
                     qPrintable(activationPath), qPrintable(activation.errorString()));

    QQuickStyle::setStyle("Material");

    Theme theme;
    // The real systemctl runs asynchronously here, so the startup calls never
    // hold back the window. The panel never moves or creates the server's
    // config directory; only the server migrates it.
    Autostart autostart;
    autostart.applyFirstRunDefaults();
    Backend backend;
    // Opening the panel starts the server, whatever the login switch says.
    autostart.startServerWhenOffline(&backend);

    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("theme", &theme);
    engine.rootContext()->setContextProperty("backend", &backend);
    engine.rootContext()->setContextProperty("autostart", &autostart);
    engine.rootContext()->setContextProperty("startHidden", parser.isSet(background));
    engine.load(QUrl("qrc:/Main.qml"));
    if (engine.rootObjects().isEmpty())
        return 1;

    window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());
    if (showRequested)
        QMetaObject::invokeMethod(window, "showPanel");

    return app.exec();
}
