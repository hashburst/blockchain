#!/usr/bin/env bash
set -euo pipefail
DIR="${1:?release directory required}"
PUB="${2:?Ed25519 public key required}"
test -s "$DIR/SHA256SUMS.sig" || { echo "unsigned package rejected" >&2; exit 1; }
openssl pkeyutl -verify -pubin -inkey "$PUB" -rawin -in "$DIR/SHA256SUMS" -sigfile "$DIR/SHA256SUMS.sig" >/dev/null
(cd "$DIR" && sha256sum -c SHA256SUMS)
python3 - "$DIR" <<'PY'
import json,sys
from pathlib import Path
p=Path(sys.argv[1])
m=json.loads((p/"MANIFEST.json").read_text())
v=json.loads((p/"PROVENANCE.json").read_text())
s=json.loads((p/"SBOM.cdx.json").read_text())
assert m["schema"]=="hashburst-g5-manifest-v1"
assert v["schema"]=="hashburst-g5-original-build-provenance-v1"
assert v["original_build_provenance"] is True
assert s["bomFormat"]=="CycloneDX" and s["specVersion"]=="1.5"
assert (p/"SOURCE_COMMIT").read_text().strip()==m["source_commit"]==v["source_commit"]
PY
echo HASHBURST_G5_PACKAGE_VERIFICATION_OK
