import requests
import json
from datetime import datetime, timedelta

VICTORIALOGS_URL = "http://localhost:9428"


def query_logs(query, limit=50):
    try:
        # Fetch logs from the last 10 minutes.
        start = (datetime.utcnow() - timedelta(minutes=10)).strftime("%Y-%m-%dT%H:%M:%SZ")
        url = f"{VICTORIALOGS_URL}/select/logsql/query"
        params = {"query": query, "limit": limit, "start": start}
        response = requests.get(url, params=params)
        if response.status_code != 200:
            print(f"Error querying logs: {response.text}")
            return []

        # Parse JSONL response.
        logs = []
        for line in response.text.strip().split("\n"):
            if line:
                try:
                    logs.append(json.loads(line))
                except json.JSONDecodeError:
                    pass
        return logs
    except Exception as e:
        print(f"Exception: {e}")
        return []


print("=== Checking lambda-echo logs ===")
logs = query_logs('_stream:{container_name="lambda-echo"}')
print(f"Total hits: {len(logs)}")
for log in logs:
    # Display message and structured data separately for readability.
    msg = log.get("_msg", "") or log.get("message", "")
    trace_id_field = log.get("trace_id", "N/A")
    print(f"[{log.get('_time')}] {msg[:100]} | trace_id: {trace_id_field}")
    # Try parsing if JSON is embedded.
    if "{" in msg:
        try:
            # Simplified: from the first { to the last }.
            start = msg.find("{")
            end = msg.rfind("}") + 1
            json_part = json.loads(msg[start:end])
            if "trace_id" in json_part:
                print(f"  -> Found trace_id in JSON body: {json_part['trace_id']}")
        except Exception:
            pass
