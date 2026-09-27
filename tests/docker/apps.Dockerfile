# Clean-room images for capturing fixtures of real applications, so that
# fixtures contain no host process lists, paths or user names.
#
#   docker build --build-arg BASE=ubuntu:24.04 -t tmux-modal-fx:ubuntu-24.04 \
#     -f tests/docker/apps.Dockerfile tests/docker
#
# Packages a base does not have (btop, lf on older releases) are skipped;
# lazygit and k9s are not packaged and come from their GitHub releases.
ARG BASE=ubuntu:24.04
FROM ${BASE}
ARG LAZYGIT_VERSION=0.65.1
ARG K9S_VERSION=0.51.0
ENV DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8 TERM=xterm-256color
COPY apps-install.sh /usr/local/sbin/apps-install.sh
RUN LAZYGIT_VERSION=${LAZYGIT_VERSION} K9S_VERSION=${K9S_VERSION} sh /usr/local/sbin/apps-install.sh
RUN useradd -m -s /bin/bash demo
USER demo
WORKDIR /home/demo
COPY --chown=demo:demo apps-home.sh /tmp/apps-home.sh
RUN sh /tmp/apps-home.sh
