# tmux built from a release tarball, for probing formats newer than the
# distribution's tmux (e.g. #{cursor_shape}, absent in 3.4).
FROM ubuntu:24.04
ARG TMUX_VERSION=3.5a
ENV DEBIAN_FRONTEND=noninteractive LANG=C.UTF-8 TERM=xterm-256color
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential pkg-config libevent-dev libncurses-dev bison curl ca-certificates \
      neovim vim-nox htop \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://github.com/tmux/tmux/releases/download/${TMUX_VERSION}/tmux-${TMUX_VERSION}.tar.gz | tar xz \
    && cd tmux-${TMUX_VERSION} && ./configure --prefix=/usr/local >/dev/null && make -j"$(nproc)" >/dev/null && make install \
    && cd .. && rm -rf tmux-${TMUX_VERSION}
RUN useradd -m -s /bin/bash demo
USER demo
WORKDIR /home/demo
