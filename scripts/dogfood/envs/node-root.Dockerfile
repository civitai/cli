# A1: node 22, running as root. npm's global prefix is writable — the happy path.
FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      zsh curl ca-certificates git jq procps less \
    && rm -rf /var/lib/apt/lists/*
RUN chsh -s /bin/zsh root && printf '%s\n' '# login shell' > /root/.zshrc
WORKDIR /work
ENV HOME=/root
