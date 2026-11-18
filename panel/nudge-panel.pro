QT += core gui qml quick quickcontrols2 network

CONFIG += c++17 release
TARGET = nudge-panel
TEMPLATE = app

HEADERS += \
    src/autostart.h \
    src/backend.h \
    src/paths.h \
    src/theme.h

SOURCES += \
    src/main.cpp \
    src/autostart.cpp \
    src/backend.cpp \
    src/paths.cpp \
    src/theme.cpp

RESOURCES += src/resources.qrc
