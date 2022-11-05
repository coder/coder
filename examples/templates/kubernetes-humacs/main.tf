terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.5.3"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.12.1"
    }
  }
}

variable "use_kubeconfig" {
  type        = bool
  sensitive   = true
  description = <<-EOF
  Use host kubeconfig? (true/false)

  Set this to false if the Coder host is itself running as a Pod on the same
  Kubernetes cluster as you are deploying workspaces to.

  Set this to true if the Coder host is running outside the Kubernetes cluster
  for workspaces.  A valid "~/.kube/config" must be present on the Coder host.
  EOF
}

variable "namespace" {
  type        = string
  sensitive   = true
  description = "The namespace to create workspaces in (must exist prior to creating workspaces)"
}

provider "kubernetes" {
  # Authenticate via ~/.kube/config or a Coder-specific ServiceAccount, depending on admin preferences
  config_path = var.use_kubeconfig == true ? "~/.kube/config" : null
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"
  env = {
    GIT_AUTHOR_NAME           = "${data.coder_workspace.me.owner}"
    GIT_COMMITTER_NAME        = "${data.coder_workspace.me.owner}"
    GIT_AUTHOR_EMAIL          = "${data.coder_workspace.me.owner_email}"
    GIT_COMMITTER_EMAIL       = "${data.coder_workspace.me.owner_email}"
    TMATE_SOCKET              = "/tmp/ii/default/target.iisocket"
    INIT_DEFAULT_REPOS_FOLDER = "/home/ii"
    #    INIT_DEFAULT_REPOS                   = "sharingio/coder"
    SHARINGIO_PAIR_NAME                  = "foo"
    SHARINGIO_PAIR_INSTANCE_SETUP_USER   = "${data.coder_workspace.me.owner}"
    SHARINGIO_PAIR_INSTANCE_SETUP_GUESTS = "hh"
    PAIR_ENVIRONMENT_DEBUG               = "true"
  }
  startup_script = <<EOT
    #!/bin/bash

    # home folder can be empty, so copying default bash settings
    if [ ! -f ~/.profile ]; then
      cp /etc/skel/.profile $HOME
    fi
    if [ ! -f ~/.bashrc ]; then
      cp /etc/skel/.bashrc $HOME
    fi

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh  | tee code-server-install.log
    code-server --auth none --port 13337 | tee code-server-install.log &
    cd $${HOME}
    git clone "https://github.com/$SHARINGIO_PAIR_USER/.doom.d" || \
      git clone https://github.com/humacs/.doom.d
    rm -f "$${HOME}"/.doom.d/*.el
    org-tangle "$${HOME}/.doom.d/ii.org"
    doom sync
    sudo mkdir -p /var/run/host/root
    sudo chown ii:ii /var/run/host/root/
    git clone --depth=1 https://github.com/sharingio/.sharing.io /tmp/var/run/host/root/.sharing.io
    git clone --depth=1 https://github.com/emacs-lsp/lsp-gitpod
    # git clone -b feature/pgtk --depth=1 https://github.com/emacs-mirror/emacs/
    git clone --depth=1 git://git.sv.gnu.org/emacs.git
    cd emacs
    sudo apt-get update && sudo apt-get install -y \
      autoconf texinfo ripgrep fasd libtool-bin libcurl4-gnutls-dev libgccjit0 libgccjit-11-dev autopoint bsd-mailx cpio dbus-x11 dconf-gsettings-backend dconf-service debhelper debugedit dh-autoreconf dh-strip-nondeterminism diffstat dwz ed gettext  gir1.2-atk-1.0 gir1.2-atspi-2.0 gir1.2-freedesktop gir1.2-gdkpixbuf-2.0 gir1.2-gtk-3.0 gir1.2-harfbuzz-0.0 gir1.2-pango-1.0 gir1.2-rsvg-2.0 icu-devtools  imagemagick imagemagick-6-common imagemagick-6.q16 intltool-debian libacl1-dev libaom3 libarchive-zip-perl libasound2-dev libatk-bridge2.0-0  libatk-bridge2.0-dev libatk1.0-dev libatspi2.0-0 libatspi2.0-dev libattr1-dev libblkid-dev libbrotli-dev libbz2-dev libcairo-gobject2  libcairo-script-interpreter2 libcairo2-dev libcolord2 libdatrie-dev libdav1d5 libdbus-1-dev libdconf1 libde265-0 libdebhelper-perl libdeflate-dev  libdjvulibre-dev libdjvulibre-text libdjvulibre21 libegl-dev libegl-mesa0 libegl1 libegl1-mesa-dev libepoxy-dev libepoxy0 libexif-dev libexif12 libffi-dev libfftw3-double3 libfile-stripnondeterminism-perl libfontconfig-dev libfontconfig1-dev libfreetype-dev libfreetype6-dev libfribidi-dev libgbm1  libgdk-pixbuf-2.0-dev libgdk-pixbuf2.0-bin libgif-dev libgl-dev libgles-dev libgles1 libgles2 libglib2.0-dev libglib2.0-dev-bin libglvnd-core-dev  libglvnd-dev libglx-dev libgpm-dev libgraphite2-dev libgtk-3-0 libgtk-3-common libgtk-3-dev libharfbuzz-dev libharfbuzz-gobject0 libharfbuzz-icu0  libheif1 libice-dev libicu-dev libilmbase-dev libilmbase25 libjbig-dev libjpeg-dev libjpeg-turbo8-dev libjpeg8-dev liblcms2-dev liblockfile-bin  liblockfile-dev liblockfile1 liblqr-1-0 liblqr-1-0-dev libltdl-dev liblzma-dev liblzo2-2 libm17n-0 libm17n-dev libmagick++-6-headers libmagick++-6.q16-8  libmagick++-6.q16-dev libmagickcore-6-arch-config libmagickcore-6-headers libmagickcore-6.q16-6 libmagickcore-6.q16-6-extra libmagickcore-6.q16-dev libmagickwand-6-headers libmagickwand-6.q16-6 libmagickwand-6.q16-dev libmount-dev libncurses-dev libncurses5-dev libnuma1 libopenexr-dev libopenexr25  libopengl-dev libopengl0 libopenjp2-7 libopenjp2-7-dev libotf-dev libotf1 libpango1.0-dev libpangoxft-1.0-0 libpcre16-3 libpcre2-16-0 libpcre2-32-0  libpcre2-dev libpcre2-posix3 libpcre3-dev libpcre32-3 libpcrecpp0v5 libpipeline1 libpixman-1-dev libpng-dev libpthread-stubs0-dev librsvg2-2 librsvg2-common librsvg2-dev libselinux1-dev libsepol-dev libsm-dev libsub-override-perl libsystemd-dev libthai-dev libtiff-dev libtiffxx5 libtool  libwayland-bin libwayland-cursor0 libwayland-dev libwayland-egl1 libwayland-server0 libwebpdemux2 libwebpmux3 libwmf-0.2-7 libwmf-dev libwmflite-0.2-7 libx11-dev libx265-199 libxau-dev libxaw7-dev libxcb-render0-dev libxcb-shm0-dev libxcb1-dev libxcomposite-dev libxcursor-dev libxdamage-dev libxdmcp-dev libxext-dev libxfixes-dev libxft-dev libxft2 libxi-dev libxinerama-dev libxkbcommon-dev libxml2-dev libxmu-dev libxmu-headers libxpm-dev libxrandr-dev libxrender-dev libxt-dev libxtst-dev m17n-db man-db pango1.0-tools pkg-config po-debconf quilt sharutils ssl-cert uuid-dev wayland-protocols x11proto-dev xaw3dg xaw3dg-dev xorg-sgml-doctools xtrans-dev xutils-dev libgtk-4-dev
    ./autogen.sh
    ./configure --with-pgtk --with-cairo --with-modules --with-native-compilation --with-json
    NATIVE_FULL_AOT=1 make -j8

    # /usr/local/bin/pair-init.sh
  EOT
}

# code-server
resource "coder_app" "code-server" {
  agent_id  = coder_agent.main.id
  name      = "code-server"
  icon      = "/icon/code.svg"
  url       = "http://localhost:13337?folder=/home/ii"
  subdomain = false
  share     = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 3
    threshold = 10
  }
}

resource "kubernetes_persistent_volume_claim" "home" {
  metadata {
    name      = "coder-${lower(data.coder_workspace.me.owner)}-${lower(data.coder_workspace.me.name)}-home"
    namespace = var.namespace
  }
  wait_until_bound = false
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        storage = "10Gi"
      }
    }
  }
}

resource "kubernetes_pod" "main" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "coder-${lower(data.coder_workspace.me.owner)}-${lower(data.coder_workspace.me.name)}"
    namespace = var.namespace
  }
  spec {
    security_context {
      run_as_user = "1000"
      fs_group    = "1000"
    }
    container {
      name  = "dev"
      image = "registry.gitlab.com/sharingio/environment/environment:2022.09.30.0909"
      # image   = "codercom/enterprise-base:ubuntu"
      command = ["sh", "-c", coder_agent.main.init_script]
      security_context {
        run_as_user = "1000"
      }
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.main.token
      }
      volume_mount {
        mount_path = "/home/ii"
        name       = "home"
        read_only  = false
      }
    }

    volume {
      name = "home"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.home.metadata.0.name
        read_only  = false
      }
    }
  }
}
