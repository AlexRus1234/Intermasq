#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
IMAGE_NAME=${INTERMASQ_LAB_BUILDER_IMAGE:-intermasq-lab-builder:local}

command -v podman >/dev/null 2>&1 || {
	printf '%s\n' 'error: podman is required' >&2
	exit 1
}

if ! podman info >/dev/null 2>&1; then
	if ! podman machine start >/dev/null 2>&1; then
		podman machine init --now
	fi
fi

podman info >/dev/null 2>&1 || {
	printf '%s\n' 'error: podman engine is not available after machine startup' >&2
	exit 1
}

mkdir -p "$SCRIPT_DIR/output"
podman build --pull=missing -f "$SCRIPT_DIR/Containerfile" -t "$IMAGE_NAME" "$SCRIPT_DIR"
podman run --rm --user 0 \
	-v "$REPO_DIR:/src:ro" \
	-v "$SCRIPT_DIR/output:/out:rw" \
	"$IMAGE_NAME" /bin/sh /src/distro/build-inside.sh
