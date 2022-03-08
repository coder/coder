FROM registry.access.redhat.com/ubi8/ubi:latest

RUN yum install -y yum-utils
RUN yum-config-manager --add-repo https://rpm.releases.hashicorp.com/RHEL/hashicorp.repo
RUN yum install -y terraform

COPY coder /coder
RUN chmod +x /coder

COPY run.sh /run.sh
RUN chmod +x /run.sh

ENTRYPOINT ["/coder/run.sh"]
