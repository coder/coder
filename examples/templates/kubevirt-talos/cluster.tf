terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "0.4.15"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.12.1"
    }
  }
}

# https://www.terraform.io/language/providers/configuration#provider-configuration-1
# > You can use expressions in the values of these configuration arguments,
# but can only reference values that are known before the configuration is applied.
# This means you can safely reference input variables, but not attributes
# exported by resources (with an exception for resource arguments that
# are specified directly in the configuration).
#### no data.X :(
# provider "kubernetes" {
#   alias                  = "vcluster"
#   host                   = yamldecode(data.kubernetes_resource.kubeconfig.data)["value"]["clusters"][0]["cluster"]["server"]
#   client_certificate     = base64decode(yamldecode(data.kubernetes_resource.kubeconfig.data)["value"]["users"][0]["user"]["client-certificate-data"])
#   client_key             = base64decode(yamldecode(data.kubernetes_resource.kubeconfig.data)["value"]["users"][0]["user"]["client-key-data"])
#   cluster_ca_certificate = base64decode(yamldecode(data.kubernetes_resource.kubeconfig.data)["value"]["clusters"][0]["cluster"]["certificate-authority-data"])
# }

data "coder_workspace" "me" {}

resource "coder_agent" "main" {
  os             = "linux"
  arch           = "amd64"
  startup_script = <<EOT
    #!/bin/bash

    # home folder can be empty, so copying default bash settings
    if [ ! -f ~/.profile ]; then
      cp /etc/skel/.profile $HOME
    fi
    if [ ! -f ~/.bashrc ]; then
      cp /etc/skel/.bashrc $HOME
    fi
    echo 'export PATH="$PATH:$HOME/bin"' >> $HOME/.bashrc
    mkdir -p bin
    curl -o bin/kubectl -L https://dl.k8s.io/v1.25.2/bin/linux/amd64/kubectl
    chmod +x bin/*

    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh  | tee code-server-install.log
    code-server --auth none --port 13337 | tee code-server-install.log &
  EOT
}

# code-server
resource "coder_app" "code-server" {
  agent_id      = coder_agent.main.id
  name          = "code-server"
  icon          = "/icon/code.svg"
  url           = "http://localhost:13337?folder=/home/coder"
  relative_path = true

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 3
    threshold = 10
  }
}

resource "kubernetes_namespace" "workspace" {
  metadata {
    name = data.coder_workspace.me.name
    labels = {
      cert-manager-tls = "sync"
    }
  }
}

resource "kubernetes_manifest" "cluster" {
  manifest = {
    "apiVersion" = "cluster.x-k8s.io/v1beta1"
    "kind"       = "Cluster"
    "metadata" = {
      "name"      = data.coder_workspace.me.name
      "namespace" = data.coder_workspace.me.name
      "labels" = {
        "cluster-name" = data.coder_workspace.me.name
      }
    }
    "spec" = {
      "controlPlaneRef" = {
        "apiVersion" = "controlplane.cluster.x-k8s.io/v1beta1"
        "kind"       = "TalosControlPlane"
        "name"       = data.coder_workspace.me.name
        "namespace"  = data.coder_workspace.me.name
      }
      "infrastructureRef" = {
        "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
        "kind"       = "KubevirtCluster"
        "name"       = data.coder_workspace.me.name
        "namespace"  = data.coder_workspace.me.name
      }
      "clusterNetwork" = {
        "pods" = {
          "cidrBlocks" = [
            "192.168.0.0/16",
          ]
        }
        "services" = {
          "cidrBlocks" = [
            "172.26.0.0/16",
          ]
        }
      }
    }
  }
}

resource "kubernetes_manifest" "kvcluster" {
  manifest = {
    "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
    "kind"       = "KubevirtCluster"
    "metadata" = {
      "name"      = data.coder_workspace.me.name
      "namespace" = data.coder_workspace.me.name
    }
    "spec" = {
      "controlPlaneServiceTemplate" = {
        "spec" = {
          "type" = "ClusterIP"
        }
      }
    }
  }
}

resource "kubernetes_manifest" "kubevirtmachinetemplate_control_plane" {
  manifest = {
    "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
    "kind"       = "KubevirtMachineTemplate"
    "metadata" = {
      "name"      = "${data.coder_workspace.me.name}-cp"
      "namespace" = data.coder_workspace.me.name
    }
    "spec" = {
      "template" = {
        "spec" = {
          "virtualMachineTemplate" = {
            "metadata" = {
              "namespace" = data.coder_workspace.me.name
            }
            "spec" = {
              "runStrategy" = "Always"
              "dataVolumeTemplates" = [
                {
                  "metadata" = {
                    "name" = "vmdisk-dv"
                  }
                  "spec" = {
                    "pvc" = {
                      "accessModes" = ["ReadWriteOnce"]
                      "resources" = {
                        "requests" = {
                          "storage" = "50Gi"
                        }
                      }
                    }
                    "source" = {
                      "registry" = {
                        "url" = "docker://docker.io/katamo/talos:latest"
                      }
                    }
                  }
                },
              ]
              "template" = {
                "spec" = {
                  "domain" = {
                    "cpu" = {
                      "cores" = 2
                    }
                    "devices" = {
                      "disks" = [
                        {
                          "disk" = {
                            "bus" = "scsi"
                          }
                          "name" = "vmdisk"
                        },
                      ]
                    }
                    "memory" = {
                      "guest" = "4Gi"
                    }
                  }
                  "evictionStrategy" = "External"
                  "volumes" = [
                    {
                      "dataVolume" = {
                        "name" = "vmdisk-dv"
                      }
                      "name" = "vmdisk"
                    },
                  ]
                }
              }
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_manifest" "taloscontrolplane_talos_em_control_plane" {
  manifest = {
    "apiVersion" = "controlplane.cluster.x-k8s.io/v1alpha3"
    "kind"       = "TalosControlPlane"
    "metadata" = {
      "name"      = data.coder_workspace.me.name
      "namespace" = data.coder_workspace.me.name
    }
    "spec" = {
      "controlPlaneConfig" = {
        "controlplane" = {
          "generateType" = "controlplane"
          "configPatches" = [
            {
              "op"    = "add"
              "path"  = "/debug"
              "value" = true
            },
            # {
            #   "op"   = "replace"
            #   "path" = "/machine/install"
            #   "value" = {
            #     "bootloader"      = true
            #     "wipe"            = false
            #     "disk"            = "/dev/sda"
            #     "image"           = "ghcr.io/siderolabs/installer:v1.2.5"
            #     "extraKernelArgs" = ["console=ttyS0"]
            #   }
            # },
            # {
            #   "op"   = "add"
            #   "path" = "/machine/kubelet/extraArgs"
            #   "value" = {
            #     "cloud-provider" = "external"
            #   }
            # },
            # {
            #   "op"   = "add"
            #   "path" = "/cluster/apiServer/extraArgs"
            #   "value" = {
            #     "cloud-provider" = "external"
            #   }
            # },
            # {
            #   "op"   = "add"
            #   "path" = "/cluster/controllerManager/extraArgs"
            #   "value" = {
            #     "cloud-provider" = "external"
            #   }
            # },
            {
              "op"    = "add"
              "path"  = "/cluster/allowSchedulingOnControlPlanes"
              "value" = true
            }
          ]
        }
        "init" = {
          "configPatches" = [
            {
              "op"   = "replace"
              "path" = "/machine/install"
              "value" = {
                "bootloader"      = true
                "wipe"            = false
                "disk"            = "/dev/sda"
                "image"           = "ghcr.io/siderolabs/installer:v1.2.5"
                "extraKernelArgs" = ["console=ttyS0"]
              }
            },
            {
              "op"    = "add"
              "path"  = "/debug"
              "value" = true
            },
            # {
            #   "op"   = "add"
            #   "path" = "/machine/kubelet/extraArgs"
            #   "value" = {
            #     "cloud-provider" = "external"
            #   }
            # },
            # {
            #   "op"   = "add"
            #   "path" = "/cluster/apiServer/extraArgs"
            #   "value" = {
            #     "cloud-provider" = "external"
            #   }
            # },
            # {
            #   "op"   = "add"
            #   "path" = "/cluster/controllerManager/extraArgs"
            #   "value" = {
            #     "cloud-provider" = "external"
            #   }
            # },
            {
              "op"    = "add"
              "path"  = "/cluster/allowSchedulingOnControlPlanes"
              "value" = true
            },
          ]
          "generateType" = "init"
        }
      }
      "infrastructureTemplate" = {
        "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
        "kind"       = "KubevirtMachineTemplate"
        "name"       = "${data.coder_workspace.me.name}-cp"
      }
      "replicas" = 1
      "version"  = "v1.25.2"
    }
  }
}

resource "kubernetes_manifest" "kubevirtmachinetemplate_md_0" {
  manifest = {
    "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
    "kind"       = "KubevirtMachineTemplate"
    "metadata" = {
      "name"      = data.coder_workspace.me.name
      "namespace" = data.coder_workspace.me.name
    }
    "spec" = {
      "template" = {
        "spec" = {
          "virtualMachineTemplate" = {
            "metadata" = {
              "namespace" = data.coder_workspace.me.name
            }
            "spec" = {
              "runStrategy" = "Always"
              "dataVolumeTemplates" = [
                {
                  "metadata" = {
                    "name" = "vmdisk-dv"
                  }
                  "spec" = {
                    "pvc" = {
                      "accessModes" = [
                        "ReadWriteOnce"
                      ]
                      "resources" = {
                        "requests" = {
                          "storage" = "50Gi"
                        }
                      }
                    }
                    "source" = {
                      "registry" = {
                        "url" = "docker://docker.io/katamo/talos:latest"
                      }
                    }
                  }
                },
              ]
              "template" = {
                "spec" = {
                  "domain" = {
                    # "firmware" = {
                    #   "kernelBoot" = {
                    #     "container" = {
                    #       "image"           = "ghcr.io/siderolabs/installer:v1.2.5"
                    #       "initrdPath"      = "/usr/install/amd64/initramfs.xz"
                    #       "kernelPath"      = "/usr/install/amd64/vmlinuz"
                    #       "imagePullPolicy" = "Always"
                    #       "imagePullSecret" = "IfNotPresent"
                    #     }
                    #     "kernelArgs" = "console=ttyS0"
                    #   }
                    # }
                    "cpu" = {
                      "cores" = 2
                    }
                    "devices" = {
                      "disks" = [
                        {
                          "disk" = {
                            "bus" = "virtio"
                          }
                          "name" = "vmdisk"
                        },
                      ]
                    }
                    "memory" = {
                      "guest" = "4Gi"
                    }
                  }
                  "evictionStrategy" = "External"
                  "volumes" = [
                    {
                      "dataVolume" = {
                        "name" = "vmdisk-dv"
                      }
                      "name" = "vmdisk"
                    },
                  ]
                }
              }
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_manifest" "talosconfigtemplate_talos_em_worker_a" {
  manifest = {
    "apiVersion" = "bootstrap.cluster.x-k8s.io/v1alpha3"
    "kind"       = "TalosConfigTemplate"
    "metadata" = {
      "labels" = {
        "cluster.x-k8s.io/cluster-name" = data.coder_workspace.me.name
      }
      "name"      = data.coder_workspace.me.name
      "namespace" = data.coder_workspace.me.name
    }
    "spec" = {
      "template" = {
        "spec" = {
          "generateType" = "join"
          "talosVersion" = "v1.2.5"
        }
      }
    }
  }
}

resource "kubernetes_manifest" "machinedeployment_md_0" {
  manifest = {
    "apiVersion" = "cluster.x-k8s.io/v1beta1"
    "kind"       = "MachineDeployment"
    "metadata" = {
      "name"      = data.coder_workspace.me.name
      "namespace" = data.coder_workspace.me.name
    }
    "spec" = {
      "clusterName" = data.coder_workspace.me.name
      "replicas"    = 0
      "selector" = {
        "matchLabels" = null
      }
      "template" = {
        "spec" = {
          "bootstrap" = {
            "configRef" = {
              "apiVersion" = "bootstrap.cluster.x-k8s.io/v1beta1"
              "kind"       = "TalosConfigTemplate"
              "name"       = data.coder_workspace.me.name
              "namespace"  = data.coder_workspace.me.name
            }
          }
          "clusterName" = "kv1"
          "infrastructureRef" = {
            "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
            "kind"       = "KubevirtMachineTemplate"
            "name"       = data.coder_workspace.me.name
            "namespace"  = data.coder_workspace.me.name
          }
          "version" = "v1.23.5"
        }
      }
    }
  }
}

resource "kubernetes_manifest" "configmap_capi_init" {
  manifest = {
    "kind" = "ConfigMap"
    "metadata" = {
      "name"      = "capi-init"
      "namespace" = data.coder_workspace.me.name
    }
    "apiVersion" = "v1"
    "data" = {
      "cool.yaml" = templatefile("cool.template.yaml",
        {
          coder_command = jsonencode(["sh", "-c", coder_agent.main.init_script]),
          coder_token   = coder_agent.main.token
          instance_name = data.coder_workspace.me.name
      })
    }
  }
}

resource "kubernetes_manifest" "clusterresourceset_capi_init" {
  manifest = {
    "apiVersion" = "addons.cluster.x-k8s.io/v1beta1"
    "kind"       = "ClusterResourceSet"
    "metadata" = {
      "name"      = data.coder_workspace.me.name
      "namespace" = data.coder_workspace.me.name
    }
    "spec" = {
      "clusterSelector" = {
        "matchLabels" = {
          "cluster-name" = data.coder_workspace.me.name
        }
      }
      "resources" = [
        {
          "kind" = "ConfigMap"
          "name" = "capi-init"
        },
        # {
        #   "kind" = "Secret"
        #   "name" = "vcluster-kubeconfig"
        # },
      ]
      "strategy" = "ApplyOnce"
    }
  }
}
