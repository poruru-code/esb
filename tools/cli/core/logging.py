import sys


# ANSI Escape Codes for Colors
class Color:
    BLUE = "\033[94m"
    GREEN = "\033[92m"
    YELLOW = "\033[93m"
    RED = "\033[91m"
    CYAN = "\033[96m"
    BOLD = "\033[1m"
    UNDERLINE = "\033[4m"
    END = "\033[0m"


def info(msg: str):
    print(f"{Color.CYAN}ℹ {msg}{Color.END}")


def success(msg: str):
    print(f"{Color.GREEN}✅ {msg}{Color.END}")


def warning(msg: str):
    print(f"{Color.YELLOW}⚠️ {msg}{Color.END}")


def error(msg: str):
    print(f"{Color.RED}❌ {msg}{Color.END}", file=sys.stderr)


def highlight(msg: str) -> str:
    return f"{Color.BOLD}{msg}{Color.END}"


def step(msg: str):
    print(f"{Color.BLUE}➜ {Color.BOLD}{msg}{Color.END}")
