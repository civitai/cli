# A3: Ubuntu 24.04 with node+npm from the distro. Older npm, distro-owned global
# prefix, unprivileged user. The "Node came from a system package manager" case.
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
      nodejs npm zsh curl ca-certificates git jq procps less \
    && rm -rf /var/lib/apt/lists/*
RUN useradd -m -s /bin/zsh dev && mkdir -p /work && chown dev:dev /work
USER dev
RUN printf '%s\n' '# login shell' > /home/dev/.zshrc
WORKDIR /work
ENV HOME=/home/dev
