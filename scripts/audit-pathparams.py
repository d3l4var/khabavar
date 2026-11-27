#!/usr/bin/env python3
"""Cross-check route path parameters against the handlers that read them.

Why this exists: the refund path and then four catalog handlers read PathValue("id") while the
route table declared {orderNumber}, {productId} and {variantId}. PathValue returned "" and every
one of those endpoints answered 404 or 500 on a real id. Nothing in the Go tests caught it,
because a handler test calls the function directly instead of going through ServeMux.

This walks the mount calls in cmd/khabavar, resolves each handler to its source, and reports any
route whose {params} are not the ones the handler reads.

Usage:
  python3 scripts/audit-pathparams.py            # report
  python3 scripts/audit-pathparams.py --fail     # exit 1 when anything is wrong
"""
import glob
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CMD_GLOB = os.path.join(ROOT, "cmd", "khabavar", "*.go")
MODULE_GLOB = os.path.join(ROOT, "internal", "modules", "*", "*.go")

# a.public("GET /api/v1/x/{id}", catalog.Handler(svc))
MOUNT_RE = re.compile(
    r'\.(public|publicCustomer|customer|admin)\(\s*"([A-Z]+ [^"]+)"\s*,\s*([^\n]+)'
)
PARAM_RE = re.compile(r"\{([a-zA-Z][a-zA-Z0-9]*)(?:\.\.\.)?\}")
PATHVALUE_RE = re.compile(r'PathValue\("([^"]+)"\)')
# pathID(r, "productId", "variantId", "id") reads whichever of these the route declared.
PATHID_RE = re.compile(r'pathID\([^)]*?\)')
FUNC_RE = re.compile(r"^func (?:\([^)]*\) )?([A-Za-z][A-Za-z0-9_]*)\s*\(", re.M)


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


def handler_functions():
    """Map exported function name -> body text, across every module package."""
    funcs = {}
    for path in glob.glob(MODULE_GLOB):
        if path.endswith("_test.go"):
            continue
        src = read(path)
        matches = list(FUNC_RE.finditer(src))
        for i, m in enumerate(matches):
            end = matches[i + 1].start() if i + 1 < len(matches) else len(src)
            funcs.setdefault(m.group(1), src[m.start():end])
    return funcs


def mounts():
    """(pattern, handler expression, mount kind) for every declared route."""
    out = []
    for path in glob.glob(CMD_GLOB):
        if path.endswith("_test.go"):
            continue
        for m in MOUNT_RE.finditer(read(path)):
            out.append((m.group(2), m.group(3).strip(), m.group(1)))
    return out


def handler_name(expr):
    """Resolve the handler a mount actually wraps.

    catalog.AdminProductDetailHandler(svc)                     -> AdminProductDetailHandler
    commerce.Idempotent(s.idem, "x", commerce.FooHandler(s))   -> FooHandler
    A middleware wrapper is not the handler; the innermost *Handler is.
    """
    handlers = re.findall(r"([A-Z][A-Za-z0-9_]*Handler)\b", expr)
    if handlers:
        return handlers[-1]
    m = re.search(r"([A-Z][A-Za-z0-9_]*)\s*\(", expr)
    if m:
        return m.group(1)
    m = re.search(r"([A-Z][A-Za-z0-9_]*)$", expr.strip())
    return m.group(1) if m else ""


def main():
    fail = "--fail" in sys.argv
    funcs = handler_functions()
    problems = []
    checked = 0

    for pattern, expr, kind in mounts():
        params = PARAM_RE.findall(pattern)
        if not params:
            continue
        name = handler_name(expr)
        body = funcs.get(name)
        if body is None:
            problems.append((pattern, name, "handler source not found"))
            continue
        # A handler often delegates: Idempotent(...) wraps it, or it calls a shared helper that
        # reads the parameter. Follow one level of calls before calling it a mismatch.
        reads = set(PATHVALUE_RE.findall(body))
        for call in PATHID_RE.findall(body):
            reads |= set(re.findall(r'"([^"]+)"', call))
        for called in set(re.findall(r"\b([A-Za-z][A-Za-z0-9_]*)\s*\(", body)):
            if called == name:
                continue
            sub = funcs.get(called)
            if sub:
                reads |= set(PATHVALUE_RE.findall(sub))
                for call in PATHID_RE.findall(sub):
                    reads |= set(re.findall(r'"([^"]+)"', call))
        checked += 1
        if not reads:
            problems.append((pattern, name, f"handler reads no PathValue; route declares {params}"))
            continue
        missing = [p for p in params if p not in reads]
        if missing:
            problems.append(
                (pattern, name, f"route declares {missing}, handler reads {sorted(reads)}")
            )

    print(f"routes with params: {checked}")
    if not problems:
        print("PATH PARAMS: clean")
        return 0

    print(f"\nMISMATCHED PATH PARAMS ({len(problems)}):")
    for pattern, name, why in problems:
        print(f"  {pattern}")
        print(f"    handler {name}: {why}")
    return 1 if fail else 0


if __name__ == "__main__":
    sys.exit(main())
