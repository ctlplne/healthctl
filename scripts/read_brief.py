"""Read one authorized brief from a local installation. A bearer token is entered privately."""

import argparse
import getpass
import ipaddress
import json
import sys
from urllib.parse import urlsplit
from urllib.request import Request, build_opener, HTTPRedirectHandler, ProxyHandler


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


def read_brief(base, token):
    u = urlsplit(base)
    if u.username or u.password or u.path not in ("", "/") or u.query or u.fragment:
        raise ValueError("Enter the application origin.")
    loopback = u.hostname == "localhost"
    if not loopback:
        try:
            loopback = ipaddress.ip_address(u.hostname or "").is_loopback
        except ValueError:
            pass
    if not loopback or u.scheme not in {"https", "http"}:
        raise ValueError(
            "The sample consumer reads from the operator's local installation."
        )
    request = Request(
        base.rstrip("/") + "/api/integration/brief",
        headers={"Authorization": "Bearer " + token, "Accept": "application/json"},
    )
    with build_opener(ProxyHandler({}), NoRedirect()).open(
        request, timeout=12
    ) as response:
        data = json.load(response)
    if data["format"] != "clinical-history/brief-1" or not data["confirmed"]:
        raise ValueError("Unexpected brief version or review state.")
    return data


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="http://127.0.0.1:8437")
    args = parser.parse_args()
    token = (
        getpass.getpass("Brief access token: ")
        if sys.stdin.isatty()
        else sys.stdin.readline().strip()
    )
    print(json.dumps(read_brief(args.base, token), ensure_ascii=False, indent=2))
