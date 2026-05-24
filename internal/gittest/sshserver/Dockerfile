FROM alpine:3.19

RUN apk add --no-cache openssh git netcat-openbsd

# Generate sshd host keys
RUN ssh-keygen -A

# Create a dedicated git user whose shell is git-shell (restricts to git ops only)
RUN adduser -D -s /usr/bin/git-shell git \
    && mkdir -p /home/git/.ssh \
    && chmod 700 /home/git/.ssh \
    && passwd -u git

# Install the test client's public key. Build context is this directory
# (internal/gittest/sshserver/) — see gittest_ssh.go for the testcontainers
# FromDockerfile request that wires it up.
COPY keys/test_client_key.pub /home/git/.ssh/authorized_keys
RUN chmod 600 /home/git/.ssh/authorized_keys \
    && chown -R git:git /home/git/.ssh

# Harden sshd: no root login, no passwords, pubkey only.
# UsePAM no is required because adduser -D creates accounts with a locked
# password field (!), which OpenSSH's PAM integration treats as "account locked"
# even when using public key auth.
RUN printf '\nPermitRootLogin no\nAllowUsers git\nPasswordAuthentication no\nPubkeyAuthentication yes\nUsePAM no\n' \
    >> /etc/ssh/sshd_config

# Create the test bare repo with GitHub-style fsck enabled. This is the key
# setting — it catches the "fullPathname" / flat-tree class of bug locally
# instead of waiting for a real GitHub push to fail.
RUN git config --global init.defaultBranch main \
    && git config --global user.email "seed@test" \
    && git config --global user.name "Seed" \
    && git config --global --add safe.directory /home/git/repo.git \
    && git init --bare --initial-branch=main /home/git/repo.git \
    && git config --file /home/git/repo.git/config receive.fsckObjects true

# Seed an initial commit on main so clones have something to pull from.
RUN mkdir /tmp/seed \
    && git clone /home/git/repo.git /tmp/seed \
    && touch /tmp/seed/.gitkeep \
    && git -C /tmp/seed add . \
    && git -C /tmp/seed commit -m "initial commit" \
    && git -C /tmp/seed push origin main \
    && rm -rf /tmp/seed \
    && chown -R git:git /home/git/repo.git

EXPOSE 22

# -D: don't daemonise  -e: log to stderr
CMD ["/usr/sbin/sshd", "-D", "-e"]
