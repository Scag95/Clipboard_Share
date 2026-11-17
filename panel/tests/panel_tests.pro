QT += core network testlib
CONFIG += c++17 testcase console
CONFIG -= app_bundle
TARGET = panel_tests
TEMPLATE = app
INCLUDEPATH += ../src
SOURCES += panel_tests.cpp ../src/backend.cpp
HEADERS += ../src/backend.h
