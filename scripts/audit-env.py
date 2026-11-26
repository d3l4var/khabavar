#!/usr/bin/env python3
"""Compare the env vars the binary reads with the ones .env.example documents.

Why: a var that the code reads but nothing documents is a deploy-time surprise — the
service starts with a default nobody knew about, or fails at 3am. The reverse (a
documented var nothing reads) is dead configuration that misleads operators.

Usage:  python3 scripts/audit-env.py [--fail-on-missing]
"""
import re
import sys

CONFIG = 'internal/platform/config/config.go'
EXAMPLE = '.env.example'

code = open(CONFIG, encoding='utf-8').read()
read_vars = set(re.findall(r'getenv(?:Int)?\("([A-Z0-9_]+)"', code))
read_vars |= set(re.findall(r'os\.Getenv\("([A-Z0-9_]+)"\)', code))

doc = open(EXAMPLE, encoding='utf-8').read()
documented = set(re.findall(r'^([A-Z0-9_]+)=', doc, re.M))

# Per-unit examples: the units point at /etc/khabavar/<unit>.env, so each one must
# exist in the repo and only use names the binary actually reads.
import glob
unit_files = sorted(glob.glob('deploy/env/*.env.example'))
unit_vars = {}
for uf in unit_files:
    unit_vars[uf] = set(re.findall(r'^([A-Z0-9_]+)=', open(uf, encoding='utf-8').read(), re.M))

missing = sorted(read_vars - documented)
extra = sorted(documented - read_vars)

print(f"read by config.Load(): {len(read_vars)}")
print(f"documented in {EXAMPLE}: {len(documented)}")
if missing:
    print("\nREAD BUT NOT DOCUMENTED (add to .env.example):")
    for v in missing:
        print(f"  {v}")
if extra:
    print("\nDOCUMENTED BUT NEVER READ (dead config — remove or wire it):")
    for v in extra:
        print(f"  {v}")
print(f"\nper-unit examples: {len(unit_files)}")
for uf, names in unit_vars.items():
    unknown = sorted(names - read_vars)
    print(f"  {uf}: {len(names)} vars" + (f"  UNKNOWN: {unknown}" if unknown else ""))
    if unknown:
        missing.append(f"{uf}:{unknown}")

if not missing and not extra:
    print("\nenv surface is consistent")
sys.exit(1 if (missing and '--fail-on-missing' in sys.argv) else 0)
