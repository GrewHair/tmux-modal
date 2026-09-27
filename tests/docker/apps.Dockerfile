# Clean-room images for capturing fixtures of real applications, so that
# fixtures contain no host process lists, paths or user names.
ARG BASE=ubuntu:24.04
FROM ${BASE}
ENV DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8 TERM=xterm-256color
RUN apt-get update && apt-get install -y --no-install-recommends htop less man-db vim-nox \
    && rm -rf /var/lib/apt/lists/*
RUN useradd -m -s /bin/bash demo
USER demo
WORKDIR /home/demo
