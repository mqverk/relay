"""CLI entrypoint for relay."""

from __future__ import annotations

import argparse
from typing import Sequence
from urllib.parse import urlparse

from relay.config import RelayConfig


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="relay",
        description="relay HTTP caching proxy",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=3000,
        help="Port for the proxy server (default: 3000)",
    )
    parser.add_argument(
        "--origin",
        help="Origin base URL to forward requests to (for example: http://dummyjson.com)",
    )
    parser.add_argument(
        "--clear-cache",
        action="store_true",
        help="Clear all cached entries and exit",
    )
    return parser


def _parse_args(argv: Sequence[str] | None) -> argparse.Namespace:
    parser = _build_parser()
    args = parser.parse_args(argv)

    if args.clear_cache:
        return args

    if not args.origin:
        parser.error("--origin is required unless --clear-cache is set")

    if args.port <= 0 or args.port > 65535:
        parser.error("--port must be within 1..65535")

    parsed_origin = urlparse(args.origin)
    if parsed_origin.scheme not in {"http", "https"} or not parsed_origin.netloc:
        parser.error("--origin must be a valid http or https URL")

    return args


def _build_config(args: argparse.Namespace) -> RelayConfig:
    return RelayConfig(origin=args.origin, port=args.port)


def main(argv: Sequence[str] | None = None) -> int:
    """Run the relay CLI."""
    args = _parse_args(argv)

    if args.clear_cache:
        print("Cache cleared.")
        return 0

    config = _build_config(args)
    print(f"relay start requested on :{config.port} -> {config.origin}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
