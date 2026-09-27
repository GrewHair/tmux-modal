# A remote host for the SSH and nested-tmux integration tier: sshd with
# key-only login for `demo`, and the apps under test (htop, vim) plus tmux
# for the nested case. Nothing tmux-modal related is installed remotely —
# the plugin must work with zero remote configuration.
ARG BASE=ubuntu:24.04
FROM ${BASE}
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      openssh-server htop vim-nox less tmux locales \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd \
    && useradd -m -s /bin/bash demo \
    && mkdir -p /home/demo/.ssh && chmod 700 /home/demo/.ssh \
    && chown -R demo:demo /home/demo/.ssh \
    && sed -i -e 's/^#\?PasswordAuthentication .*/PasswordAuthentication no/' \
              -e 's/^#\?PermitRootLogin .*/PermitRootLogin no/' /etc/ssh/sshd_config \
    && echo 'AcceptEnv LANG LC_*' >> /etc/ssh/sshd_config
# The test mounts its public key at /authorized_keys; the entrypoint installs
# it with the permissions sshd insists on.
COPY sshd-entrypoint.sh /usr/local/bin/sshd-entrypoint.sh
EXPOSE 22
ENTRYPOINT ["/usr/local/bin/sshd-entrypoint.sh"]
