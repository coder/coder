# Match the Ubuntu base and mise bootstrap used by the Coder dogfood image.
FROM ubuntu:26.04@sha256:5e275723f82c67e387ba9e3c24baa0abdcb268917f276a0561c97bef9450d0b4

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]
ARG DEBIAN_FRONTEND=noninteractive
ARG REVISION=e26c1934a5f96c5209e7b054419db5f6c5c12323
ARG MISE_VERSION=2026.5.12
RUN apt-get update && apt-get install -y --no-install-recommends \
        bash bats bison build-essential ca-certificates cmake curl file flex git \
        iproute2 jq less libgbm-dev libicu-dev libpq-dev libreadline-dev libssl-dev \
        libx11-xcb1 openssh-client openssl pkg-config postgresql-18 \
        postgresql-client-18 procps python3 python3-pip python3-venv ripgrep rsync \
        shellcheck strace unzip uuid-dev wget xz-utils zip zlib1g-dev zstd \
    && rm -rf /var/lib/apt/lists/* \
    && userdel -r ubuntu \
    && useradd --create-home --uid 1000 --user-group --shell /bin/bash workload \
    && install -d -o workload -g workload /work/coder

ENV HOME=/home/workload \
    MISE_DATA_DIR=/home/workload/.local/share/mise \
    MISE_TRUSTED_CONFIG_PATHS=/work/coder \
    GOPATH=/home/workload/go \
    GOCACHE=/home/workload/.cache/go-build \
    PLAYWRIGHT_BROWSERS_PATH=/home/workload/.cache/ms-playwright
ENV PATH="${MISE_DATA_DIR}/shims:${GOPATH}/bin:/usr/lib/postgresql/18/bin:${PATH}"
WORKDIR /work/coder
USER 1000:1000
# Fetch only public sources. No local checkout, credentials, or agent state enter
# the image. All tools and dependency caches belong to the workload user.
RUN git init . \
    && git remote add origin https://github.com/coder/coder.git \
    && git fetch --depth=1 origin "$REVISION" \
    && git checkout --detach FETCH_HEAD \
    && test "$(git rev-parse HEAD)" = "$REVISION"
USER root
RUN case "$(uname -m)" in x86_64) target=linux-x64 ;; aarch64) target=linux-arm64 ;; *) exit 1 ;; esac \
    && checksum=$(scripts/mise_checksum.sh .github/actions/setup-mise/checksums.toml "$MISE_VERSION" "$target") \
    && test -n "$checksum" \
    && curl --fail --silent --show-error --location \
        "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-${target}" \
        --output /usr/local/bin/mise \
    && echo "$checksum  /usr/local/bin/mise" | sha256sum -c \
    && chmod 755 /usr/local/bin/mise
USER 1000:1000
# Bound compiler parallelism during preparation without constraining later runs.
RUN GOMAXPROCS=4 GOFLAGS=-p=4 mise install --locked --jobs=4 \
    && mise reshim
RUN git ls-files -z 'go.mod' '**/go.mod' | while IFS= read -r -d '' module; do \
        (cd "$(dirname "$module")" && go mod download all); \
    done
RUN pnpm install --frozen-lockfile \
    && pnpm --dir site install --frozen-lockfile
# The repository's browser tests use Chromium. Install its native libraries as
# root, then download the pinned Playwright browser as the unprivileged user.
USER root
RUN pnpm --dir site exec playwright install-deps chromium \
    && rm -rf /var/lib/apt/lists/*
USER 1000:1000
RUN pnpm --dir site exec playwright install chromium
# Warm representative unit-test compilation, not the Docker/external API suites.
RUN GOPROXY=off GOTOOLCHAIN=local GOMAXPROCS=4 go test -p=4 ./coderd/util/ptr ./coderd/util/slice \
    && pnpm --dir site test src/utils/fileSize.test.ts --maxWorkers=2
# Include command-line clients, but not a Docker daemon, socket, or credentials.
USER root
RUN apt-get update && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/* \
    && case "$(uname -m)" in \
        x86_64) arch=x86_64; checksum=4f798b3ee1e0140eab5bf30b0edc4e84f4cdb53255a429dc3bbae9524845d640 ;; \
        aarch64) arch=aarch64; checksum=e6b53725a73763ab3f988c73f8772eaed429754c1a579db5ff11f21990fd1817 ;; \
        *) exit 1 ;; \
    esac \
    && curl --fail --silent --show-error --location \
        "https://download.docker.com/linux/static/stable/${arch}/docker-27.5.1.tgz" \
        --output /tmp/docker-cli.tgz \
    && echo "$checksum  /tmp/docker-cli.tgz" | sha256sum -c \
    && tar -xzf /tmp/docker-cli.tgz --directory /usr/local/bin --strip-components=1 docker/docker \
    && rm /tmp/docker-cli.tgz
USER 1000:1000
# These tools have independent lockfiles, not packages in the root workspace.
RUN (cd scripts/apidocgen && ../pnpm_install.sh --frozen-lockfile) \
    && (cd offlinedocs && ../scripts/pnpm_install.sh --frozen-lockfile) \
    && touch node_modules/.installed site/node_modules/.installed
USER root
RUN apt-get update && apt-get install -y --no-install-recommends nyancat \
    && rm -rf /var/lib/apt/lists/*
RUN case "$(uname -m)" in \
        x86_64) platform=linux-x64; checksum=708c77b204db30086ff6f1ed34577bda03e1be74ced80557cbf7b54249ebf0bd ;; \
        aarch64) platform=linux-arm64; checksum=53a0d7e219d7b0804ecba5dbc002745e89f786cc946d2de85a6470d5b2d1fb06 ;; \
        *) exit 1 ;; \
    esac \
    && curl --fail --silent --show-error --location \
        "https://downloads.claude.ai/claude-code-releases/2.1.318/${platform}/claude" \
        --output /usr/local/bin/claude \
    && echo "$checksum  /usr/local/bin/claude" | sha256sum -c \
    && chmod 755 /usr/local/bin/claude
ENV DISABLE_AUTOUPDATER=1
USER 1000:1000
LABEL dev.coder.workload.repository="https://github.com/coder/coder.git"
LABEL dev.coder.workload.revision="${REVISION}"
CMD ["/bin/bash"]
