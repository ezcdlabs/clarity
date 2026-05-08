FROM golang:alpine

RUN apk add --no-cache git openssh-client bash

# Install the test SSH identity
RUN mkdir -p /root/.ssh && chmod 700 /root/.ssh
COPY test/integration/keys/test_client_key /root/.ssh/id_ed25519
RUN chmod 600 /root/.ssh/id_ed25519

# SSH client config: route the "server" hostname to the git server container.
# StrictHostKeyChecking=accept-new trusts new host keys but rejects changes
# (safe for an isolated test network; avoids baking in a specific host key).
RUN printf 'Host server\n  User git\n  IdentityFile /root/.ssh/id_ed25519\n  StrictHostKeyChecking accept-new\n  BatchMode yes\n' \
    > /root/.ssh/config \
    && chmod 600 /root/.ssh/config

# git identity for commits made inside tests
RUN git config --global user.email "test@clarity" \
    && git config --global user.name "Test"

# The clarity source is mounted at /workspace (see docker-compose.yml).
WORKDIR /workspace
