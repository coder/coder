<div align="center">
  <a href="https://coder.com#gh-light-mode-only">
    <img src="./docs/images/logo-black.png" style="width: 128px">
  </a>
  <a href="https://coder.com#gh-dark-mode-only">
    <img src="./docs/images/logo-white.png" style="width: 128px">
  </a>

  <h1>
  Ambientes de Desenvolvimento em Nuvem Auto-Hospedados
  </h1>

  <a href="https://coder.com#gh-light-mode-only">
    <img src="./docs/images/banner-black.png" style="width: 650px">
  </a>
  <a href="https://coder.com#gh-dark-mode-only">
    <img src="./docs/images/banner-white.png" style="width: 650px">
  </a>

  <br>
  <br>

[Início Rápido](#início-rápido) | [Documentação](https://coder.com/docs) | [Por que Coder](https://coder.com/why) | [Enterprise](https://coder.com/docs/enterprise)

[![discord](https://img.shields.io/discord/747933592273027093?label=discord)](https://discord.gg/coder)
[![release](https://img.shields.io/github/v/release/coder/coder)](https://github.com/coder/coder/releases/latest)
[![godoc](https://pkg.go.dev/badge/github.com/coder/coder.svg)](https://pkg.go.dev/github.com/coder/coder)
[![Go Report Card](https://goreportcard.com/badge/github.com/coder/coder/v2)](https://goreportcard.com/report/github.com/coder/coder/v2)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/9511/badge)](https://www.bestpractices.dev/projects/9511)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/coder/coder/badge)](https://api.securityscorecards.dev/projects/github.com/coder/coder)
[![license](https://img.shields.io/github/license/coder/coder)](./LICENSE)

</div>

[Coder](https://coder.com) permite que organizações configurem ambientes de desenvolvimento em sua infraestrutura de nuvem pública ou privada. Ambientes de desenvolvimento em nuvem são definidos com Terraform, conectados através de um túnel seguro de alta velocidade Wireguard® e desligados automaticamente quando não estão em uso para economizar custos. O Coder oferece às equipes de engenharia a flexibilidade de usar a nuvem para as cargas de trabalho mais benéficas para elas.

- Defina ambientes de desenvolvimento em nuvem no Terraform
  - VMs EC2, Pods Kubernetes, Contêineres Docker, etc.
- Desligue automaticamente recursos ociosos para economizar custos
- Integre desenvolvedores em segundos em vez de dias

<p align="center">
  <img src="./docs/images/hero-image.png">
</p>

## Início Rápido

A maneira mais conveniente de experimentar o Coder é instalá-lo em sua máquina local e experimentar provisionar ambientes de desenvolvimento em nuvem usando Docker (funciona no Linux, macOS e Windows).

```
# Primeiro, instale o Coder
curl -L https://coder.com/install.sh | sh

# Inicie o servidor Coder (armazena dados em ~/.cache/coder)
coder server

# Navegue para http://localhost:3000 para criar seu usuário inicial,
# criar um modelo Docker e provisionar um workspace
```

## Instalação

A maneira mais fácil de instalar o Coder é usar nosso
[script de instalação](https://github.com/coder/coder/blob/main/install.sh) para Linux
e macOS. Para Windows, use o arquivo mais recente `..._installer.exe` do GitHub
Releases.

```bash
curl -L https://coder.com/install.sh | sh
```

Você pode executar o script de instalação com `--dry-run` para ver os comandos que serão usados para instalar sem executá-los. Execute o script de instalação com `--help` para obter flags adicionais.

> Veja [instalação](https://coder.com/docs/install) para métodos adicionais.

Uma vez instalado, você pode iniciar uma implantação de produção com um único comando:

```shell
# Configura automaticamente uma URL de acesso externo em *.try.coder.app
coder server

# Requer uma instância PostgreSQL (versão 13 ou superior) e URL de acesso externo
coder server --postgres-url <url> --access-url <url>
```

Use `coder --help` para obter uma lista de flags e variáveis de ambiente. Use nossos [guias de instalação](https://coder.com/docs/install) para um passo a passo completo.

## Documentação

Navegue em nossa documentação [aqui](https://coder.com/docs) ou visite uma seção específica abaixo:

- [**Modelos**](https://coder.com/docs/templates): Modelos são escritos em Terraform e descrevem a infraestrutura para workspaces
- [**Workspaces**](https://coder.com/docs/workspaces): Workspaces contêm os IDEs, dependências e informações de configuração necessárias para o desenvolvimento de software
- [**IDEs**](https://coder.com/docs/ides): Conecte seu editor existente a um workspace
- [**Administração**](https://coder.com/docs/admin): Aprenda a operar o Coder
- [**Enterprise**](https://coder.com/docs/enterprise): Saiba mais sobre nossos recursos pagos desenvolvidos para grandes equipes

## Suporte

Sinta-se à vontade para [abrir um problema](https://github.com/coder/coder/issues/new) se tiver dúvidas, encontrar bugs ou tiver uma solicitação de recurso.

[Junte-se ao nosso Discord](https://discord.gg/coder) para fornecer feedback sobre recursos em andamento e conversar com a comunidade que usa o Coder!

## Integrações

Estamos sempre trabalhando em novas integrações. Sinta-se à vontade para abrir um problema e pedir uma integração. Contribuições são bem-vindas em qualquer repositório oficial ou da comunidade.

### Oficial

- [**Extensão VS Code**](https://marketplace.visualstudio.com/items?itemName=coder.coder-remote): Abra qualquer workspace do Coder no VS Code com um único clique
- [**Extensão JetBrains Gateway**](https://plugins.jetbrains.com/plugin/19620-coder): Abra qualquer workspace do Coder no JetBrains Gateway com um único clique
- [**Construtor de Contêineres de Desenvolvimento**](https://github.com/coder/envbuilder): Construa ambientes de desenvolvimento usando `devcontainer.json` no Docker, Kubernetes e OpenShift
- [**Registro de Módulos**](https://registry.coder.com): Estenda ambientes de desenvolvimento com casos de uso comuns
- [**Stream de Logs do Kubernetes**](https://github.com/coder/coder-logstream-kube): Transmita eventos de Pods do Kubernetes para os logs de inicialização do Coder
- [**Marketplace de Extensões VS Code Auto-Hospedado**](https://github.com/coder/code-marketplace): Um marketplace de extensões privado que funciona em redes restritas ou isoladas, integrando-se com [code-server](https://github.com/coder/code-server).
- [**Configurar Coder**](https://github.com/marketplace/actions/setup-coder): Uma ação para configurar o CLI do Coder em fluxos de trabalho do GitHub.

### Comunidade

- [**Provisionar Coder com Terraform**](https://github.com/ElliotG/coder-oss-tf): Provisionar Coder no Google GKE, Azure AKS, AWS EKS, DigitalOcean DOKS, IBMCloud K8s, OVHCloud K8s e Scaleway K8s Kapsule com Terraform
- [**Ação do GitHub para Modelos do Coder**](https://github.com/marketplace/actions/update-coder-template): Uma ação do GitHub que atualiza os modelos do Coder

## Contribuindo

Estamos sempre felizes em ver novos contribuidores para o Coder. Se você é novo no código do Coder, temos
[um guia sobre como começar](https://coder.com/docs/CONTRIBUTING). Adoraríamos ver suas
contribuições!

## Contratação

Candidate-se [aqui](https://jobs.ashbyhq.com/coder?utm_source=github&utm_medium=readme&utm_campaign=unknown) se estiver interessado em se juntar à nossa equipe.
