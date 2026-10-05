#!/usr/bin/env python3
"""Read-only inventory for the approved chain-1337 repair; no restart or writes remotely."""
import argparse
import datetime
import json
import pathlib
import subprocess

REMOTE = r'''
import base64, hashlib, json, os, pathlib, stat, subprocess, urllib.request
LIMIT = 16 * 1024 * 1024
UNIT = "hashburst-node.service"
PROPS = ("ActiveState", "SubState", "Result", "MainPID", "ExecMainStatus", "NRestarts",
         "User", "Group", "FragmentPath", "DropInPaths", "EnvironmentFiles",
         "ProtectSystem", "ReadWritePaths")

def service():
    p = subprocess.run(["systemctl", "show", UNIT] + ["--property=" + x for x in PROPS],
                       text=True, capture_output=True, timeout=15)
    return {"exit_code": p.returncode, "properties": dict(
        line.split("=", 1) for line in p.stdout.splitlines() if "=" in line)}

def read_public(path):
    p = pathlib.Path(path)
    if p.resolve(strict=True) != p:
        raise RuntimeError("Noncanonical path refused")
    fd = os.open(str(p), os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as f:
        a = os.fstat(f.fileno())
        if not stat.S_ISREG(a.st_mode) or a.st_size > LIMIT:
            raise RuntimeError("Unexpected type or size")
        data = f.read(LIMIT + 1)
        b = os.fstat(f.fileno())
    stamp = lambda s: (s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns, s.st_ctime_ns)
    if stamp(a) != stamp(b) or len(data) != a.st_size:
        raise RuntimeError("File changed during read")
    return data, stamp(b)

result = {"read_only": True, "unit": UNIT, "service_before": service()}
# Explicitly select only public settings. Never return the environment or key contents.
public_names = {"NODE_ID", "RPC_PORT", "P2P_PORT", "STORAGE_DIR", "P2P_KEY_PATH"}
settings = {}
try:
    for line in pathlib.Path("/etc/hashburst/env").read_text().splitlines():
        key, sep, value = line.partition("=")
        if sep and key.strip() in public_names:
            settings[key.strip()] = value.strip().strip("\"'")
except OSError as exc:
    result["environment_file_error"] = str(exc)
result["declared_public_settings"] = settings
pid = int(result["service_before"]["properties"].get("MainPID", "0"))
if pid:
    try:
        result["process_executable"] = os.readlink("/proc/%d/exe" % pid)
        result["process_cwd"] = os.readlink("/proc/%d/cwd" % pid)
        raw = pathlib.Path("/proc/%d/environ" % pid).read_bytes()
        live = {}
        for entry in raw.split(b"\0"):
            key, sep, value = entry.partition(b"=")
            if key.decode(errors="replace") in public_names and sep:
                live[key.decode()] = value.decode(errors="replace")
        result["effective_public_settings"] = live
    except OSError as exc:
        result["process_error"] = str(exc)
try:
    binary = pathlib.Path("/usr/local/bin/hashburst-node")
    with binary.open("rb") as f:
        before = os.fstat(f.fileno())
        if not stat.S_ISREG(before.st_mode) or before.st_size > 128 * 1024 * 1024:
            raise RuntimeError("Unexpected binary type or size")
        digest = hashlib.file_digest(f, "sha256") if hasattr(hashlib, "file_digest") else None
        if digest is None:
            digest = hashlib.sha256()
            for chunk in iter(lambda: f.read(1024 * 1024), b""):
                digest.update(chunk)
        after = os.fstat(f.fileno())
        if (before.st_size, before.st_mtime_ns) != (after.st_size, after.st_mtime_ns):
            raise RuntimeError("Binary changed during read")
    result["installed_binary_sha256"] = digest.hexdigest()
except Exception as exc:
    result["binary_error"] = str(exc)
# A failed service has no effective environment; retain that uncertainty explicitly.
storage = result.get("effective_public_settings", {}).get("STORAGE_DIR")
result["ledger_directory_source"] = "process_environment" if storage else "declared_or_default_candidate"
storage = storage or settings.get("STORAGE_DIR", "/var/lib/hashburst")
result["storage_dir"] = storage
result["files"] = {}
try:
    if not pathlib.Path(storage).is_absolute() or "$" in storage:
        raise RuntimeError("Cannot safely resolve declared storage directory")
    names = ("blockchain.dat", "blockchain.idx")
    first = {n: read_public(str(pathlib.Path(storage) / n)) for n in names}
    second = {n: read_public(str(pathlib.Path(storage) / n)) for n in names}
    if first != second:
        raise RuntimeError("Ledger changed between reads; no pair exported")
    result["files"] = {n: {"bytes": len(d), "sha256": hashlib.sha256(d).hexdigest(),
        "base64": base64.b64encode(d).decode()} for n, (d, _) in first.items()}
    result["stable_pair_captured"] = True
except Exception as exc:
    result["ledger_error"] = str(exc)
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
for name in ("health", "blocks"):
    try:
        with opener.open("http://127.0.0.1:8009/api/" + name, timeout=10) as r:
            data = r.read(1048577)
        if len(data) > 1048576:
            raise RuntimeError("API response exceeds 1 MiB")
        result[name] = json.loads(data)
    except Exception as exc:
        result[name] = {"error": str(exc)}
result["service_after"] = service()
print(json.dumps(result))
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=pathlib.Path)
    args = parser.parse_args()
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    path = args.output or pathlib.Path("legacy-repair-inspection-" + stamp + ".json")
    # Reserve a new local report before opening SSH. Never overwrite prior evidence.
    with path.open("x", encoding="utf-8") as out:
        report = {"created_at": stamp, "read_only": True, "hosts": {}}
        def save():
            out.seek(0)
            json.dump(report, out, indent=2)
            out.write("\n")
            out.truncate()
            out.flush()
        save()
        for host in ("77.90.188.155", "77.90.188.157"):
            print("READ_ONLY=" + host, flush=True)
            try:
                p = subprocess.run(["ssh", "-o", "ControlMaster=no", "-o", "ControlPath=none",
                    "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=15", "-o",
                    "ServerAliveCountMax=3", "-o", "PubkeyAuthentication=no", "-o",
                    "PreferredAuthentications=keyboard-interactive,password", "root@" + host,
                    "python3 -"], input=REMOTE, text=True, stdout=subprocess.PIPE, timeout=180)
                report["hosts"][host] = json.loads(p.stdout) if p.returncode == 0 else {"ssh_exit": p.returncode}
            except Exception as exc:
                report["hosts"][host] = {"error": str(exc)}
            save()
    print("REPORT=" + str(path.resolve()))
    print("NO_SERVICE_OR_LEDGER_CHANGED")


if __name__ == "__main__":
    main()
