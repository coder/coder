FROM ubuntu:24.04 AS go

# Install Go and gcc from apt, then build gomobile/gobind for Android
# Go bindings.
ARG GOPATH="/tmp/"

RUN apt-get update && \
	apt-get install --yes golang gcc && \
	mkdir --parents "$GOPATH" && \
	go install golang.org/x/mobile/cmd/gomobile@latest && \
	go install golang.org/x/mobile/cmd/gobind@latest && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/*

FROM ubuntu:24.04

SHELL ["/bin/bash", "-c"]

# Install packages from apt repositories.
ARG DEBIAN_FRONTEND="noninteractive"

# Install core system packages needed for Android development.
RUN apt-get update && \
	apt-get install --yes \
	build-essential \
	ca-certificates \
	curl \
	git \
	gnupg \
	htop \
	jq \
	less \
	libusb-1.0-0 \
	openssh-server \
	screen \
	sudo \
	tmux \
	udev \
	unzip \
	usbutils \
	vim \
	wget \
	zip && \
	# Delete package cache to avoid consuming space in layer.
	apt-get clean && \
	rm -rf /var/lib/apt/lists/*

# Install JDK 17 for Android builds.
RUN apt-get update && \
	apt-get install --yes openjdk-17-jdk && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/*

ENV JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64

# Install Android SDK command-line tools.
ARG ANDROID_SDK_TOOLS_VERSION=14742923
ENV ANDROID_HOME=/opt/android-sdk
ENV ANDROID_SDK_ROOT=/opt/android-sdk

RUN mkdir -p ${ANDROID_HOME}/cmdline-tools && \
	curl --silent --show-error --location --fail \
	"https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_SDK_TOOLS_VERSION}_latest.zip" \
	-o /tmp/cmdline-tools.zip && \
	unzip -q /tmp/cmdline-tools.zip -d ${ANDROID_HOME}/cmdline-tools && \
	mv ${ANDROID_HOME}/cmdline-tools/cmdline-tools ${ANDROID_HOME}/cmdline-tools/latest && \
	rm /tmp/cmdline-tools.zip

ENV PATH=${PATH}:${ANDROID_HOME}/cmdline-tools/latest/bin:${ANDROID_HOME}/platform-tools

# Accept all Android SDK licenses and install SDK components.
# platform-tools: adb and fastboot
# build-tools: aapt, dx, zipalign for building APKs
# platforms: target API levels for compilation
# ndk: Native Development Kit for C/C++ code
# cmake: required by NDK for native builds
RUN yes | sdkmanager --licenses > /dev/null 2>&1 && \
	sdkmanager \
	"platform-tools" \
	"build-tools;35.0.0" \
	"platforms;android-35" \
	"platforms;android-34" \
	"ndk;27.2.12479018" \
	"cmake;3.22.1" \
	"cmdline-tools;latest"

ENV ANDROID_NDK_HOME=${ANDROID_HOME}/ndk/27.2.12479018

# Install Gradle 8.11 for project builds.
ARG GRADLE_VERSION=8.11
RUN curl --silent --show-error --location --fail \
	"https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip" \
	-o /tmp/gradle.zip && \
	unzip -q /tmp/gradle.zip -d /opt && \
	mv /opt/gradle-${GRADLE_VERSION} /opt/gradle && \
	rm /tmp/gradle.zip

ENV PATH=${PATH}:/opt/gradle/bin

# Install Go from apt and copy only the gomobile/gobind binaries
# built in the go stage.
RUN apt-get update && \
	apt-get install --yes golang && \
	apt-get clean && \
	rm -rf /var/lib/apt/lists/*

COPY --from=go /tmp/bin /usr/local/bin

ENV GOPATH=/home/coder/go
ENV PATH=${PATH}:${GOPATH}/bin

# See https://github.com/cli/cli/issues/6175#issuecomment-1235984381 for
# proof the apt repository is unreliable.
RUN GH_CLI_VERSION=$(curl -s "https://api.github.com/repos/cli/cli/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v([^"]+)".*/\1/') && \
	curl -L "https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_linux_amd64.deb" -o gh.deb && \
	dpkg -i gh.deb && \
	rm gh.deb

# Configure udev rules for common Android device vendors so that
# USB-connected devices are accessible without root.
RUN mkdir -p /etc/udev/rules.d && \
	echo '# Google' > /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="18d1", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# Samsung' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="04e8", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# HTC' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="0bb4", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# Huawei' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="12d1", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# LG' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="1004", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# Motorola' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="22b8", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# OnePlus' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="2a70", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# Sony' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="0fce", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	echo '# Xiaomi' >> /etc/udev/rules.d/51-android.rules && \
	echo 'SUBSYSTEM=="usb", ATTR{idVendor}=="2717", MODE="0666", GROUP="plugdev"' >> /etc/udev/rules.d/51-android.rules && \
	chmod a+r /etc/udev/rules.d/51-android.rules

# Add coder user and allow passwordless sudo.
RUN useradd coder \
	--create-home \
	--shell=/bin/bash \
	--uid=1000 \
	--user-group && \
	mkdir -p /etc/sudoers.d && \
	echo 'coder ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/nopasswd && \
	chmod 750 /etc/sudoers.d/ && \
	chmod 640 /etc/sudoers.d/nopasswd

# Configure ADB: write common vendor IDs and enable listening on all
# interfaces for network ADB connections.
RUN mkdir -p /home/coder/.android && \
	printf '0x18d1\n0x04e8\n0x0bb4\n0x12d1\n0x1004\n0x22b8\n0x2a70\n0x0fce\n0x2717\n' \
	> /home/coder/.android/adb_usb.ini && \
	echo 'adb_server_port=5037' > /home/coder/.android/adb_env.ini && \
	chown -R coder:coder /home/coder/.android

# Adjust OpenSSH config.
RUN echo "PermitUserEnvironment yes" >>/etc/ssh/sshd_config

# Ensure the android-sdk directory is accessible to the coder user.
RUN chmod -R a+r ${ANDROID_HOME} && \
	find ${ANDROID_HOME} -type d -exec chmod a+x {} +

USER coder

WORKDIR /home/coder
