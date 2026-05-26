FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

# Install Go, Node, and all dev tooling in a single layer to minimize image size.
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl \
        ca-certificates \
        gnupg \
        unzip \
        git \
        make \
    && \
    # --- Go 1.26 ---
    curl -fsSL https://go.dev/dl/go1.26.0.linux-amd64.tar.gz -o /tmp/go.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz && \
    # --- Node 22 LTS via NodeSource ---
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    # --- golang-migrate v4.18 ---
    curl -fsSL https://github.com/golang-migrate/migrate/releases/download/v4.18.1/migrate.linux-amd64.tar.gz \
        -o /tmp/migrate.tar.gz && \
    tar -C /usr/local/bin -xzf /tmp/migrate.tar.gz migrate && \
    rm /tmp/migrate.tar.gz && \
    # --- golangci-lint v1.62 ---
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
        | sh -s -- -b /usr/local/bin v1.62.0 && \
    # --- buf v1.47 ---
    curl -fsSL https://github.com/bufbuild/buf/releases/download/v1.47.2/buf-Linux-x86_64 \
        -o /usr/local/bin/buf && \
    chmod +x /usr/local/bin/buf && \
    # --- air and sqlc via go install ---
    export PATH="$PATH:/usr/local/go/bin" && \
    export GOPATH=/root/go && \
    go install github.com/air-verse/air@latest && \
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest && \
    # --- cleanup ---
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

ENV PATH="$PATH:/usr/local/go/bin:/root/go/bin"

# Create non-root vscode user (uid 1000) for remote user
RUN groupadd --gid 1000 vscode && \
    useradd --uid 1000 --gid 1000 --create-home --shell /bin/bash vscode
