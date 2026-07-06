import QtQuick 2.0
import calamares.slideshow 1.0

Presentation {
    id: presentation

    Timer {
        interval: 15000
        running: true
        repeat: true
        onTriggered: presentation.goToNextSlide()
    }

    Slide {
        Image {
            id: logo1
            source: "veilbox-logo.png"
            width: 128
            height: 128
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.top
            anchors.topMargin: 20
        }

        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: logo1.bottom
            anchors.topMargin: 20
            text: "Veilbox Linux"
            font.pointSize: 24
            color: "#00d4aa"
        }

        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.verticalCenter
            text: "Minimal. Container-native. DevOps-ready."
            font.pointSize: 14
            color: "#cccccc"
        }
    }

    Slide {
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.top
            anchors.topMargin: 40
            text: "Niri + Noctalia Desktop"
            font.pointSize: 20
            color: "#00d4aa"
        }

        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.verticalCenter
            text: "A minimal, tiling Wayland compositor with the Noctalia shell."
            font.pointSize: 14
            color: "#cccccc"
        }
    }

    Slide {
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.top
            anchors.topMargin: 40
            text: "Container Runtime & DevOps Tools"
            font.pointSize: 20
            color: "#00d4aa"
        }

        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.verticalCenter
            text: "Docker CE, kubectl, helm, terraform, ansible, and more.\nPre-installed and ready to use."
            font.pointSize: 14
            color: "#cccccc"
            horizontalAlignment: Text.AlignHCenter
        }
    }

    Slide {
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.top
            anchors.topMargin: 40
            text: "Thank you for choosing Veilbox!"
            font.pointSize: 20
            color: "#00d4aa"
        }

        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.verticalCenter
            text: "The installer will guide you through the setup.\nThis will only take a few minutes."
            font.pointSize: 14
            color: "#cccccc"
            horizontalAlignment: Text.AlignHCenter
        }
    }
}
