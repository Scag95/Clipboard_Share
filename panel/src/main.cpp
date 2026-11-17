// clipboard-share-panel - approve phones and manage paired devices for the
// Clipboard Share server, from a window and a tray icon.

#include <QCommandLineParser>
#include <QDir>
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
#include <QUrl>

#include "backend.h"
#include "theme.h"

namespace {

const QString kActivationName = "clipboard-share-panel";

// Asks the already-running panel to show its window. Returns false when there
// is none to ask, so a stale lock never keeps the panel from starting.
bool activateRunningInstance() {
    QLocalSocket socket;
    socket.connectToServer(kActivationName);
    if (!socket.waitForConnected(500))
        return false;
    socket.write("show\n");
    socket.waitForBytesWritten(500);
    return true;
}

} // namespace

int main(int argc, char *argv[]) {
    QGuiApplication app(argc, argv);
    app.setApplicationName("clipboard-share-panel");
    app.setApplicationVersion("0.1.0");

    // Matches the window to clipboard-share.desktop, so the compositor (Wayland
    // app_id) and the launcher pick up the installed icon.
    app.setDesktopFileName("clipboard-share");
    app.setWindowIcon(QIcon::fromTheme("clipboard-share", QIcon(":/clipboard-share.svg")));

    QCommandLineParser parser;
    parser.setApplicationDescription("Approve phones and manage paired devices for Clipboard Share.");
    parser.addHelpOption();
    parser.addVersionOption();
    QCommandLineOption background("background", "Start hidden in the tray (for autostart).");
    parser.addOption(background);
    parser.process(app);

    // One panel per session: a second launch only raises the first.
    const QString runtime = QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
    QLockFile lock((runtime.isEmpty() ? QDir::tempPath() : runtime) + "/clipboard-share-panel.lock");
    lock.setStaleLockTime(0);
    if (!lock.tryLock(0)) {
        if (activateRunningInstance())
            return 0;
        // Lock held but nobody answers: the previous panel died without cleanup.
        lock.removeStaleLockFile();
        if (!lock.tryLock(0))
            return 1;
    }

    QQuickStyle::setStyle("Material");

    Theme theme;
    Backend backend;

    QQmlApplicationEngine engine;
    engine.rootContext()->setContextProperty("theme", &theme);
    engine.rootContext()->setContextProperty("backend", &backend);
    engine.rootContext()->setContextProperty("startHidden", parser.isSet(background));
    engine.load(QUrl("qrc:/Main.qml"));
    if (engine.rootObjects().isEmpty())
        return 1;

    auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().first());

    // Receives "show" from a second launch.
    QLocalServer activation;
    QLocalServer::removeServer(kActivationName);
    QObject::connect(&activation, &QLocalServer::newConnection, &app, [&activation, window] {
        if (auto *client = activation.nextPendingConnection()) {
            client->deleteLater();
            QMetaObject::invokeMethod(window, "showPanel");
        }
    });
    activation.listen(kActivationName);

    return app.exec();
}
