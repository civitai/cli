# A4: a civitai CLI is ALREADY installed, and it is four releases old (0.1.101 —
# the exact version the prior dogfood run's failure was recorded against).
# Exercises step 2's "already prints a version, note it and go to step 3" and
# step 4's `civitai upgrade` branch.
FROM node:22-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      zsh curl ca-certificates git jq procps less \
    && rm -rf /var/lib/apt/lists/*
RUN npm install -g @civitai/cli@0.1.101 --no-audit --no-fund
RUN chsh -s /bin/zsh root && printf '%s\n' '# login shell' > /root/.zshrc
WORKDIR /work
ENV HOME=/root
