// Drives the real Main.qml offscreen: real mouse clicks and key presses on the
// window, a real Backend, and a fake control socket standing in for the server.
// Asserts the commands the server receives, which is the observable effect.

#include <QFileInfo>
#include <QGuiApplication>
#include <QImage>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickItem>
#include <QQuickStyle>
#include <QQuickWindow>
#include <QTemporaryDir>
#include <QtTest>

#include "autostart.h"
#include "backend.h"
#include "fakeserver.h"
#include "theme.h"

using namespace fake;

class UiTests : public QObject {
    Q_OBJECT

    // Plays systemctl for the autostart switches: records and answers.
    struct FakeSystemctl {
        QList<QStringList> calls;
        bool enabled = true;
        int failCode = 0;
        QString failOutput;
        Autostart::Runner runner() {
            return [this](const QStringList &args) -> Autostart::Result {
                calls.append(args);
                if (args.value(0) == "is-enabled")
                    return {enabled ? 0 : 1, {}};
                if (failCode != 0)
                    return {failCode, failOutput};
                enabled = args.value(0) == "enable" ? true : args.value(0) == "disable" ? false : enabled;
                return {0, {}};
            };
        }
    };

    struct Session {
        FakeServer server;
        Backend backend;
        Theme theme;
        QTemporaryDir home;
        FakeSystemctl systemctl;
        Autostart autostart;
        QQmlApplicationEngine engine;
        QQuickWindow *window = nullptr;
        Session(const QString &path, std::function<void(FakeServer &)> setup, bool startHidden = false)
            : server(path), backend((setup(server), path), 100), theme(path + ".theme"),
              autostart(home.filePath("state"), home.filePath("xdg"), systemctl.runner()) {
            engine.rootContext()->setContextProperty("theme", &theme);
            engine.rootContext()->setContextProperty("backend", &backend);
            engine.rootContext()->setContextProperty("autostart", &autostart);
            engine.rootContext()->setContextProperty("startHidden", startHidden);
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
        // Popups (the revoke dialog) are reparented to the window's overlay, which
        // is itself a child item of the content item, so this walk reaches them.
        return itemWithText(window->contentItem(), text);
    }

    static void click(QQuickWindow *window, QQuickItem *item) {
        const QPointF center = item->mapToScene(QPointF(item->width() / 2, item->height() / 2));
        QTest::mouseClick(window, Qt::LeftButton, {}, center.toPoint());
    }

    static int sawCount(const FakeServer &server, const QString &cmd) {
        int n = 0;
        for (const auto &command : server.commands)
            n += command.value("cmd").toString() == cmd;
        return n;
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

    // S5: the code the phone must also show sits in the pairing requests section.
    void theVerificationCodeIsShownInThePairingSection() {
        Session s(m_path, [](FakeServer &srv) {
            srv.code = "6B86-B273-FF34";
            srv.requests = {request("r7", "Pixel", "192.168.0.13")};
        });
        QVERIFY(s.window);
        const QString text = "Verification code: 6B86-B273-FF34";
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, text) != nullptr, 3000);
        QCOMPARE(itemWithText(s.window, text)->property("textFormat").toInt(), int(Qt::PlainText));
    }

    void noCodeIsShownWithoutOne() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r8", "Pixel", "192.168.0.13")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Approve") != nullptr, 3000);
        QVERIFY(itemWithText(s.window, "Verification code: ") == nullptr);
    }

    // Polls that bring the same list must not rebuild the cards: a click that
    // crosses a rebuild would land on a delegate being destroyed.
    void unchangedPollsKeepTheSameCards() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r6", "Pixel", "192.168.0.13")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Approve") != nullptr, 3000);
        const QPointer<QQuickItem> button = itemWithText(s.window, "Approve");
        const int polls = sawCount(s.server, "list");

        QTRY_VERIFY_WITH_TIMEOUT(sawCount(s.server, "list") >= polls + 3, 3000);
        QTest::qWait(150);

        QVERIFY2(!button.isNull(), "a poll with the same list destroyed the card");
        QCOMPARE(itemWithText(s.window, "Approve"), button.data());
    }

    void denyButtonSendsDeny() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r2", "Pixel", "192.168.0.13")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Deny") != nullptr, 3000);

        click(s.window, itemWithText(s.window, "Deny"));

        QTRY_VERIFY_WITH_TIMEOUT(sawCommand(s.server, "deny"), 3000);
    }

    // A pairing request can come from anyone on the LAN. If it opened the window and
    // a bare key approved it, typing in another app at the wrong moment would pair
    // an attacker with the desktop's mouse and keyboard.
    void aKeyPressNeverApprovesARequest() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r3", "Pixel", "192.168.0.13")}; });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Approve") != nullptr, 3000);
        s.window->show();
        s.window->requestActivate();
        QVERIFY(QTest::qWaitForWindowActive(s.window));

        QTest::keyClick(s.window, Qt::Key_A);
        QTest::keyClick(s.window, Qt::Key_A, Qt::ControlModifier);
        QTest::qWait(400);

        QVERIFY(!sawCommand(s.server, "approve-last"));
        QVERIFY(!sawCommand(s.server, "approve"));
    }

    // The device name comes from an unauthenticated request. Rendered as rich text,
    // markup in it could fake the approval card exactly where the user decides.
    void remoteSuppliedTextIsShownAsPlainText() {
        const QString hostile = "<h1>Trusted laptop</h1>";
        Session s(m_path, [&](FakeServer &srv) {
            srv.requests = {request("r5", hostile, "<b>10.0.0.1</b>")};
            srv.devices = {device("d5", hostile, "<b>10.0.0.2</b>")};
        });
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, hostile) != nullptr, 3000);

        QCOMPARE(itemWithText(s.window, hostile)->property("textFormat").toInt(), int(Qt::PlainText));
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Revoke") != nullptr, 3000);
        click(s.window, itemWithText(s.window, "Revoke"));
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, "Cancel") != nullptr, 3000);
        const auto dialogText = QString("%1 will lose access and has to ask for approval again.").arg(hostile);
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, dialogText) != nullptr, 3000);
        QCOMPARE(itemWithText(s.window, dialogText)->property("textFormat").toInt(), int(Qt::PlainText));
    }

    void aNewRequestDoesNotOpenAHiddenWindow() {
        Session s(m_path, [](FakeServer &srv) { srv.requests = {request("r4", "Attacker", "10.0.0.9")}; }, true);
        QVERIFY(s.window);
        QVERIFY(!s.window->isVisible());

        QTRY_COMPARE_WITH_TIMEOUT(s.backend.requests().size(), 1, 3000);
        QTest::qWait(300);

        QVERIFY2(!s.window->isVisible(), "a request from the network opened the window and took focus");
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

    // The login switches drive systemctl and the XDG override, and spring back
    // with the reason when a change fails.
    void theSettingsSwitchesControlTheirAutostart() {
        Session s(m_path, [](FakeServer &) {});
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, kPanelSwitch) != nullptr, 3000);
        QQuickItem *panel = itemWithText(s.window, kPanelSwitch);
        QQuickItem *server = itemWithText(s.window, kServerSwitch);
        QVERIFY(server);
        QVERIFY(panel->property("checked").toBool());
        QVERIFY(server->property("checked").toBool());
        const QString override = s.home.filePath("xdg/autostart/nudge-panel.desktop");

        click(s.window, panel);
        QTRY_VERIFY_WITH_TIMEOUT(QFileInfo::exists(override), 3000);
        QFile written(override);
        QVERIFY(written.open(QIODevice::ReadOnly));
        QVERIFY(written.readAll().contains("\nHidden=true\n"));
        QVERIFY(!panel->property("checked").toBool());

        click(s.window, panel);
        QTRY_VERIFY_WITH_TIMEOUT(!QFileInfo::exists(override), 3000);
        QVERIFY(panel->property("checked").toBool());

        click(s.window, server);
        QTRY_COMPARE_WITH_TIMEOUT(s.systemctl.calls.value(s.systemctl.calls.size() - 1),
                                  (QStringList{"disable", Autostart::Unit}), 3000);
        QVERIFY(!server->property("checked").toBool());
        click(s.window, server);
        QTRY_COMPARE_WITH_TIMEOUT(s.systemctl.calls.value(s.systemctl.calls.size() - 1),
                                  (QStringList{"enable", Autostart::Unit}), 3000);
        QVERIFY(server->property("checked").toBool());
    }

    void aFailingSwitchSpringsBackAndSaysWhy() {
        Session s(m_path, [](FakeServer &) {});
        s.systemctl.failCode = 1;
        s.systemctl.failOutput = "Failed to disable unit: Access denied";
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, kServerSwitch) != nullptr, 3000);
        QQuickItem *server = itemWithText(s.window, kServerSwitch);

        click(s.window, server);

        const QString error = "No se pudo desactivar el inicio del servidor: Failed to disable unit: Access denied";
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, error) != nullptr, 3000);
        QVERIFY(server->property("checked").toBool());
        QVERIFY(s.autostart.serverEnabled());
    }

    // A failed first-run enable stays visible in the Settings card (S10).
    void aFailedFirstRunEnableIsShownInSettings() {
        Session s(m_path, [](FakeServer &) {});
        s.systemctl.enabled = false;
        s.systemctl.failCode = 1;
        s.systemctl.failOutput = "Failed to enable unit: boom";

        s.autostart.applyFirstRunDefaults();

        const QString error = "No se pudo activar el inicio del servidor: Failed to enable unit: boom";
        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(s.window, error) != nullptr, 3000);
        QVERIFY(!QFileInfo::exists(s.autostart.markerPath()));
    }

    // The app is called Nudge in the window and uses the Nudge icon (S1).
    void theWindowAndIconSayNudge() {
        Session s(m_path, [](FakeServer &) {});
        QVERIFY(s.window);
        QCOMPARE(s.window->title(), QString("Nudge"));

        QImage icon(":/nudge.svg");
        if (icon.isNull())
            QSKIP("no SVG image plugin to render the icon");
        icon = icon.scaled(108, 108).convertToFormat(QImage::Format_ARGB32);
        // White cursor, teal background, transparent rounded corner.
        const QColor cursor = icon.pixelColor(52, 50);
        const QColor background = icon.pixelColor(54, 4);
        QVERIFY2(cursor.red() > 240 && cursor.green() > 240 && cursor.blue() > 240, qPrintable(cursor.name()));
        QVERIFY2(background.red() < 30 && background.green() > 80 && background.blue() > 80,
                 qPrintable(background.name()));
        QCOMPARE(icon.pixelColor(0, 0).alpha(), 0);
    }

    void anUnreachableServerIsSaidInTheWindow() {
        Backend backend(m_dir.filePath("absent.sock"), 100);
        Theme theme(m_path + ".theme");
        FakeSystemctl systemctl;
        Autostart autostart(m_dir.filePath("state"), m_dir.filePath("xdg"), systemctl.runner());
        QQmlApplicationEngine engine;
        engine.rootContext()->setContextProperty("theme", &theme);
        engine.rootContext()->setContextProperty("backend", &backend);
        engine.rootContext()->setContextProperty("autostart", &autostart);
        engine.rootContext()->setContextProperty("startHidden", false);
        engine.load(QUrl("qrc:/Main.qml"));
        auto *window = qobject_cast<QQuickWindow *>(engine.rootObjects().value(0));
        QVERIFY(window);

        QTRY_VERIFY_WITH_TIMEOUT(itemWithText(window, "Server not running") != nullptr, 3000);
        QVERIFY(itemWithText(window, "Start it with: systemctl --user start nudge") != nullptr);
    }

private:
    static constexpr const char *kServerSwitch = "Iniciar el servidor al iniciar sesión";
    static constexpr const char *kPanelSwitch = "Abrir el panel al iniciar sesión";
    QTemporaryDir m_dir;
    QString m_path;
    int m_n = 0;
};

QTEST_MAIN(UiTests)
#include "ui_tests.moc"
