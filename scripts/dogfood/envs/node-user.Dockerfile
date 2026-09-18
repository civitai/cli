# A2: node 22, running as an unprivileged user. npm's global prefix (/usr/local)
# is NOT writable, so `npm install -g` fails with EACCES — the case prompt.md's
# "If the install fails" section documents.
FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      zsh curl ca-certificates git jq procps less \
    && rm -rf /var/lib/apt/lists/*
RUN useradd -m -s /bin/zsh dev && mkdir -p /work && chown dev:dev /work
USER dev
RUN printf '%s\n' '# login shell' > /home/dev/.zshrc
WORKDIR /work
ENV HOME=/home/dev
