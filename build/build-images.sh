#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
platform="${GLYPHFLOW_PLATFORM:-linux/amd64}"
image="${GLYPHFLOW_IMAGE:-glyphflow:latest}"
target="${1:-all}"

case "$target" in
    linux)
        images=("$image" "glyphflow:linux")
        archive="$repo_root/build/glyphflow-linux.tar.gz"
        ;;
    windows)
        # Windows targets use Docker Desktop/WSL2 Linux containers.
        images=("$image" "glyphflow:windows")
        archive="$repo_root/build/glyphflow-windows.tar.gz"
        ;;
    all)
        images=("glyphflow:linux" "glyphflow:windows" "$image")
        archive="$repo_root/build/glyphflow-images.tar.gz"
        ;;
    *)
        echo "usage: $0 [linux|windows|all]" >&2
        exit 2
        ;;
esac

build_args=(
    buildx build
    --platform "$platform"
    --file "$repo_root/build/Dockerfile"
    --load
)
for image in "${images[@]}"; do
    build_args+=(--tag "$image")
done
build_args+=("$repo_root")

docker "${build_args[@]}"

docker save "${images[@]}" | gzip > "$archive"
