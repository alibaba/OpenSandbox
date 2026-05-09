"""mitmproxy addon: use SNI hostname for upstream connection in transparent mode.

In transparent mode the client resolves DNS and connects to an IP.
iptables redirects the TCP stream to mitmproxy, which sees the original
destination IP and by default uses that IP for upstream TLS verification.
Server certificates rarely include the IP in SAN, causing:

    Certificate verify failed: IP address mismatch

This addon hooks server_connect and rewrites the server address + SNI to
the hostname from the TLS ClientHello SNI when it is a real hostname
(not an IP literal).  This requires connection_strategy=lazy and
upstream_cert=false so mitmproxy does not eagerly connect to the original
IP before the addon runs.
"""

import re

from mitmproxy import ctx

_IP_RE = re.compile(r"^[\d.]+$|^[\da-fA-F:]+$")


def _is_ip(host: str) -> bool:
    return bool(_IP_RE.match(host))


class ResolveBySNI:
    def load(self, _loader):
        ctx.log("[resolve_by_sni] addon loaded")

    def server_connect(self, data):
        server = data.server
        addr = server.address
        sni = server.sni

        ctx.log(f"[resolve_by_sni] server_connect: address={addr} sni={sni}")

        if addr is None:
            ctx.log("[resolve_by_sni] address is None, skip")
            return

        host = addr[0] if isinstance(addr, (tuple, list)) else str(addr)
        port = addr[1] if isinstance(addr, (tuple, list)) and len(addr) > 1 else 443

        if not _is_ip(host):
            ctx.log(f"[resolve_by_sni] host {host} is not IP, skip")
            return

        if not sni:
            ctx.log(f"[resolve_by_sni] no SNI, skip (host={host})")
            return

        if _is_ip(sni):
            ctx.log(
                f"[resolve_by_sni] SNI is also IP ({sni}), cannot resolve; skip"
            )
            return

        ctx.log(f"[resolve_by_sni] rewrite {host}:{port} -> {sni}:{port}")
        data.server.address = (sni, port)
        data.server.sni = sni


addons = [ResolveBySNI()]
