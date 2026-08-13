#!/usr/bin/env sh
set -eu

output=${1:-internal/v0-review/SBOM.spdx.json}
mkdir -p "$(dirname "$output")"
python3 -c '
import json, sys
from pathlib import Path

packages = [{"SPDXID": "SPDXRef-glyphflow-backend", "name": "github.com/VBenevides/Glyphflow/backend", "versionInfo": "local", "downloadLocation": "NOASSERTION"}]
seen = set()
for line in Path("backend/go.sum").read_text().splitlines():
    fields = line.split()
    if len(fields) < 2 or fields[0].endswith("/go.mod"):
        continue
    name, version = fields[:2]
    if (name, version) in seen:
        continue
    seen.add((name, version))
    safe = "".join(ch if ch.isalnum() else "-" for ch in name + "-" + version)
    packages.append({"SPDXID": "SPDXRef-" + safe, "name": name, "versionInfo": version, "downloadLocation": "NOASSERTION"})
document = {"spdxVersion": "SPDX-2.3", "dataLicense": "CC0-1.0", "SPDXID": "SPDXRef-DOCUMENT", "name": "glyphflow-backend-sbom", "documentNamespace": "https://github.com/VBenevides/Glyphflow/sbom/backend", "creationInfo": {"creators": ["Tool: Glyphflow generate-sbom"]}, "packages": packages}
print(json.dumps(document, indent=2, sort_keys=True))
' > "$output.tmp"
mv "$output.tmp" "$output"
