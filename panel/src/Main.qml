import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Material
import QtQuick.Layouts
import Qt.labs.platform as Platform

ApplicationWindow {
    id: window

    width: 440
    height: 620
    minimumWidth: 380
    minimumHeight: 440
    visible: !startHidden
    title: "Clipboard Share"
    color: "#14161a"

    Material.theme: Material.Dark
    Material.accent: theme.accent
    Material.roundedScale: theme.radius

    // Without a tray there is nothing to bring the window back, so closing quits.
    property bool hasTray: tray.available
    property string revokeId: ""
    property string revokeName: ""

    function showPanel() {
        show()
        raise()
        requestActivate()
    }

    onClosing: function(close) {
        if (hasTray) {
            close.accepted = false
            hide()
        }
    }

    // Anyone on the LAN can send a pairing request, so a request must never take
    // focus or open the window: the user could be typing in another app and a
    // stray keypress or click would approve a stranger. It is announced from the
    // tray instead, and the user opens the panel on purpose.
    Connections {
        target: backend
        function onNewRequest(name, ip) {
            tray.showMessage(qsTr("Pairing request"), qsTr("%1 (%2) wants to pair").arg(name).arg(ip),
                             Platform.SystemTrayIcon.Information, 15000)
        }
    }

    Platform.SystemTrayIcon {
        id: tray
        visible: true
        icon.source: "qrc:/clipboard-share.svg"
        tooltip: backend.requests.length > 0
                 ? qsTr("Clipboard Share - %n pairing request(s)", "", backend.requests.length)
                 : qsTr("Clipboard Share")
        menu: Platform.Menu {
            Platform.MenuItem { text: qsTr("Open"); onTriggered: window.showPanel() }
            Platform.MenuSeparator {}
            Platform.MenuItem { text: qsTr("Quit"); onTriggered: Qt.quit() }
        }
        onMessageClicked: window.showPanel()
        onActivated: function(reason) {
            if (reason === Platform.SystemTrayIcon.Trigger)
                window.visible ? window.hide() : window.showPanel()
        }
    }

    Shortcut { sequence: "Q"; context: Qt.ApplicationShortcut; onActivated: Qt.quit() }
    Shortcut { sequence: "?"; context: Qt.ApplicationShortcut; onActivated: help.open() }
    Shortcut { sequence: "Escape"; context: Qt.ApplicationShortcut; onActivated: window.hasTray ? window.hide() : Qt.quit() }

    function ago(iso) {
        const seconds = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
        if (isNaN(seconds)) return ""
        if (seconds < 60) return qsTr("%n second(s) ago", "", seconds)
        return qsTr("%n minute(s) ago", "", Math.round(seconds / 60))
    }

    function day(iso) {
        const date = new Date(iso)
        return isNaN(date.getTime()) ? "" : date.toLocaleDateString(Qt.locale())
    }

    component Section: Label {
        Layout.fillWidth: true
        Layout.topMargin: 14
        font.pixelSize: 12
        font.bold: true
        font.letterSpacing: 1
        color: "#9aa0a6"
    }

    component Card: Rectangle {
        default property alias content: inner.data
        Layout.fillWidth: true
        implicitHeight: inner.implicitHeight + 24
        radius: theme.radius
        color: "#1d2026"
        border.color: "#2b2f36"
        ColumnLayout {
            id: inner
            anchors.fill: parent
            anchors.margins: 12
            spacing: 8
        }
    }

    // Capped and centered so a tiled window does not stretch the cards across the screen.
    ColumnLayout {
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.margins: 16
        width: Math.min(parent.width - 32, 560)
        spacing: 8

        RowLayout {
            Layout.fillWidth: true
            spacing: 10
            Rectangle {
                width: 12; height: 12; radius: 6
                color: backend.online ? "#34c759" : "#ff453a"
            }
            ColumnLayout {
                spacing: 0
                Label {
                    text: backend.hostName
                    font.pixelSize: 18
                    font.bold: true
                }
                Label {
                    text: backend.online ? qsTr("Server running") : qsTr("Server not running")
                    color: backend.online ? "#9aa0a6" : "#ff453a"
                    font.pixelSize: 12
                }
            }
            Item { Layout.fillWidth: true }
            ToolButton { text: "?"; onClicked: help.open(); ToolTip.visible: hovered; ToolTip.text: qsTr("Shortcuts") }
        }

        // Says what is wrong, never works around it silently.
        Card {
            visible: backend.error !== ""
            color: "#3a1d1d"
            border.color: "#ff453a"
            Label {
                Layout.fillWidth: true
                text: backend.error
                wrapMode: Text.Wrap
                color: "#ffb4ab"
            }
            Label {
                visible: !backend.online
                Layout.fillWidth: true
                wrapMode: Text.Wrap
                color: "#9aa0a6"
                font.pixelSize: 12
                text: qsTr("Start it with: systemctl --user start clipboard-share")
            }
        }

        Flickable {
            Layout.fillWidth: true
            Layout.fillHeight: true
            contentHeight: content.implicitHeight
            clip: true
            boundsBehavior: Flickable.StopAtBounds
            ScrollBar.vertical: ScrollBar {}

            ColumnLayout {
                id: content
                width: parent.width
                spacing: 8

                Section { text: qsTr("PAIRING REQUESTS") }

                Label {
                    visible: backend.requests.length === 0
                    Layout.fillWidth: true
                    wrapMode: Text.Wrap
                    color: "#9aa0a6"
                    text: backend.online
                          ? qsTr("No requests. Open Clipboard Share on your phone and pick this computer.")
                          : ""
                }

                Repeater {
                    model: backend.requests
                    delegate: Card {
                        required property var modelData
                        Label { text: modelData.name; font.pixelSize: 16; font.bold: true; Layout.fillWidth: true; elide: Text.ElideRight }
                        Label {
                            text: modelData.ip + "  -  " + window.ago(modelData.requestedAt)
                            color: "#9aa0a6"
                            font.pixelSize: 12
                        }
                        RowLayout {
                            Layout.fillWidth: true
                            Button {
                                text: qsTr("Approve")
                                highlighted: true
                                Layout.fillWidth: true
                                onClicked: backend.approve(modelData.requestId)
                            }
                            Button {
                                text: qsTr("Deny")
                                flat: true
                                Layout.fillWidth: true
                                onClicked: backend.deny(modelData.requestId)
                            }
                        }
                    }
                }

                Section { text: qsTr("PAIRED DEVICES") }

                Label {
                    visible: backend.devices.length === 0
                    Layout.fillWidth: true
                    wrapMode: Text.Wrap
                    color: "#9aa0a6"
                    text: backend.online ? qsTr("No paired devices yet.") : ""
                }

                Repeater {
                    model: backend.devices
                    delegate: Card {
                        required property var modelData
                        RowLayout {
                            Layout.fillWidth: true
                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: 2
                                Label { text: modelData.name; font.pixelSize: 15; font.bold: true; Layout.fillWidth: true; elide: Text.ElideRight }
                                Label {
                                    text: qsTr("%1  -  paired %2").arg(modelData.ip).arg(window.day(modelData.firstApproved))
                                    color: "#9aa0a6"
                                    font.pixelSize: 12
                                }
                            }
                            Button {
                                text: qsTr("Revoke")
                                flat: true
                                onClicked: {
                                    window.revokeId = modelData.id
                                    window.revokeName = modelData.name
                                    revokeDialog.open()
                                }
                            }
                        }
                    }
                }
            }
        }

        Label {
            visible: backend.message !== ""
            Layout.fillWidth: true
            text: backend.message
            color: theme.accent
            font.pixelSize: 12
        }
    }

    // Revoking cuts a phone off, so ask first.
    Dialog {
        id: revokeDialog
        anchors.centerIn: parent
        modal: true
        title: qsTr("Revoke device?")
        standardButtons: Dialog.Cancel | Dialog.Ok
        // An explicit width: a wrapping label sized from its parent loops with the
        // dialog's own implicit width.
        width: Math.min(360, window.width - 32)
        contentItem: Label {
            wrapMode: Text.Wrap
            text: qsTr("%1 will lose access and has to ask for approval again.").arg(window.revokeName)
        }
        onAccepted: backend.revoke(window.revokeId)
    }

    Dialog {
        id: help
        anchors.centerIn: parent
        modal: true
        title: qsTr("Shortcuts")
        standardButtons: Dialog.Close
        GridLayout {
            columns: 2
            columnSpacing: 20
            Label { text: "Esc"; font.bold: true }
            Label { text: qsTr("Hide to the tray") }
            Label { text: "Q"; font.bold: true }
            Label { text: qsTr("Quit") }
            Label { text: "?"; font.bold: true }
            Label { text: qsTr("This help") }
        }
    }
}
