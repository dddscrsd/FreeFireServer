import json, re, os
base = os.path.dirname(os.path.abspath(__file__))
ref = os.path.join(base, "..", "original_to_read", "htpp.py")
lines = open(ref, encoding="utf-8", errors="replace").read().splitlines()
# reference route -> first @app.route line number
routes = {}
for i, l in enumerate(lines):
    m = re.search(r'@app\.route\(["\']/([A-Za-z0-9]+)["\']', l)
    if m and m.group(1) not in routes:
        routes[m.group(1)] = i + 1
emap = json.load(open(os.path.join(base, "endpoint_map.json")))
cmd = json.load(open(os.path.join(base, "cmd_map.json")))
enum_names = set(cmd.values())
by_ep = {}
for e in emap:
    by_ep.setdefault(e["endpoint"], e)   # first occurrence
manifest = []
for name, ln in sorted(routes.items()):
    e = by_ep.get(name)
    if e:
        status = "port"               # has proto req/res types from the map
    elif name in enum_names:
        status = "port_resolve"       # exists in 1.70 enum; agent resolves proto types
    else:
        status = "skip"               # 1.43-only / infra route, not in 1.70
    manifest.append({"endpoint": name, "py_line": ln,
                     "reqType": (e or {}).get("reqType"),
                     "resType": (e or {}).get("resType"),
                     "status": status})
manifest = [m for m in manifest if m["status"] != "skip"]
json.dump(manifest, open(os.path.join(base, "porting_manifest.json"), "w"), indent=1)
from collections import Counter
c = Counter(m["status"] for m in manifest)
print(f"portable endpoints={len(manifest)}  {dict(c)}")
