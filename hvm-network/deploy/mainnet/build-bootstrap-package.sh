#!/usr/bin/env bash
set -euo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"
if [ -n "$(git status --porcelain)" ]; then
  echo 'Refusing package from modified source tree' >&2
  exit 1
fi
commit=$(git rev-parse HEAD)
output=${1:?Supply a new output directory outside the repository}
python3 - "$root" "$output" <<'PY'
import pathlib,sys
root=pathlib.Path(sys.argv[1]).resolve()
out=pathlib.Path(sys.argv[2]).resolve()
if out == root or root in out.parents:
    raise SystemExit('Output must be outside source tree')
out.mkdir(mode=0o700,parents=False,exist_ok=False)
PY
output=$(cd "$output" && pwd)
go -C hvm-network test -race ./internal/mainnetidentity ./cmd/hvm-mainnet-identity ./internal/testnet ./cmd/hashburst-mainnet
go -C hvm-network test -race ./blockchain -run 'TestMainnetBootstrap|TestMainnetImport' -count=1
for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
  mkdir "$output/$target"
  for command in hvm-mainnet-identity hvm-mainnet-bootstrap hashburst-mainnet; do
    GOOS=${target%-*} GOARCH=${target#*-} CGO_ENABLED=0 \
      go -C hvm-network build -trimpath -o "$output/$target/$command" "./cmd/$command"
  done
done
printf '%s\n' "$commit" > "$output/SOURCE_COMMIT"
printf '%s\n' 'OFFLINE CANDIDATE; NOT PRODUCTION ACTIVATION OR FINAL RELEASE' > "$output/STATUS.txt"
cp hvm-network/deploy/mainnet/BOOTSTRAP-PROCEDURE.md hvm-network/deploy/mainnet/ACTIVATION-INPUTS.md "$output/"
python3 - "$output" <<'PY'
import hashlib,pathlib,sys,tarfile
out=pathlib.Path(sys.argv[1])
files=sorted(p for p in out.rglob('*') if p.is_file())
with (out/'SHA256SUMS').open('x') as f:
    for p in files:
        f.write(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+str(p.relative_to(out))+'\n')
archive=out.with_suffix('.tar.gz')
with archive.open('xb') as raw:
    with tarfile.open(fileobj=raw,mode='w:gz') as tar:
        tar.add(out,arcname=out.name)
print('CANDIDATE_PACKAGE='+str(archive))
PY
