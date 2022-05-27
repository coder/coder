# From the base image (built on Docker host)
FROM coder-base:v0.1

# Install everything as root
USER root

# Install JDK (OpenJDK 8)
RUN DEBIAN_FRONTEND="noninteractive" apt-get update -y && \
    apt-get install -y openjdk-11-jdk
ENV JAVA_HOME /usr/lib/jvm/java-11-openjdk-amd64
ENV PATH $PATH:$JAVA_HOME/bin

# Install Maven
ARG MAVEN_VERSION=3.6.3
ARG MAVEN_SHA512=c35a1803a6e70a126e80b2b3ae33eed961f83ed74d18fcd16909b2d44d7dada3203f1ffe726c17ef8dcca2dcaa9fca676987befeadc9b9f759967a8cb77181c0

ENV MAVEN_HOME /usr/share/maven
ENV MAVEN_CONFIG "/home/coder/.m2"

RUN mkdir -p $MAVEN_HOME $MAVEN_HOME/ref \
    && echo "Downloading Maven" \
    && curl -fsSL -o /tmp/apache-maven.tar.gz https://apache.osuosl.org/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz \
    \
    && echo "Checking downloaded file hash" \
    && echo "${MAVEN_SHA512}  /tmp/apache-maven.tar.gz" | sha512sum -c - \
    \
    && echo "Unzipping Maven" \
    && tar -xzf /tmp/apache-maven.tar.gz -C $MAVEN_HOME --strip-components=1 \
    \
    && echo "Cleaning and setting links" \
    && rm -f /tmp/apache-maven.tar.gz \
    && ln -s $MAVEN_HOME/bin/mvn /usr/bin/mvn

# Install Gradle
ENV GRADLE_VERSION=6.7
ARG GRADLE_SHA512=d495bc65379d2a854d2cca843bd2eeb94f381e5a7dcae89e6ceb6ef4c5835524932313e7f30d7a875d5330add37a5fe23447dc3b55b4d95dffffa870c0b24493

ENV GRADLE_HOME /usr/bin/gradle

RUN mkdir -p /usr/share/gradle /usr/share/gradle/ref \
    && echo "Downloading Gradle" \
    && curl -fsSL -o /tmp/gradle.zip https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip \
    \
    && echo "Checking downloaded file hash" \
    && echo "${GRADLE_SHA512}  /tmp/gradle.zip" | sha512sum -c - \
    \
    && echo "Unziping gradle" \
    && unzip -d /usr/share/gradle /tmp/gradle.zip \
    \
    && echo "Cleaning and setting links" \
    && rm -f /tmp/gradle.zip \
    && ln -s /usr/share/gradle/gradle-${GRADLE_VERSION} /usr/bin/gradle

ENV PATH $PATH:$GRADLE_HOME/bin

# Set back to coder user
USER coder
