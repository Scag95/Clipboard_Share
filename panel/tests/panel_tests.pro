QT += core network testlib
CONFIG += c++17 testcase console
CONFIG -= app_bundle
TARGET = panel_tests
TEMPLATE = app
INCLUDEPATH += ../src
SOURCES += panel_tests.cpp ../src/backend.cpp
HEADERS += ../src/backend.h
# The single-instance tests drive the panel binary that bin/build makes.
DEFINES += PANEL_BINARY=\\\"$$clean_path($$OUT_PWD/../clipboard-share-panel)\\\"
