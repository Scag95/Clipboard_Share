QT += core network testlib
CONFIG += c++17 testcase console
CONFIG -= app_bundle
TARGET = panel_tests
TEMPLATE = app
INCLUDEPATH += ../src
SOURCES += panel_tests.cpp ../src/autostart.cpp ../src/backend.cpp ../src/paths.cpp
HEADERS += ../src/autostart.h ../src/backend.h ../src/paths.h
# The single-instance tests drive the panel binary that bin/build makes.
DEFINES += PANEL_BINARY=\\\"$$clean_path($$OUT_PWD/../nudge-panel)\\\"
# The panel's autostart override must match the packaged entry.
DEFINES += PACKAGED_AUTOSTART=\\\"$$clean_path($$PWD/../../pkgbuild/nudge-panel-autostart.desktop)\\\"
