---
title: Secure Access on GetEndpoint and Signed Endpoint
authors:
  - "@Pangjiping"
creation-date: 2026-04-19
last-updated: 2026-04-21
status: implementing
---

# OSEP-0011: Secure Access on GetEndpoint and Signed Endpoint

## Summary

Optional `secure_access` on sandbox create. There are **two** complementary mechanisms:

1. **Static header authorization (from `GetEndpoint`)** — when `secure_access` is enabled, **`GetEndpoint`** returns a stable opaque **`SecureAccessToken`**. Clients attach it to **all subsequent requests** as  
   **`OPENSANDBOX-SECURE-ACCESS: <token>`**  
   Ingress evaluates this header **before** route-signature verification, with **fail-fast** semantics when the header field is **present** but wrong (see § *Ingress verification*).

2. **Route `signature` (short route token)** — a **9-character** value embedded in host / header / path: **`hex8`** (8 lowercase hex) + **`signed_key_id`** (**exactly 1** char **`[0-9a-z]`**).  
   **Every signed route also carries an `expires` value:** Unix epoch seconds as **`uint64`**, encoded for routing and signing as **`expires_b36`**: **base-36** using **lowercase** digits **`0-9`** and letters **`a-z`**, **no leading zeros** (except **`expires_sec == 0`** is **`0`**). Equivalently (Go): **`strconv.FormatUint(expires_sec, 36)`** / **`strconv.ParseUint(s, 36, 64)`**. It appears in **`canonical_bytes`** and as its **own** `-`-delimited segment: **`{sandbox_id}-{port}-{expires_b36}-{signature}`**.  
   **Minting** always requires an **`expires`** input (see API). Ingress enforces **`now ≤ expires_seconds`** after decoding.

There is **no** signing of app path or query, and **no** DNS parent domain in the signed material. The wildcard parent domain is **routing-only**.

## Static access token (`GetEndpoint`)

When the sandbox has **`secure_access` enabled**, **`GetEndpoint(sandboxId)`** (or equivalent lifecycle response) includes **`SecureAccessToken`**.

**Client rule:** for **every** follow-up request through the gateway:

```http
OPENSANDBOX-SECURE-ACCESS: <token>
```

**Ingress rule (secure sandbox):** define **header present** as: the **`OPENSANDBOX-SECURE-ACCESS`** field appears on the HTTP request (any value, including empty). Then:

- If **present** and the value **matches** **`SecureAccessToken`** (constant-time compare) → **allow**; route-signature verification is **not** required.  
- If **present** and the value **does not match** → **`401` immediately**; ingress **must not** fall through to route-signature verification (prevents “bad/stale header + valid signed URL” from being accepted).  
- If **absent** → ingress may authenticate using the route **`signature`** path (when provided and valid).

## `expires_b36` encoding

Let **`expires_sec`** be **`uint64`** Unix epoch seconds (UTC).

**`expires_b36`** is the **base-36** encoding of **`expires_sec`** using **lowercase** alphabet **`0-9a-z`**, with **no leading zeros**, except **`expires_sec == 0`** is encoded as **`0`**. Normative reference (Go): **`strconv.FormatUint(expires_sec, 36)`** for minting and **`strconv.ParseUint(segment, 36, 64)`** for ingress.

- **Length:** **1** to **13** characters inclusive for any **`uint64`** value (max is **`18446744073709551615`** → **`3w5e11264sgsf`**).  
- **Charset:** **`[0-9a-z]`** only; reject uppercase.  
- **Routing segment** and **`canonical_bytes`** embed the **same** literal string (not decimal seconds).  
- **Ingress:** reject empty, invalid charset, overflow on parse, or length **> 13** → **`400`**. Then **`401`** if **`now > expires_sec`**.

> **Rationale:** Base36 is shorter than decimal for typical timestamps (e.g. **`2000000000`** → **`x2qxvk`**, 6 chars) while staying URL/host friendly without extra escaping.

## Signing algorithm (signed routes **always** include `expires_b36`)

### Inputs and constraints

- **`sandbox_id`**: verbatim in canonical (may contain `-`).
- **`port`**: decimal **`1..65535`**, **no leading zeros**.
- **`expires_b36`**: **required** for any minted signed route; rules above.
- **`secret_bytes`**: raw decoded secret for **`signed_key_id`** (see config: **`key_id`** is **1** char **`[0-9a-z]`**).

### `canonical_bytes` (UTF-8)

Always (note: **`{expires_b36}`** is base36, **not** decimal):

```text
v1\nshort\n{sandbox_id}\n{port}\n{expires_b36}\n
```

### `inner` and `signature`

`BE32(x)` = 4-byte big-endian uint32.

```text
inner     = BE32(len(secret_bytes)) || secret_bytes || BE32(len(canonical_bytes)) || canonical_bytes
digest    = SHA256(inner)
hex_all   = lowercase_hex(digest)
hex8      = hex_all[0:8]
signature = hex8 + signed_key_id       // 9 chars total
```

### Routing token (always four logical segments for **signed** routes)

```text
{sandbox-id}-{port}-{expires_b36}-{signature}
```

**Right-to-left parse:**

1. **Last**: **`signature`** (**`[0-9a-f]{8}[0-9a-z]{1}`** — exactly **9** characters).
2. **Second-to-last**: **`expires_b36`** (**`[0-9a-z]{1,13}`**, decode with **base 36** to **`uint64`**).
3. **Third-to-last**: **`port`** (decimal, rules above).
4. **Remaining** (joined with `-`): **`sandbox_id`**.

**Unsigned legacy (no route signature):** **`{sandbox_id}-{port}`** — two segments only.

## API

- **CreateSandbox:** `secure_access.enabled` (default `false`).
- **`GetEndpoint(sandboxId)`:** when secure access is on, includes **`SecureAccessToken`** for **`OPENSANDBOX-SECURE-ACCESS`**.
- **Mint signed URL / host token (all require expiry input):**
  - **`GET /sandboxes/{sandboxId}/endpoints/secure/{port}?expires=<unix_seconds>`** (and/or **`GetSignedEndpoint`** with the same query).  
  - **`expires`** query is a **decimal `uint64`** Unix second (human-friendly). The server **normalizes** to **`expires_b36`** (rules above) for both **`canonical_bytes`** and the returned routing token.  
  - Missing **`expires`** on mint → **`400`**.

Returned signed routing material always uses **`{sandbox_id}-{port}-{expires_b36}-{signature}`**.

## Gateway routing

### Host / header token (split on `-` from the **right**)

- **Signed:** **`{sandbox_id}-{port}-{expires_b36}-{signature}`**.
- **Unsigned legacy:** **`{sandbox_id}-{port}`**.

| Mode | Where | Example (illustrative) |
|------|-------|-------------------------|
| **Wildcard** | Host: `{sandbox_id}-{port}-{expires_b36}-{signature}.<parent>` | `my-sandbox-8080-x2qxvk-aabbccddk.sandbox.example.com` — **`expires_b36`** = **`x2qxvk`** (**`2000000000`** sec, Go **`FormatUint(..., 36)`**); **`signature`** = **`aabbccddk`**; parent = **`sandbox.example.com`**. |
| **Header** | Value: same `-`-joined token | `my-sandbox-8080-x2qxvk-aabbccddk` |
| **URI** | Prefix: `/{sandbox_id}/{port}/{expires_b36}/{signature}/` + upstream remainder | `/my-sandbox/8080/x2qxvk/aabbccddk/v1/status` — upstream after strip: **`/v1/status`**. |

### URI parsing

**Secure sandboxes (secure access required):**

- If segments 2–4 are syntactically valid **`port`**, **`expires_b36`**, **`signature`**, treat the path as **signed OSEP**: strip **`/{sandbox_id}/{port}/{expires_b36}/{signature}`** and forward the remainder + query unchanged.

**Unsecured sandboxes (secure access not required) — legacy safeguard:**

- Even when segments 2–4 **happen to match** the **`expires_b36`** / **`signature`** charset and length rules, ingress **must not** treat them as a signed routing prefix for forwarding purposes.
- Instead, **re-parse the full path using legacy URI rules** (first segment = **`sandbox_id`**, second = **`port`**, **everything after** is the upstream path, including any segments that looked like **`expires_b36`** / **`signature`**). This preserves existing **unsigned** apps whose paths could collide with the signed shape and avoids silently rewriting upstream paths.

**How to decide:** after resolving **`sandbox_id`** from the first path segment, consult **`GetEndpoint` / secure-access policy**. Apply the **signed OSEP** strip **only** when the sandbox **requires** secure access; otherwise apply **legacy** parsing for URI mode.

**Legacy unsigned (always):** `/{sandbox_id}/{port}/…` when the path is not using the signed prefix **or** when legacy re-parse is mandated above.

Strip the signed prefix **only** on the secure path; forward path + query unchanged relative to the chosen interpretation.

## Ingress verification

1. **Parse routing input** (mode-dependent). For **URI** mode, a path may **syntactically** match **`/{sandbox_id}/{port}/{expires_b36}/{signature}/…`**; still resolve **`sandbox_id`** (at minimum the first segment) for lookup.
2. **`GetEndpoint(sandbox_id)`** once: secure-access flag, **`SecureAccessToken`**, and backend endpoint.
3. **URI mode + secure access not required:** **re-parse the full path using legacy URI rules** for **`sandbox_id`**, **`port`**, and upstream **`requestURI`** (§ *URI parsing* / unsecured safeguard). **Do not** strip **`expires_b36`** / **`signature`**-shaped segments from the forwarded path.
4. **Secure access required** (final signed interpretation for URI / host / header):
   - **Header branch:** if **`OPENSANDBOX-SECURE-ACCESS`** is **present** (see § *Static access token*): **match** → **allow**; **mismatch** → **`401`** (no route-signature fallback).
   - **Signature branch:** if the header is **absent** and a signed route token is present: decode **`expires_sec`** from **`expires_b36`**, require **`now ≤ expires_sec`**, rebuild **`canonical_bytes`** with the **same** **`expires_b36`**, verify **`signature`** → **`401`** on failure; if no signed credential → **`401`**.
5. **Secure access not required** (URI after step 3 legacy re-parse, or unsigned host/header shapes): **allow** without route-signature verification.

## Config

**Server (`~/.sandbox.toml`):**

```toml
[ingress.secure_access]
enabled = true
active_key = "a"                    # 1 char [0-9a-z], must exist in keys

[[ingress.secure_access.keys]]
key_id = "a"
secret = "base64:..."

[[ingress.secure_access.keys]]
key_id = "b"
secret = "base64:..."
```

**Ingress:** `--secure-access-keys` uses the same **1-character** `key_id` per segment, e.g. **`a=base64:...,b=base64:...`**.

```bash
opensandbox-ingress --secure-access-enabled \
  --secure-access-keys "a=base64:...,b=base64:..."
```

## Errors

- **`400`:** missing **`expires`** on mint, malformed token, invalid **`expires_b36`** (empty / bad charset / length **> 13** / parse overflow), bad **`port`** / **`signature`**.
- **`401`:** header mismatch, **`now > expires_sec`**, bad **`hex8`**, unknown key, missing credential when required.

## Tests

- Unit: `inner` / `hex8`, four-part right split with `-` in **`sandbox_id`**, **`expires_b36`** canonicalization (no leading zeros, **`0`** case, round-trip **`ParseUint(..., 36, 64)`**).
- Integration: three modes; invalid **`expires_b36`** → **`400`**; past expiry → **`401`**; mint without **`expires`** → **`400`**.
