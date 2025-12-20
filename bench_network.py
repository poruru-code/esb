import socket
import time
import statistics


def bench(target_host, target_ip, port=8080, rounds=10):
    name_times = []
    ip_times = []

    print(f"Benchmarking connection to {target_host} ({target_ip}):{port}")

    for i in range(rounds):
        # Name resolution + connection
        start = time.perf_counter()
        try:
            with socket.create_connection((target_host, port), timeout=1):
                name_times.append(time.perf_counter() - start)
        except Exception as e:
            print(f"Name attempt {i} failed: {e}")

        # IP connection
        start = time.perf_counter()
        try:
            with socket.create_connection((target_ip, port), timeout=1):
                ip_times.append(time.perf_counter() - start)
        except Exception as e:
            print(f"IP attempt {i} failed: {e}")

    if name_times:
        print(
            f"Hostname avg: {statistics.mean(name_times) * 1000:.3f}ms (min: {min(name_times) * 1000:.3f}ms)"
        )
    if ip_times:
        print(
            f"IP address avg: {statistics.mean(ip_times) * 1000:.3f}ms (min: {min(ip_times) * 1000:.3f}ms)"
        )


if __name__ == "__main__":
    import sys

    if len(sys.argv) < 3:
        print("Usage: python bench.py <hostname> <ip> [port]")
    else:
        port = int(sys.argv[3]) if len(sys.argv) > 3 else 8080
        bench(sys.argv[1], sys.argv[2], port=port)
