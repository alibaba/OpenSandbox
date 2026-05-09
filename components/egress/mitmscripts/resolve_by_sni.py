"""mitmproxy addon: use SNI hostname for upstream connection in transparent mode.

In transparent mode the client resolves DNS and connects to an IP.
iptables redirects the TCP stream to mitmproxy, which sees the original
destination IP and by default uses that IP for upstream TLS verification.
Server certificates rarely include the IP in SAN, causing:

    Certificate verify failed: IP address mismatch

This addon rewrites the server address to the SNI hostname (when present
and not itself an IP), so mitmproxy:
- resolves DNS for the hostname
- connects to the resolved address
- verifies the certificate against the hostname
"""

import re

from mitmproxy import ctx

_IP_RE = re.compile(r"^[\d.]+$|^[\da-fA-F:]+$")


def _is_ip(host: str) -> bool:
    return bool(_IP_RE.match(host))


class ResolveBySNI:
    def server_connect(self, data):
        addr = data.server.address
        if addr is None:
            return

        host = addr[0] if isinstance(addr, (tuple, list)) else str(addr)
        if not _is_ip(host):
            return

        sni = getattr(data.server, "sni", None)
        if not sni or _is_ip(sni):
            return

        port = addr[1] if isinstance(addr, (tuple, list)) and len(addr) > 1 else 443
        ctx.log(f"[resolve_by_sni] {host}:{port} -> {sni}:{port}")
        data.server.address = (sni, port)


addons = [ResolveBySNI()]
