"""Configuration types for relay."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class RelayConfig:
    """Runtime configuration for the relay proxy."""

    origin: str
    port: int
