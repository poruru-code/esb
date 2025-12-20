import sys
import os

# Ensure root is in path
sys.path.append(os.path.abspath("."))

try:
    print("Success")
except Exception as e:
    print(f"Error: {e}")
    import traceback

    traceback.print_exc()
