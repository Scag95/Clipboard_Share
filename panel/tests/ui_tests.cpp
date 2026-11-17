// Drives the real Main.qml offscreen: real mouse clicks and key presses on the
// window, a real Backend, and a fake control socket standing in for the server.
// Asserts the commands the server receives, which is the observable effect.

#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QTemporaryDir>
#include <QtTest>

#include "backend.h"
#include "fakeserver.h"
#include "theme.h"

using namespace fake;

class UiTests : public QObject {
    Q_OBJECT

    struct Session {
        FakeServer server;
        Backend backend;
        Theme theme;
        QQmlApplicationEngine engine;
        QQuickWindow *window = nullptr;
        Session(const QString &path, std::function<void(FakeServer &)> setup)
            : server(path), backend((setup(server), path), 100), theme(path + ".theme") {
            engine.rootContext()->setContextProperty("theme", &theme);
            engine.rootContext()->setContextProperty("backend", &backend);
            engine.rootContext()->setContextProperty("startHidden", false);
            engine.load(QUrl("qrc:/Main.qml"));
            window = qobject_cast<QQuickWindow *>(engine.rootObjects().value(0));
        }
    };

    // The first visible item whose "text" property equals the given text (a button
    // label). Walks the visual tree: delegates and dialog content hang from their
    // visual parent, not from the QObject parent findChildren() would follow.
    static QQuickItem *itemWithText(QQuickItem *root, const QString &text) {
        if (root->isVisible() && root->property("text").toString() == text)
            return root;
        for (auto *child : root->childItems())
            if (auto *found = itemWithText(child, text))
                return found;
        return nullptr;
    }

    static QQuickItem *itemWithText(QQuickWindow *window, const QString &text) {
        // Popups (the revoke dialog) live in the overlay, outside the content item.
        if (auto *found = itemWithText(window->contentItem(), text))
            return found;
        return nullptr;
    }

    static void click(QQuickWindow *window, QQuickItem *item) {
        const QPointF center = item->mapToScene(QPointF(item->width() / 2, item->height() / 2));
        QTest::mouseClick(window, Qt::LeftButton, {}, center.toPoint());
    }

    static bool sawCommand(const FakeServer &server, const QString &cmd) {
        for (const auto &command : server.commands)
            if (command.value("cmd").toString() == cmd)
                return true;
        return false;
    }

private slots:
    void initTestCase() { QQuickStyle::setStyle("Material"); }
    void init() { QVERIFY(m_dir.isValid()); m_path = m_dir.filePath(QString("ctl%1.sock").arg(++m_n)); }

    void approveButtonSendsApproveWithTheRequestId() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r1", "Pixel", "192.168.0.13")}; });
        QVERIFY(s.window);
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Approve") != nullptr, 3000);

        click(s.window, itemWithText(s.window, "Approve"));

        QTRY_VERIFY_WITH_TIMEOUT(sawCommand(s.server, "approve"), 3000);
        for (const auto &command : s.server.commands)
            if (command.value("cmd").toString() == "approve")
                QCOMPARE(command.value("request").toString(), QString("r1"));
    }

    void denyButtonSendsDeny() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r2", "Pixel", "192.168.0.13")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Deny") != nullptr, 3000);

        click(s.window, itemWithText(s.window, "Deny"));

        QTRY_VERIFY_WITH_TIMEOUT(sawCommand(s.server, "deny"), 3000);
    }

    void keyAApprovesTheNewestRequest() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r3", "Pixel", "192.168.0.13")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Approve") != nullptr, 3000);
        s.window->show();
        s.window->requestActivate();
        QVERIFY(QTest::qWaitForWindowActive(s.window));

        QTest::keyClick(s.window, Qt::Key_A);

        QTRY_VERIFY_WITH_TIMEOUT(sawCommand(s.server, "approve-last"), 3000);
    }

    void revokeAsksForConfirmationAndOnlyThenRevokes() {
        Session s(m_path, [](FakeServer &srv) { srv.devices = {device("d1", "Galaxy", "192.168.0.20")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Revoke") != nullptr, 3000);

        click(s.window, itemWithText(s.window, "Revoke"));
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "OK") != nullptr, 3000);
        QVERIFY2(!sawCommand(s.server, "revoke"), "revoked before the user confirmed");

        click(s.window, itemWithText(s.window, "OK"));

        QTRY_VERIFY_WITH_TIMEOUT(sawCommand(s.server, "revoke"), 3000);
        for (const auto &command : s.server.commands)
            if (command.value("cmd").toString() == "revoke")
                QCOMPARE(command.value("device").toString(), QString("d1"));
    }

    void cancellingTheRevokeDialogSendsNothing() {
        Session s(m_path, [](FakeServer &srv) { srv.devices = {device("d1", "Galaxy", "192.168.0.20")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Revoke") != nullptr, 3000);
        click(s.window, itemWithText(s.window, "Revoke"));
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Cancel") != nullptr, 3000);

        click(s.window, itemWithText(s.window, "Cancel"));
        QTest::qWait(300);

        QVERIFY(!sawCommand(s.server, "revoke"));
    }

    void anUnreachableServerIsSaidInTheWindow() {
        Backend backend(m_dir.filePath("absent.sock"), 100);
        Theme theme(m_path + ".theme");
        QQmlApplicationEngine engine;
        engine.rootContext()->setContextProperty("theme", &theme);
        engine.rootContext()->setContextProperty("backend", &backend);
        engine.rootContext()->setContextProperty("startHidden", false);
        engine.load(QUrl("qrc:/Main.qml"));
        auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().value(0));
        QVERIFY(window);

        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(window, "Server not running") != nullptr, 3000);
    }

private:
    QTemporaryDir m_dir;
    QString m_path;
    int m_n = 0;
};

QTEST_MAIN(UiTests)
#include "ui_tests.moc"
