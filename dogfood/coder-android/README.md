# Android Development Template

This Coder template provides a complete Android development environment for
building the [coder-mobile-android](https://github.com/coder/coder-mobile-android)
project.

## What's Included

| Tool            | Version | Purpose                            |
|-----------------|---------|------------------------------------|
| JDK             | 17      | Android build toolchain            |
| Android SDK     | 35      | Target platform APIs               |
| Android NDK     | r27     | Native C/C++ compilation           |
| Build Tools     | 35.0.0  | APK packaging (aapt, d8, zipalign) |
| Gradle          | 8.11    | Build system                       |
| Go              | apt     | gomobile / native Go code          |
| gomobile/gobind | latest  | Go-to-Android bindings             |
| ADB             | latest  | Device communication               |
| CMake           | 3.22.1  | NDK native builds                  |
| GitHub CLI      | latest  | Repository management              |

## ADB and Local Device Connections

The workspace is pre-configured with ADB and udev rules for common Android
device vendors (Google, Samsung, HTC, Huawei, LG, Motorola, OnePlus, Sony,
Xiaomi).

### Connecting a Device Over the Network

Since workspaces run in Docker containers, USB passthrough is not directly
available. Use ADB over TCP/IP instead:

1. On your **local machine**, connect the device via USB and run:

   ```sh
   adb tcpip 5555
   ```

2. Find the device's IP address:

   ```sh
   adb shell ip route | awk '{print $9}'
   ```

3. Inside the **Coder workspace**, connect to the device:

   ```sh
   adb connect <device-ip>:5555
   ```

### Port Forwarding with Coder

You can also forward the local ADB port into the workspace using the Coder
CLI:

```sh
# On your local machine (where the device is plugged in):
coder port-forward <workspace-name> --tcp 5037:5037
```

This forwards the local ADB server port into the workspace so that `adb
devices` inside the workspace sees your locally connected devices.

## Building coder-mobile-android

```sh
cd ~/coder-mobile-android
./gradlew assembleDebug
```

## Building Go Libraries for Android

```sh
gomobile bind -target=android -androidapi 28 ./path/to/gopackage
```
