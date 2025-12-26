import requests
import uuid
import time
import json
from datetime import datetime, timezone
from tests.conftest import GATEWAY_URL, VERIFY_SSL, query_victorialogs


class TestTrace:
    def test_chained_trace_consistency(self, auth_token):
        """
        E2E: Client -> Gateway -> Lambda A -> Lambda B で Trace ID が維持されるか

        検証ポイント:
        1. Lambda A が返す trace_id が送信した Trace ID と一致するか
        2. テスト開始時刻以降のログで、Trace ID が全ての通過コンポーネントに出現するか
           - esb-gateway
           - lambda-integration
           - lambda-connectivity
        """
        # テスト開始時刻を記録
        test_start_time = datetime.now(timezone.utc)

        # AWS 互換 Trace ID の生成
        epoch_hex = hex(int(time.time()))[2:]
        unique_id = uuid.uuid4().hex[:24]
        custom_trace_id = f"Root=1-{epoch_hex}-{unique_id};Sampled=1"
        root_id = f"1-{epoch_hex}-{unique_id}"

        # 1. Lambda A を呼び出し、内部で Lambda B (connectivity) を呼び出させる
        payload = {"next_target": "lambda-connectivity"}

        response = requests.post(
            f"{GATEWAY_URL}/2015-03-31/functions/lambda-integration/invocations",
            json=payload,
            headers={"Authorization": f"Bearer {auth_token}", "X-Amzn-Trace-Id": custom_trace_id},
            verify=VERIFY_SSL,
            timeout=30,
        )

        assert response.status_code == 200, f"Request failed with status {response.status_code}"

        data = response.json()
        body = json.loads(data.get("body", "{}"))

        # --- 検証 1: Lambda A のレスポンス内 trace_id ---
        lambda_a_trace_id = body.get("trace_id")
        assert lambda_a_trace_id is not None, "Lambda A did not return trace_id in response"
        assert lambda_a_trace_id != "not-found", (
            "Lambda A failed to receive Trace ID. Got 'not-found'. "
            "Expected Trace ID to be propagated via X-Amz-Client-Context header."
        )

        # Root 部分が一致するか確認
        expected_root = f"Root={root_id}"
        assert expected_root in lambda_a_trace_id, (
            f"Lambda A received wrong Trace ID. "
            f"Expected root: {expected_root}, Got: {lambda_a_trace_id}"
        )

        # 連鎖呼び出しの child 情報が存在するか
        child_info = body.get("child")
        assert child_info is not None, "Lambda A did not return child (Lambda B) info"

        print(f"[OK] Lambda A trace_id: {lambda_a_trace_id}")
        print("[OK] Lambda B (child) response received")

        # --- 検証 2: VictoriaLogs で各コンポーネントでの Trace ID 出現を確認 ---
        # ログ到達待ち (最大 45秒)
        # Note: query_victorialogs は min_hits=1 で戻るため、Gateway ログが見つかった時点で
        # Lambda ログがまだでも返ってきてしまう。そのため、ループで全コンポーネントが揃うのを待つ。

        expected_components = {"esb-gateway", "lambda-integration", "lambda-connectivity"}
        found_components = set()

        wait_start = time.time()
        wait_timeout = 45

        print(f"Waiting for logs from: {expected_components} (Timeout: {wait_timeout}s)")

        while time.time() - wait_start < wait_timeout:
            # テスト開始時刻を ISO8601 形式に変換して VictoriaLogs クエリに渡す
            start_time_iso = test_start_time.strftime("%Y-%m-%dT%H:%M:%SZ")
            logs = query_victorialogs(root_id, timeout=1, start=start_time_iso)
            hits = logs.get("hits", [])

            current_found = set()
            for log in hits:
                container_name = log.get("container_name", "")
                stream = log.get("_stream", "")

                for component in expected_components:
                    if component in container_name or f'container_name="{component}"' in stream:
                        current_found.add(component)

            found_components = current_found
            missing = expected_components - found_components

            if not missing:
                break

            time.sleep(2)

        print(f"Found {len(hits)} logs for Trace ID root: {root_id}")
        print(f"Components with Trace ID: {found_components}")

        missing_components = expected_components - found_components
        if missing_components:
            # 厳格な検証: 全コンポーネントで Trace ID が見つからなければエラー
            print(f"DEBUG Logs found: {json.dumps(logs, indent=2)}", flush=True)
            raise AssertionError(
                f"[FAILED] Trace ID did not appear in VictoriaLogs for: {missing_components}. "
                f"Found in: {found_components}. "
                f"Ensure sitecustomize.py is auto-hydrating trace IDs and logs are shipped."
            )
        else:
            print(f"[OK] Trace ID propagated to all expected components: {found_components}")
