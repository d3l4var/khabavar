#!/usr/bin/env python3
"""Hammer the two places where a duplicate request costs money.

1) N parallel POST /orders with the SAME Idempotency-Key. Exactly one order must exist.
2) N parallel GET /payments/callback with the same authority. The order must end PAID once,
   and the repeat calls must not error or double-confirm.

Usage: python3 scripts/concurrency-check.py [base_url]
"""
import json
import urllib.error
import sys
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8090"
PRODUCT = "3b8efdfb-994a-471b-8fe9-636247eff2ef"
VARIANT = "6b8d24fc-869f-48f2-92b7-eee637b20bb3"
N = 8


def post(path, body, headers=None):
    req = urllib.request.Request(
        BASE + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", **(headers or {})},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")


def get(path):
    req = urllib.request.Request(BASE + path, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")


def main():
    # A fresh key per run: reusing one replays the stored response, which is correct behaviour
    # but tests nothing about this run.
    key = f"concurrency-check-{int(time.time() * 1000)}"
    body = {
        "guest_phone": "09120000999",
        "shipping_address": {"city": "تهران"},
        "items": [{
            "product_id": PRODUCT, "variant_id": VARIANT, "quantity": 1,
            "sku_snapshot": "SKU-SMOKE-1", "title_snapshot": "همزمانی",
            "unit_price": 25000, "supply_type": "OWNED_INVENTORY",
        }],
    }

    print(f"1) {N} parallel POST /orders with one Idempotency-Key")
    with ThreadPoolExecutor(max_workers=N) as pool:
        results = list(pool.map(lambda _: post("/api/v1/orders", body, {"Idempotency-Key": key}), range(N)))

    codes = sorted({c for c, _ in results})
    ids = {b.get("id") for _, b in results if b.get("id")}
    print(f"   statuses: {codes}")
    print(f"   distinct order ids: {len(ids)} -> {sorted(ids)}")
    if len(ids) != 1:
        print("   FAIL: more than one order was created for a single idempotency key")
        return 1

    order_id = ids.pop()

    print(f"\n2) opening a payment intent for {order_id}")
    st, intent = post(f"/api/v1/orders/{order_id}/payment-intents", {})
    print(f"   {st} {intent}")
    if st != 201:
        print("   FAIL: could not open an intent")
        return 1
    redirect = intent.get("redirect_url", "")
    authority = ""
    if "authority=" in redirect:
        authority = redirect.split("authority=")[1].split("&")[0]
    if not authority:
        print("   FAIL: no authority in the redirect")
        return 1

    print(f"\n3) {N} parallel callbacks for the same authority")
    with ThreadPoolExecutor(max_workers=N) as pool:
        cbs = list(pool.map(lambda _: get(f"/api/v1/payments/callback?Authority={authority}&Status=OK"), range(N)))

    cb_codes = sorted({c for c, _ in cbs})
    print(f"   statuses: {cb_codes}")
    bad = [c for c in cb_codes if c >= 500]
    if bad:
        print(f"   FAIL: server errors during duplicate callbacks: {bad}")
        return 1

    # A guest order is readable only by the phone that placed it, so the read carries it.
    st, order = get(f"/api/v1/orders/{order_id}?phone=09120000999")
    status = order.get("status") or order.get("data", {}).get("status")
    print(f"\n4) final order status: {status}")
    if status != "PAID":
        print(f"   FAIL: order is {status}, want PAID")
        return 1

    print("\nCONCURRENCY: clean")
    return 0


if __name__ == "__main__":
    sys.exit(main())
