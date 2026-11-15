#!/bin/zsh
# in-container.zsh — run any make target inside the tools container, with nothing to install but podman or docker.
#
# The first run builds the image (Go, jq, zsh, GNAT 15, GPRbuild, GNATprove); later runs reuse it.
# Results are written to this repository's build/ folder, so you can open them afterwards.
#
# Usage:
#   tools/in-container.zsh check
#   tools/in-container.zsh prove
#   MY_KEY=… tools/in-container.zsh try BASE_URL=https://api.example.com/v1 MODEL=model-id KEY_ENV=MY_KEY
#
# Your API key is passed in only by name (KEY_ENV=…), from your environment, and is never written to disk.
#
# WHY THERE ARE NO MANUAL INSTALL STEPS
# This script is the only supported way to set up the tools, on purpose. Half-followed install notes give a
# different compiler or prover version, or a missing library, and then the proofs or scores seem not to
# reproduce when the real cause is the setup. One automated route leaves no room for that.
#
# IF YOU WANT TO DO IT BY HAND
# Everything this script does is written down in files you can read, and those files are what actually runs,
# so they cannot drift from the truth:
#   Containerfile   the base system (Ubuntu 24.04), the Go version, Alire, and the exact Ada toolchain
#                   (gnat_native 15.1.2, gprbuild, gnatprove) installed with "alr install"
#   Makefile        every step of the study, one target each, with the exact commands, including
#                   the GNATprove switches the proofs were run with (--level=2 --report=all)
#   proven/*/admit-A-*/proof.gpr, proven/*-edge/edge.gpr
#                   the project files for each proof and each command-line program
# Repeat those steps on your own machine and you will be running what we ran.
#
# Supported: macOS (Apple Silicon) and Ubuntu 24.04 (x86_64 or arm64).
set -e
repo=${0:A:h:h}
image=epistemics-study
tool=$(command -v podman || command -v docker || true)
[[ -n $tool ]] || { echo "in-container: install podman or docker first" >&2; exit 1; }
(( $# > 0 )) || set -- help

case $(uname -m) in arm64|aarch64) platform=linux/arm64;; *) platform=linux/amd64;; esac
if ! $tool image inspect $image > /dev/null 2>&1; then
  echo "Building the tools container for $platform (first run only, a few minutes)…"
  $tool build --platform $platform -t $image $repo
fi

envargs=()
for a in "$@"; do
  [[ $a == KEY_ENV=* ]] && envargs+=(-e ${a#KEY_ENV=})
done

tty=()
[[ -t 0 && -t 1 ]] && tty=(-it)
mkdir -p $repo/build
exec $tool run --rm $tty $envargs -v $repo/build:/study/build --network host $image make "$@"
