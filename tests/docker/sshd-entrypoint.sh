#!/bin/sh
set -e
if [ -f /authorized_keys ]; then
	install -o demo -g demo -m 600 /authorized_keys /home/demo/.ssh/authorized_keys
fi
ssh-keygen -A >/dev/null
exec /usr/sbin/sshd -D -e
