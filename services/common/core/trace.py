import time
import secrets
from typing import Optional


class TraceId:
    """
    AWS X-Ray Trace ID format:
    Root=1-timestamp-randomuuid;Parent=parentid;Sampled=sampled
    """

    def __init__(self, root: str, parent: Optional[str] = None, sampled: str = "1"):
        self.root = root
        self.parent = parent
        self.sampled = sampled

    @classmethod
    def generate(cls) -> "TraceId":
        """Generate a new Trace ID (Root=1-timehex-uniqueid)."""
        # AWS compliant: 8-digit hex timestamp.
        epoch_hex = f"{int(time.time()):08x}"
        unique_id = secrets.token_hex(12)  # 24 chars
        root = f"1-{epoch_hex}-{unique_id}"
        return cls(root=root, sampled="1")

    @classmethod
    def parse(cls, header: str) -> "TraceId":
        """Parse an X-Amzn-Trace-Id header string."""
        # print(f"[TraceId] Parsing header: '{header}'")
        parts = {}
        for part in header.split(";"):
            if "=" in part:
                try:
                    k, v = part.split("=", 1)
                    parts[k.strip()] = v.strip()
                except ValueError:
                    continue

        root = parts.get("Root", "")
        parent = parts.get("Parent")
        sampled = parts.get("Sampled", "1")

        # Fallback if a raw ID is provided without Root= format.
        if not root and header and "-" in header and "=" not in header:
            # print(f"[TraceId] Header looks like a raw ID, using it as root")
            root = header.strip()

        # print(f"[TraceId] Parsed: root='{root}', parent='{parent}', sampled='{sampled}'")
        return cls(root=root, parent=parent, sampled=sampled)

    def to_root_id(self) -> str:
        """Return only the Root ID (used as Request ID)."""
        return self.root

    def __str__(self) -> str:
        """Generate the header-formatted string."""
        s = f"Root={self.root}"
        if self.parent:
            s += f";Parent={self.parent}"
        if self.sampled:
            s += f";Sampled={self.sampled}"
        return s
