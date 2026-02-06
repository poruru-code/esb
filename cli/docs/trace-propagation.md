<!--
Where: cli/docs/trace-propagation.md
What: Trace propagation via X-Amzn-Trace-Id and ClientContext.
Why: Explain runtime hooks and Gateway propagation logic.
-->
# X-Amzn-Trace-Id によるトレーシング

本ドキュメントでは、本基盤における分散トレーシングの実装について、コードレベルで解説します。

## 概要

AWS Lambda 本番環境では `X-Amzn-Trace-Id` ヘッダーが自動的に `_X_AMZN_TRACE_ID` 環境変数に変換されますが、Lambda RIE (Runtime Interface Emulator) ではこの機能がありません。本基盤では **ClientContext 経由のブリッジ機構**を実装してこの制限を回避しています。

## Trace ID フォーマット

AWS X-Ray 準拠のフォーマット:

```
Root=1-{timestamp:08x}-{unique_id:24hex};Sampled=1

例: Root=1-67687c5a-a1b2c3d4e5f67890abcdef01;Sampled=1
```

| フィールド | 説明 |
|-----------|------|
| `Root` | トレースのルート識別子 (`1-XXXXXXXX-XXXXXXXXXXXXXXXXXXXXXXXX`) |
| `Parent` | 親スパンID（オプション） |
| `Sampled` | サンプリングフラグ (`0` or `1`) |

**実装**: `services/common/core/trace.py`

## Request ID について

Gateway はリクエストごとにユニークな `Request ID` (UUID) を生成します。Trace ID とは別の識別子です。

- **クライアントへの返却**: レスポンスヘッダ `x-amzn-RequestId` に含まれます。
- **ログ**: Gateway の構造化ログには `aws_request_id` フィールドとして記録されます。
- **Lambda への伝播**:
    - `event.requestContext.requestId`: Gateway が生成した ID が格納されます。
    - `context.aws_request_id`: Lambda RIE が生成する実行 ID です（**Gateway の ID とは異なる場合があります**）。

## トレース伝播フロー

```mermaid
sequenceDiagram
    participant Client
    participant Gateway
    participant Agent as Agent (gRPC)
    participant LambdaA as Lambda A (RIE)
    participant LambdaB as Lambda B (RIE)

    Client->>Gateway: POST /api/xxx<br/>X-Amzn-Trace-Id: Root=1-xxx
    
    Note over Gateway: 1. Middleware でパース<br/>2. ContextVar に保存
    
    Gateway->>Agent: EnsureContainer (gRPC)
    Agent-->>Gateway: WorkerInfo (ip:port)
    
    Gateway->>LambdaA: POST /invocations<br/>X-Amzn-Trace-Id: Root=1-xxx<br/>X-Amz-Client-Context: base64({custom:{trace_id:...}})
    
    Note over LambdaA: sitecustomize.py が<br/>自動的に ClientContext から<br/>復元し _X_AMZN_TRACE_ID にセット
    
    LambdaA->>LambdaB: boto3.invoke()<br/>※sitecustomize.py が<br/>ClientContext に自動注入
    
    Note over LambdaB: 同様に trace_id を復元
    
    LambdaB-->>LambdaA: Response
    LambdaA-->>Gateway: Response
    Gateway-->>Client: Response<br/>X-Amzn-Trace-Id: Root=1-xxx
```

## コンポーネント詳細

### 1. Gateway Middleware

**ファイル**: `services/gateway/main.py`

```python
@app.middleware("http")
async def request_id_middleware(request: Request, call_next):
    # ヘッダーから Trace ID を取得または新規生成
    trace_id_str = request.headers.get("X-Amzn-Trace-Id")
    
    if trace_id_str:
        set_trace_id(trace_id_str)  # パースして ContextVar に保存
    else:
        trace = TraceId.generate()
        trace_id_str = str(trace)
        set_trace_id(trace_id_str)
    
    response = await call_next(request)
    
    # レスポンスヘッダーに付与
    response.headers["X-Amzn-Trace-Id"] = trace_id_str
    
    clear_trace_id()  # リクエスト終了時にクリア
    return response
```

**役割**:
- 受信した `X-Amzn-Trace-Id` ヘッダーをパース
- 存在しない場合は新規生成
- `ContextVar` に保存して後続処理で利用可能に
- レスポンスヘッダーにも付与

---

### 2. Request Context (ContextVar)

**ファイル**: `services/common/core/request_context.py`

```python
from contextvars import ContextVar

_trace_id_var: ContextVar[Optional[str]] = ContextVar("trace_id", default=None)

def get_trace_id() -> Optional[str]:
    """現在のリクエストの Trace ID を取得"""
    return _trace_id_var.get()

def set_trace_id(trace_id_str: str) -> str:
    """Trace ID をパースしてセット"""
    trace = TraceId.parse(trace_id_str)
    _trace_id_var.set(str(trace))
    return str(trace)
```

**役割**:
- `asyncio` 対応のスレッドローカル変数
- どこからでも `get_trace_id()` で現在の Trace ID を取得可能

---

### 3. Lambda Invoker (ClientContext 注入)

**ファイル**: `services/gateway/services/lambda_invoker.py`

```python
async def do_post():
    headers = {"Content-Type": "application/json"}
    
    if trace_id:
        # HTTP ヘッダーとして伝播
        headers["X-Amzn-Trace-Id"] = trace_id
        
        # RIE 対策: ClientContext に埋め込む
        client_context = {"custom": {"trace_id": trace_id}}
        json_ctx = json.dumps(client_context)
        b64_ctx = base64.b64encode(json_ctx.encode("utf-8")).decode("utf-8")
        headers["X-Amz-Client-Context"] = b64_ctx
```

**役割**:
- Lambda RIE へのリクエスト時に `X-Amzn-Trace-Id` ヘッダーを付与
- **RIE はこのヘッダーを無視するため**、`X-Amz-Client-Context` にも埋め込む
- ClientContext は Base64 エンコードされた JSON

---



### 4. sitecustomize.py (自動注入・自動復元)

**ファイル**: `runtime/python/extensions/sitecustomize/site-packages/sitecustomize.py`

本基盤環境では、Pythonプロセス起動時に `sitecustomize.py` が自動的にロードされ、以下のパッチを適用します。これにより**アプリケーションコードへの変更は一切不要**です。

#### A. 自動復元 (Hydration)
`awslambdaric` (Lambda Runtime Interface Client) にパッチを当て、イベント受信時に `ClientContext` から Trace ID を取り出し、`_X_AMZN_TRACE_ID` 環境変数にセットします。

#### B. 自動注入 (Injection)
```python
def _inject_client_context_hook(params, **kwargs):
    """
    boto3 Lambda.invoke() 呼び出し時に自動的に
    ClientContext に trace_id を注入するフック
    """
    trace_id = _get_current_trace_id()  # 環境変数から取得
    if not trace_id:
        return
    
    ctx_data = {}
    if "ClientContext" in params:
        ctx_data = json.loads(base64.b64decode(params["ClientContext"]))
    
    if "custom" not in ctx_data:
        ctx_data["custom"] = {}
    
    if "trace_id" not in ctx_data["custom"]:
        ctx_data["custom"]["trace_id"] = trace_id
        params["ClientContext"] = base64.b64encode(
            json.dumps(ctx_data).encode()
        ).decode()

# boto3 イベントに登録
client.meta.events.register(
    "provide-client-params.lambda.Invoke",
    _inject_client_context_hook
)
```

**役割**:
- Lambda 関数内から `boto3.client("lambda").invoke()` を呼び出した時
- 自動的に現在の Trace ID を ClientContext に注入
- 開発者は何もせずに Trace ID が連鎖伝播される

---

### 5. Java Runtime (javaagent)

**ファイル**: `runtime/java/extensions/agent/`

Java ランタイムでは `lambda-java-agent.jar` が `JAVA_TOOL_OPTIONS` で自動注入され、以下を行います。

- `LambdaClient#invoke()` 呼び出し時に `ClientContext.custom.trace_id` を自動注入
- `TraceContext` から Trace ID / Request ID を取得し、ログや送信に付与
- アプリケーションコードへの変更は不要

## Implementation references
- `services/gateway/main.py`
- `services/common/core/trace.py`
- `services/common/core/request_context.py`
- `services/gateway/services/lambda_invoker.py`
- `runtime/python/extensions/sitecustomize/site-packages/sitecustomize.py`
- `runtime/java/extensions/wrapper/src/com/runtime/lambda/HandlerWrapper.java`
- `runtime/java/extensions/agent/src/main/java/com/runtime/agent/AgentMain.java`

---

## ログへの Trace ID 付与

**ファイル**: `services/common/core/logging_config.py`

```python
class CustomJsonFormatter(logging.Formatter):
    def format(self, record):
        log_record = {
            "_time": self._format_time(record),
            "level": record.levelname,
            "message": record.getMessage(),
            "trace_id": get_trace_id(),  # ContextVar から取得
            # ...
        }
        return json.dumps(log_record)
```

すべてのログに自動的に `trace_id` フィールドが追加されます。

---

## トラブルシューティング

### Trace ID が `not-found` になる

**原因**: `sitecustomize.py` のパッチが適用されていない可能性があります。

**解決策**:
Dockerfileで `sitecustomize.py` が正しくコピーされているか、また環境変数 `PYTHONPATH` が正しく設定されているか確認してください。本基盤標準のベースイメージを使用していれば自動的に設定されます。
```

### Lambda 連鎖呼び出しで Trace ID が途切れる

**原因**: sitecustomize.py が正しくロードされていない

**確認方法**:
```bash
docker logs lambda-xxx | grep sitecustomize
# "[sitecustomize] All patches applied" が出力されていれば OK
```

### VictoriaLogs で Trace ID が表示されない

**原因**: ログフォーマッタが `trace_id` フィールドを出力していない

**確認**: fluent-bit 経由でログが正しくパースされているか確認
