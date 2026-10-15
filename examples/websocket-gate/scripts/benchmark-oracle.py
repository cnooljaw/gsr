"""用固定官方 C/Lua 编码器生成或检查 Benchmark 的 Sproto wire 签名。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

example = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--output", type=Path, help="输出独立签名证据；省略时只检查夹具")
args = parser.parse_args()
oracle = Path(os.environ["SPROTO_ORACLE_DIR"]).resolve()
fixtures = json.loads((example / "benchmarks/representative.json").read_text())
source = json.loads((example / "benchmarks/oracle-source.json").read_text())
for name, digest in source["files"].items():
    actual = hashlib.sha256((oracle / name).read_bytes()).hexdigest()
    if actual != digest:
        raise ValueError(f"oracle source differs from {source['revision']}: {name}")


def lua_literal(value):
    if isinstance(value, dict):
        return "{" + ",".join("[" + lua_literal(k) + "]=" + lua_literal(v) for k, v in value.items()) + "}"
    if isinstance(value, list):
        return "{" + ",".join(lua_literal(v) for v in value) + "}"
    return json.dumps(value, ensure_ascii=False)


schema = (example / "benchmarks/schema/representative.sproto").read_text()
script = "local sproto=require 'sproto'\nlocal sp=sproto.parse(" + lua_literal(schema) + ")\n"
script += "local function hex(s) return (s:gsub('.',function(c) return string.format('%02x',string.byte(c)) end)) end\n"
for fixture in fixtures:
    script += "do local raw=sp:encode(" + lua_literal(fixture["type"]) + "," + lua_literal(fixture["data"]) + ")\n"
    script += "print(" + lua_literal(fixture["name"]) + ",hex(raw),hex(sproto.pack(raw))) end\n"
environment = {**os.environ, "LUA_PATH": str(oracle / "?.lua") + ";;"}
result = subprocess.run([os.environ.get("LUA", "lua"), "-"], input=script, text=True,
                        cwd=oracle, env=environment, capture_output=True, check=True)
signatures = {}
for line in result.stdout.splitlines():
    name, raw, packed = line.split()
    signatures[name] = {}
    for key, wire in [("sproto_encode", raw), ("sproto_pack", packed)]:
        content = bytes.fromhex(wire)
        signatures[name][key] = {"bytes": len(content), "sha256": hashlib.sha256(content).hexdigest()}
if args.output:
    args.output.write_text(json.dumps(signatures, indent=2) + "\n")
else:
    for fixture in fixtures:
        for key, signature in signatures[fixture["name"]].items():
            assert fixture["wire"][key] == signature, (fixture["name"], key)
    print(f"官方 C/Lua wire 对照通过：{len(fixtures)} 组")
