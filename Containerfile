# Tools container: everything needed to repeat the study, including the SPARK prover.
#
# Easiest: tools/in-container.zsh <make target>   (builds this image on first use, for your machine's own CPU)
# By hand:
#   podman build -t epistemics-study .          (or: docker build -t epistemics-study .)
#   podman run --rm epistemics-study make check
#
# Holds Go, jq, zsh and make, plus GNAT 15, GPRbuild and GNATprove from Alire.
FROM docker.io/library/ubuntu:24.04

ARG GO_VERSION=1.26.1
ARG ALR_VERSION=2.1.0
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl unzip git make zsh jq libc6-dev \
 && rm -rf /var/lib/apt/lists/*

# Go, for this machine's CPU.
RUN goarch=$(case $(uname -m) in aarch64|arm64) echo arm64;; *) echo amd64;; esac) \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${goarch}.tar.gz" | tar -xz -C /usr/local
ENV PATH="/usr/local/go/bin:${PATH}" GOTOOLCHAIN=local

# Alire, the Ada package manager, then the toolchain the proofs were run with.
RUN curl -fsSL -o /tmp/alr.zip \
      "https://github.com/alire-project/alire/releases/download/v${ALR_VERSION}/alr-${ALR_VERSION}-bin-$(uname -m)-linux.zip" \
 && unzip -q /tmp/alr.zip -d /opt/alire && rm /tmp/alr.zip \
 && ln -s /opt/alire/bin/alr /usr/local/bin/alr
RUN alr -n settings --global --set toolchain.assistant false \
 && alr -n install gnat_native=15.1.2 gprbuild gnatprove
ENV PATH="/root/.alire/bin:${PATH}"

WORKDIR /study
COPY . /study
RUN make -s help
CMD ["make", "help"]
