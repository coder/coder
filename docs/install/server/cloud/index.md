---
title: Cloud platforms
---

We provide install guides and example templates for deploying Coder to your cloud of choice.

If you already run a Kubernetes cluster in the cloud, follow [Install Coder on Kubernetes](../kubernetes/index.md) instead.
That guide covers managed clusters such as Amazon Elastic Kubernetes Service (EKS), Azure Kubernetes Service (AKS), and Google Kubernetes Engine (GKE).
Kubernetes is the recommended platform for deployments that others depend on.
The AWS Marketplace guide uses a CloudFormation template that creates a new EKS cluster for a proof of concept.

<div class="tabs">

## AWS

For a deployment that other people depend on, install Coder with Helm on Amazon Elastic Kubernetes Service (EKS):

- [Install Coder on Amazon EKS](./aws-marketplace.md#install-coder-on-amazon-eks)

To try Coder Community Edition in a sandbox account, launch it from the AWS Marketplace:

- [Install Coder Community Edition from AWS Marketplace](./aws-marketplace.md#launch-coder-community-edition-from-the-aws-marketplace)

Alternatively, install the [CLI binary](https://coder.com/downloads) on any Linux machine.

For installation guidance specific to EKS, refer to the [AWS section in Kubernetes installation docs](../kubernetes/index.md#aws).

## GCP

We publish a GCP Marketplace listing with Coder pre-installed.
Follow the tutorial here:

- [Install Coder on GCP Compute Engine](./compute-engine.md)

Alternatively, install the [CLI binary](https://coder.com/downloads) on any Linux machine or follow our [Kubernetes](../kubernetes/index.md) documentation to install Coder on an existing Google Kubernetes Engine (GKE) cluster.

## Azure

Use the following guide to run Coder on an Azure VM:

- [Install Coder on an Azure VM](./azure-vm.md)

Alternatively, install the [CLI binary](https://coder.com/downloads) on any Linux machine or follow our [Kubernetes](../kubernetes/index.md) documentation to install Coder on an existing AKS cluster.

## Other

Is your cloud missing?
Check [community](../community.md) install methods or install the [standalone binary](https://coder.com/downloads).

</div>
