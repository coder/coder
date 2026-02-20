# nsjail Jail Type

nsjail is Agent Boundaries' default jail type that uses Linux namespaces to
provide process isolation. It creates unprivileged network namespaces to control
and monitor network access for processes running under Boundary.

**Running on Docker?** See [nsjail on Docker](./docker.md) for runtime
and permission requirements.

**Running on Kubernetes?** See [nsjail on Kubernetes](./k8s.md) for runtime
and permission requirements.

**Running on ECS?** See [nsjail on ECS](./ecs.md) for runtime and permission
requirements.

## Overview

nsjail leverages Linux namespace technology to isolate processes at the network
level. When Agent Boundaries runs with nsjail, it creates a separate network
namespace for the isolated process, allowing Agent Boundaries to intercept and
filter all network traffic according to the configured policy.

This jail type requires Linux capabilities to create and manage network
namespaces, which means it has specific runtime requirements when running in
containerized environments like Docker and Kubernetes.

## Architecture

<img width="1228" height="604" alt="Boundary" src="https://github.com/user-attachments/assets/1b7c8c5b-7b8f-4adf-8795-325bd28715c6" />
