# A remote host for the SSH and nested-tmux integration tier: sshd with
# key-only login for `demo`, the apps under test, and tmux for the nested
# case. Nothing tmux-modal related is installed remotely — the plugin must
# work with zero remote configuration.
ARG BASE=ubuntu:24.04
FROM ${BASE}
ARG LAZYGIT_VERSION=0.65.1
ENV DEBIAN_FRONTEND=noninteractive
COPY apps-install.sh /usr/local/sbin/apps-install.sh
COPY debconf-probe.templates debconf-probe.sh ucf-probe.sh /usr/local/share/tmux-modal/
RUN APPS="openssh-server htop vim-nox neovim less man-db tmux locales tig btop ncdu ranger lf nnn mc fzf git ca-certificates curl whiptail debconf-i18n debconf-utils ucf sudo" \
      LAZYGIT_VERSION=${LAZYGIT_VERSION} sh /usr/local/sbin/apps-install.sh \
    && mkdir -p /run/sshd \
    && useradd -m -s /bin/bash demo \
    && echo 'demo ALL=(ALL) NOPASSWD: ALL' >/etc/sudoers.d/demo \
    && mkdir -p /home/demo/.ssh && chmod 700 /home/demo/.ssh \
    && chown -R demo:demo /home/demo/.ssh \
    && sed -i -e 's/^#\?PasswordAuthentication .*/PasswordAuthentication no/' \
              -e 's/^#\?PermitRootLogin .*/PermitRootLogin no/' /etc/ssh/sshd_config \
    && echo 'AcceptEnv LANG LC_*' >> /etc/ssh/sshd_config
COPY --chown=demo:demo apps-home.sh /tmp/apps-home.sh
RUN su demo -c 'sh /tmp/apps-home.sh'
# The test mounts its public key at /authorized_keys; the entrypoint installs
# it with the permissions sshd insists on.
COPY sshd-entrypoint.sh /usr/local/bin/sshd-entrypoint.sh
EXPOSE 22
ENTRYPOINT ["/usr/local/bin/sshd-entrypoint.sh"]
