---
title: Example policy
description: Review a broad Agent Firewall allowlist policy for multi-language workspaces.
---

> [!NOTE]
> Agent Firewall is part of [AI Governance](../ai-governance.md), which is included with a Premium license.

This page contains a broad Agent Firewall policy for workspaces where agents build software in many languages.
It allows the domains an agent needs to fetch dependencies, read source repositories, and pull container images, and blocks everything else.

Coder uses a policy of this shape on its own deployment.
Treat it as a starting point: allow only the domains your workspaces actually need, and add your own Coder deployment domain.
Most entries come from the [default allowed domains for Claude Code on the web](https://code.claude.com/docs/en/claude-code-on-the-web#default-allowed-domains).

For the syntax of each entry, including method filters and wildcards, refer to the [rules engine docs](rules-engine.md).
To load this policy in a template, pass it to the module's `agent_firewall_config` argument as described in [Agent Firewall](index.md).

```yaml
allowlist:
  # Your Coder deployment.
  - domain=coder.example.com

  # Anthropic services.
  - domain=api.anthropic.com
  - domain=statsig.anthropic.com
  - domain=claude.ai

  # Version control.
  - domain=github.com
  - domain=www.github.com
  - domain=api.github.com
  - domain=raw.githubusercontent.com
  - domain=objects.githubusercontent.com
  - domain=codeload.github.com
  - domain=avatars.githubusercontent.com
  - domain=camo.githubusercontent.com
  - domain=gist.github.com
  - domain=gitlab.com
  - domain=www.gitlab.com
  - domain=registry.gitlab.com
  - domain=bitbucket.org
  - domain=www.bitbucket.org
  - domain=api.bitbucket.org

  # Container registries.
  - domain=registry-1.docker.io
  - domain=auth.docker.io
  - domain=index.docker.io
  - domain=hub.docker.com
  - domain=www.docker.com
  - domain=production.cloudflare.docker.com
  - domain=download.docker.com
  - domain=*.gcr.io
  - domain=ghcr.io
  - domain=mcr.microsoft.com
  - domain=*.data.mcr.microsoft.com

  # Cloud platforms.
  - domain=cloud.google.com
  - domain=accounts.google.com
  - domain=gcloud.google.com
  - domain=*.googleapis.com
  - domain=storage.googleapis.com
  - domain=compute.googleapis.com
  - domain=container.googleapis.com
  - domain=azure.com
  - domain=portal.azure.com
  - domain=microsoft.com
  - domain=www.microsoft.com
  - domain=*.microsoftonline.com
  - domain=packages.microsoft.com
  - domain=dotnet.microsoft.com
  - domain=dot.net
  - domain=visualstudio.com
  - domain=dev.azure.com
  - domain=oracle.com
  - domain=www.oracle.com
  - domain=java.com
  - domain=www.java.com
  - domain=java.net
  - domain=www.java.net
  - domain=download.oracle.com
  - domain=yum.oracle.com

  # Package managers, JavaScript/Node.
  - domain=registry.npmjs.org
  - domain=www.npmjs.com
  - domain=www.npmjs.org
  - domain=npmjs.com
  - domain=npmjs.org
  - domain=yarnpkg.com
  - domain=registry.yarnpkg.com

  # Package managers, Python.
  - domain=pypi.org
  - domain=www.pypi.org
  - domain=files.pythonhosted.org
  - domain=pythonhosted.org
  - domain=test.pypi.org
  - domain=pypi.python.org
  - domain=pypa.io
  - domain=www.pypa.io

  # Package managers, Ruby.
  - domain=rubygems.org
  - domain=www.rubygems.org
  - domain=api.rubygems.org
  - domain=index.rubygems.org
  - domain=ruby-lang.org
  - domain=www.ruby-lang.org
  - domain=rubyforge.org
  - domain=www.rubyforge.org
  - domain=rubyonrails.org
  - domain=www.rubyonrails.org
  - domain=rvm.io
  - domain=get.rvm.io

  # Package managers, Rust.
  - domain=crates.io
  - domain=www.crates.io
  - domain=static.crates.io
  - domain=rustup.rs
  - domain=static.rust-lang.org
  - domain=www.rust-lang.org

  # Package managers, Go.
  - domain=proxy.golang.org
  - domain=sum.golang.org
  - domain=index.golang.org
  - domain=golang.org
  - domain=www.golang.org
  - domain=go.dev
  - domain=dl.google.com
  - domain=goproxy.io
  - domain=pkg.go.dev

  # Package managers, JVM.
  - domain=maven.org
  - domain=repo.maven.org
  - domain=central.maven.org
  - domain=repo1.maven.org
  - domain=jcenter.bintray.com
  - domain=gradle.org
  - domain=www.gradle.org
  - domain=services.gradle.org
  - domain=spring.io
  - domain=repo.spring.io

  # Package managers, other languages.
  - domain=packagist.org
  - domain=www.packagist.org
  - domain=repo.packagist.org
  - domain=nuget.org
  - domain=www.nuget.org
  - domain=api.nuget.org
  - domain=pub.dev
  - domain=api.pub.dev
  - domain=hex.pm
  - domain=www.hex.pm
  - domain=cpan.org
  - domain=www.cpan.org
  - domain=metacpan.org
  - domain=www.metacpan.org
  - domain=api.metacpan.org
  - domain=cocoapods.org
  - domain=www.cocoapods.org
  - domain=cdn.cocoapods.org
  - domain=haskell.org
  - domain=www.haskell.org
  - domain=hackage.haskell.org
  - domain=swift.org
  - domain=www.swift.org

  # Linux distributions.
  - domain=archive.ubuntu.com
  - domain=security.ubuntu.com
  - domain=ubuntu.com
  - domain=www.ubuntu.com
  - domain=*.ubuntu.com
  - domain=ppa.launchpad.net
  - domain=launchpad.net
  - domain=www.launchpad.net

  # Development tools and platforms.
  - domain=dl.k8s.io
  - domain=pkgs.k8s.io
  - domain=k8s.io
  - domain=www.k8s.io
  - domain=releases.hashicorp.com
  - domain=apt.releases.hashicorp.com
  - domain=rpm.releases.hashicorp.com
  - domain=archive.releases.hashicorp.com
  - domain=hashicorp.com
  - domain=www.hashicorp.com
  - domain=repo.anaconda.com
  - domain=conda.anaconda.org
  - domain=anaconda.org
  - domain=www.anaconda.com
  - domain=anaconda.com
  - domain=continuum.io
  - domain=apache.org
  - domain=www.apache.org
  - domain=archive.apache.org
  - domain=downloads.apache.org
  - domain=eclipse.org
  - domain=www.eclipse.org
  - domain=download.eclipse.org
  - domain=nodejs.org
  - domain=www.nodejs.org

  # Cloud services and monitoring.
  - domain=statsig.com
  - domain=www.statsig.com
  - domain=api.statsig.com
  - domain=*.sentry.io

  # Content delivery and mirrors.
  - domain=*.sourceforge.net
  - domain=packagecloud.io
  - domain=*.packagecloud.io

  # Schema and configuration.
  - domain=json-schema.org
  - domain=www.json-schema.org
  - domain=json.schemastore.org
  - domain=www.schemastore.org
log_dir: /tmp/boundary_logs
log_level: warn
proxy_port: 8087
```
