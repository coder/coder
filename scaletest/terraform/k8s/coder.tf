data "google_client_config" "default" {}

locals {
  coder_helm_repo    = "https://helm.coder.com/v2"
  coder_helm_chart   = "coder"
  coder_release_name = var.name
  coder_namespace    = "coder-${var.name}"
  coder_admin_email  = "admin@coder.com"
  coder_admin_user   = "coder"
  coder_access_url   = "http://${var.coder_address}"
}

resource "null_resource" "coder_namespace" {
  triggers = {
    namespace       = local.coder_namespace
    kubeconfig_path = var.kubernetes_kubeconfig_path
  }
  provisioner "local-exec" {
    when    = create
    command = <<EOF
      KUBECONFIG=${self.triggers.kubeconfig_path} kubectl create namespace ${self.triggers.namespace}
    EOF
  }
  provisioner "local-exec" {
    when    = destroy
    command = "true"
  }
}

resource "kubernetes_secret" "coder-db" {
  type = "Opaque"
  metadata {
    name      = "coder-db-url"
    namespace = local.coder_namespace
  }
  depends_on = [null_resource.coder_namespace]
  data = {
    url = var.coder_db_url
  }
}

resource "helm_release" "coder-chart" {
  repository = local.coder_helm_repo
  chart      = local.coder_helm_chart
  name       = local.coder_release_name
  version    = var.coder_chart_version
  namespace  = local.coder_namespace
  depends_on = [
    null_resource.coder_namespace
  ]
  values = [<<EOF
coder:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: "cloud.google.com/gke-nodepool"
            operator: "In"
            values: ["${var.kubernetes_nodepool_coder}"]
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 1
        podAffinityTerm:
          topologyKey: "kubernetes.io/hostname"
          labelSelector:
            matchExpressions:
            - key:      "app.kubernetes.io/instance"
              operator: "In"
              values:   ["${local.coder_release_name}"]
  env:
    - name: "CODER_ACCESS_URL"
      value: "${local.coder_access_url}"
    - name: "CODER_CACHE_DIRECTORY"
      value: "/tmp/coder"
    - name: "CODER_ENABLE_TELEMETRY"
      value: "false"
    - name: "CODER_LOGGING_HUMAN"
      value: "/dev/null"
    - name: "CODER_LOGGING_STACKDRIVER"
      value: "/dev/stderr"
    - name: "CODER_PG_CONNECTION_URL"
      valueFrom:
        secretKeyRef:
          name: "${kubernetes_secret.coder-db.metadata.0.name}"
          key: url
    - name: "CODER_PPROF_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_ENABLE"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_AGENT_STATS"
      value: "true"
    - name: "CODER_PROMETHEUS_COLLECT_DB_METRICS"
      value: "true"
    - name: "CODER_VERBOSE"
      value: "true"
    - name: "CODER_EXPERIMENTS"
      value: "${var.coder_experiments}"
    - name: "CODER_DANGEROUS_DISABLE_RATE_LIMITS"
      value: "true"
    # Disabling built-in provisioner daemons
    - name: "CODER_PROVISIONER_DAEMONS"
      value: "0"
  image:
    repo: ${var.coder_image_repo}
    tag: ${var.coder_image_tag}
  replicaCount: "${var.coder_replicas}"
  resources:
    requests:
      cpu: "${var.coder_cpu_request}"
      memory: "${var.coder_mem_request}"
    limits:
      cpu: "${var.coder_cpu_limit}"
      memory: "${var.coder_mem_limit}"
  securityContext:
    readOnlyRootFilesystem: true
  service:
    enable: true
    sessionAffinity: None
    loadBalancerIP: "${var.coder_address}"
  volumeMounts:
  - mountPath: "/tmp"
    name: cache
    readOnly: false
  volumes:
  - emptyDir:
      sizeLimit: 1024Mi
    name: cache
EOF
  ]
}

resource "local_file" "kubernetes_template" {
  filename = "${path.module}/../.coderv2/templates/kubernetes/main.tf"
  content  = <<EOF
    terraform {
      required_providers {
        coder = {
          source  = "coder/coder"
          version = "~> 0.7.0"
        }
        kubernetes = {
          source  = "hashicorp/kubernetes"
          version = "~> 2.18"
        }
      }
    }

    provider "coder" {}

    provider "kubernetes" {
      config_path = null # always use host
    }

    data "coder_workspace" "me" {}

    resource "coder_agent" "main" {
      os                     = "linux"
      arch                   = "amd64"
      startup_script_timeout = 180
      startup_script         = ""
    }

    resource "kubernetes_pod" "main" {
      count = data.coder_workspace.me.start_count
      metadata {
        name      = "coder-$${lower(data.coder_workspace.me.owner)}-$${lower(data.coder_workspace.me.name)}"
        namespace = "${local.coder_namespace}"
        labels = {
          "app.kubernetes.io/name"     = "coder-workspace"
          "app.kubernetes.io/instance" = "coder-workspace-$${lower(data.coder_workspace.me.owner)}-$${lower(data.coder_workspace.me.name)}"
        }
      }
      spec {
        security_context {
          run_as_user = "1000"
          fs_group    = "1000"
        }
        container {
          name              = "dev"
          image             = "${var.workspace_image}"
          image_pull_policy = "Always"
          command           = ["sh", "-c", coder_agent.main.init_script]
          security_context {
            run_as_user = "1000"
          }
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.main.token
          }
          resources {
            requests = {
              "cpu"    = "${var.workspace_cpu_request}"
              "memory" = "${var.workspace_mem_request}"
            }
            limits = {
              "cpu"    = "${var.workspace_cpu_limit}"
              "memory" = "${var.workspace_mem_limit}"
            }
          }
        }

        affinity {
          node_affinity {
            required_during_scheduling_ignored_during_execution {
              node_selector_term {
                match_expressions {
                  key = "cloud.google.com/gke-nodepool"
                  operator = "In"
                  values = ["${var.kubernetes_nodepool_workspaces}"]
                }
              }
            }
          }
        }
      }
    }
  EOF
}

# TODO(cian): Remove this when we have support in the Helm chart.
# Ref: https://github.com/coder/coder/issues/8243
resource "local_file" "provisionerd_deployment" {
  filename = "${path.module}/../.coderv2/provisionerd-deployment.yaml"
  content  = <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/instance: ${var.name}
    app.kubernetes.io/name: provisionerd
  name: provisionerd
  namespace: ${local.coder_namespace}
spec:
  replicas: ${var.provisionerd_replicas}
  selector:
    matchLabels:
      app.kubernetes.io/instance: ${var.name}
      app.kubernetes.io/name: provisionerd
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      creationTimestamp: null
      labels:
        app.kubernetes.io/instance: ${var.name}
        app.kubernetes.io/name: provisionerd
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: cloud.google.com/gke-nodepool
                operator: In
                values:
                - ${var.kubernetes_nodepool_coder}
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app.kubernetes.io/instance
                  operator: In
                  values:
                  - ${var.name}
              topologyKey: kubernetes.io/hostname
            weight: 1
      containers:
      - args:
        - server
        command:
        - /opt/coder
        env:
        - name: CODER_HTTP_ADDRESS
          value: 0.0.0.0:8080
        - name: CODER_PROMETHEUS_ADDRESS
          value: 0.0.0.0:2112
        - name: CODER_ACCESS_URL
          value: ${local.coder_access_url}
        - name: CODER_CACHE_DIRECTORY
          value: /tmp/coder
        - name: CODER_ENABLE_TELEMETRY
          value: "false"
        - name: CODER_LOGGING_HUMAN
          value: /dev/null
        - name: CODER_LOGGING_STACKDRIVER
          value: /dev/stderr
        - name: CODER_PG_CONNECTION_URL
          valueFrom:
            secretKeyRef:
              key: url
              name: coder-db-url
        - name: CODER_PPROF_ENABLE
          value: "true"
        - name: CODER_PROMETHEUS_ENABLE
          value: "true"
        - name: CODER_PROMETHEUS_COLLECT_AGENT_STATS
          value: "true"
        - name: CODER_PROMETHEUS_COLLECT_DB_METRICS
          value: "true"
        - name: CODER_VERBOSE
          value: "true"
        - name: CODER_PROVISIONER_DAEMONS
          value: "${var.provisionerd_concurrency}"
        image: "${var.coder_image_repo}:${var.coder_image_tag}"
        imagePullPolicy: IfNotPresent
        lifecycle: {}
        livenessProbe:
          failureThreshold: 3
          httpGet:
            path: /api/v2/buildinfo
            port: http
            scheme: HTTP
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        name: provisionerd
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        - containerPort: 2112
          name: prometheus-http
          protocol: TCP
        readinessProbe:
          failureThreshold: 3
          httpGet:
            path: /api/v2/buildinfo
            port: http
            scheme: HTTP
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        resources:
          limits:
            cpu: "${var.provisionerd_cpu_limit}"
            memory: "${var.provisionerd_mem_limit}"
          requests:
            cpu: "${var.provisionerd_cpu_request}"
            memory: "${var.provisionerd_mem_request}"
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsGroup: 1000
          runAsNonRoot: true
          runAsUser: 1000
          seccompProfile:
            type: RuntimeDefault
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /tmp
          name: cache
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      serviceAccount: coder
      serviceAccountName: coder
      terminationGracePeriodSeconds: 60
      volumes:
      - emptyDir:
          sizeLimit: 10Gi
        name: cache
    EOF
}

resource "null_resource" "provisionerd_deployment_apply" {
  depends_on = [helm_release.coder-chart, local_file.provisionerd_deployment]
  triggers = {
    kubeconfig_path = var.kubernetes_kubeconfig_path
    manifest_path   = local_file.provisionerd_deployment.filename
  }
  provisioner "local-exec" {
    command = <<EOF
      KUBECONFIG=${self.triggers.kubeconfig_path} kubectl apply -f ${self.triggers.manifest_path}
    EOF
  }
}

resource "local_file" "output_vars" {
  filename = "${path.module}/../../.coderv2/url"
  content  = local.coder_access_url
}

output "coder_url" {
  description = "URL of the Coder deployment"
  value       = local.coder_access_url
}
