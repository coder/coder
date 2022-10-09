resource "kubernetes_namespace" "work-namespace" {
  metadata {
    annotations = {
      name = "ii-annotation"
    }

    labels = {
      cert-manager-tls = "sync"
    }

    name = var.namespace
  }
}
resource "kubernetes_manifest" "cluster_vclusters_vcluster1" {
  manifest = {
    "apiVersion" = "cluster.x-k8s.io/v1beta1"
    "kind"       = "Cluster"
    "metadata" = {
      "name"      = "vcluster1"
      "namespace" = var.namespace
    }
    "spec" = {
      "controlPlaneRef" = {
        "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
        "kind"       = "VCluster"
        "name"       = "vcluster1"
      }
      "infrastructureRef" = {
        "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
        "kind"       = "VCluster"
        "name"       = "vcluster1"
      }
    }
  }
}

resource "kubernetes_manifest" "vcluster_vclusters_vcluster1" {
  provisioner "local-exec" {
    command = "kubectl wait --for=condition=Ready --timeout=30s -n ${var.namespace} cluster vcluster1"
  }
  provisioner "local-exec" {
    command = "kubectl get secrets -n ${var.namespace} vcluster1-kubeconfig -o jsonpath={.data.value} | base64 -d > kubeconfig"
  }
  manifest = {
    "apiVersion" = "infrastructure.cluster.x-k8s.io/v1alpha1"
    "kind"       = "VCluster"
    "metadata" = {
      "name"      = "vcluster1"
      "namespace" = var.namespace
    }
    "spec" = {
      "controlPlaneEndpoint" = {
        "host" = ""
        "port" = 0
      }
      "helmRelease" = {
        "chart" = {
          "name"    = null
          "repo"    = null
          "version" = null
        }
        "values" = <<-EOT
        service:
          type: NodePort
        syncer:
          extraArgs:
            - --tls-san=${var.tls-san}
        EOT
      }
      "kubernetesVersion" = var.k8s-version
    }
  }
}

# This is generated from the vcluster...
# Need to find a way for it to wait before running, so that the secret exists
data "kubernetes_resource" "kubeconfig" {
  api_version = "v1"
  kind        = "Secret"
  depends_on = [
    kubernetes_manifest.vcluster_vclusters_vcluster1
  ]
  metadata {
    name      = "vcluster-kubeconfig"
    namespace = var.namespace
  }
}

# We'll need to use the kubeconfig from above to provision the coder/pair environment
