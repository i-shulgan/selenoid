#!/bin/sh
set -e

# The selenoid user talks to the Docker daemon through the docker group,
# created by the docker-ce or docker.io package.
if ! getent passwd selenoid >/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin --groups docker selenoid
fi

# The service is not enabled or started: it needs /etc/selenoid/browsers.json.
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
fi
