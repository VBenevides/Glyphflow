#!/usr/bin/env bash
set -Eeuo pipefail

output_dir=${1:?usage: $0 OUTPUT_DIR VERSION APPLICATION_IMAGE}
version=${2:?usage: $0 OUTPUT_DIR VERSION APPLICATION_IMAGE}
application_image=${3:?usage: $0 OUTPUT_DIR VERSION APPLICATION_IMAGE}
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
nats_image=${NATS_IMAGE_REF:-nats@sha256:b83efabe3e7def1e0a4a31ec6e078999bb17c80363f881df35edc70fcb6bb927}
postgres_image=${POSTGRES_IMAGE_REF:-postgres@sha256:44c4ee9810eff91f7eab4d822642e01115b1a9eccce4bcbdde7604752d68eac6}
mkdir -p "$output_dir"
output_dir="$(cd -- "$output_dir" && pwd)"
bundle_dir="$output_dir/glyphflow-$version"
images_archive="$bundle_dir/glyphflow-$version-deployment-images.tar.gz"

command -v docker >/dev/null || { echo "release package: Docker is required" >&2; exit 1; }
command -v sha256sum >/dev/null || { echo "release package: sha256sum is required" >&2; exit 1; }
command -v tar >/dev/null || { echo "release package: tar is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "release package: python3 is required" >&2; exit 1; }

mkdir -p "$bundle_dir"
docker pull "$application_image" >/dev/null
docker pull "$nats_image" >/dev/null
docker pull "$postgres_image" >/dev/null

image_digest() {
  local image=$1
  local digest
  digest=$(docker image inspect --format '{{index .RepoDigests 0}}' "$image")
  [ -n "$digest" ] || { echo "release package: no registry digest for $image" >&2; exit 1; }
  printf '%s' "$digest"
}

application_digest=$(image_digest "$application_image")
nats_digest=$(image_digest "$nats_image")
postgres_digest=$(image_digest "$postgres_image")

cp "$repo_root"/compose.yaml "$repo_root"/compose.production.yaml "$repo_root"/compose.partial.yaml \
  "$repo_root"/README.md "$repo_root"/docs/DEPLOYMENT.md "$repo_root"/LICENSE \
  "$repo_root"/THIRD-PARTY-NOTICES "$bundle_dir/"
cat > "$bundle_dir/images.env" <<EOF
GLYPHFLOW_IMAGE=$application_image
NATS_IMAGE=$nats_image
POSTGRES_IMAGE=$postgres_image
EOF

VERSION="$version" APPLICATION_IMAGE="$application_image" APPLICATION_DIGEST="$application_digest" \
NATS_IMAGE="$nats_image" NATS_DIGEST="$nats_digest" POSTGRES_IMAGE="$postgres_image" \
POSTGRES_DIGEST="$postgres_digest" IMAGES_ARCHIVE="$(basename "$images_archive")" \
python3 - "$bundle_dir/release-manifest.json.tmp" <<'PY'
import json
import os
import sys

manifest = {
    "version": os.environ["VERSION"],
    "compose": ["compose.yaml", "compose.production.yaml", "compose.partial.yaml"],
    "documentation": "DEPLOYMENT.md",
    "environment": "images.env",
    "images": {
        "glyphflow": {"reference": os.environ["APPLICATION_IMAGE"], "digest": os.environ["APPLICATION_DIGEST"]},
        "nats": {"reference": os.environ["NATS_IMAGE"], "digest": os.environ["NATS_DIGEST"]},
        "postgresql": {"reference": os.environ["POSTGRES_IMAGE"], "digest": os.environ["POSTGRES_DIGEST"]},
    },
    "offline_images": os.environ["IMAGES_ARCHIVE"],
}
with open(sys.argv[1], "w", encoding="utf-8") as output:
    json.dump(manifest, output, indent=2, sort_keys=True)
    output.write("\n")
PY
mv "$bundle_dir/release-manifest.json.tmp" "$bundle_dir/release-manifest.json"

docker save "$application_image" "$nats_image" "$postgres_image" | gzip -9 > "$images_archive"
(
  cd "$bundle_dir"
  find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
)

archive="$output_dir/glyphflow-$version-deployment.tar.gz"
tar -C "$output_dir" -czf "$archive" "glyphflow-$version"
sha256sum "$archive" > "$archive.sha256"
printf 'release package: %s\n' "$archive"
