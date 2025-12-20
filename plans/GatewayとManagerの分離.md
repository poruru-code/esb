実装計画をまとめたMarkdownファイルを作成しました。以下のリンクからダウンロードできます。

`TDD_Implementation_Plan.md` というファイル名で保存してください。

```markdown
# GatewayとManagerの分離 TDD実装計画書

GatewayとManagerを分離し、堅牢なマイクロサービスアーキテクチャへ移行するための詳細な実装計画です。
本計画は**「インタフェース定義」→「テスト作成(Red)」→「実装(Green)」→「リファクタリング(Refactor)」** のTDDサイクルに基づいています。

---

## 0. 事前準備: ディレクトリ構成と責務の定義

リポジトリ構成を以下のように再編します。

```text
.
├── services/
│   ├── gateway/           # 公開用 (認証, ルーティング) - Docker依存なし
│   │   ├── main.py        # Gatewayのエントリーポイント
│   │   └── client.py      # Managerと通信するHTTPクライアント
│   └── manager/           # 内部用 (コンテナ管理) - Docker依存あり
│       ├── main.py        # Managerのエントリーポイント
│       └── service.py     # コンテナ操作ロジック (旧 container.py)
├── shared/                # 共有スキーマ (Pydantic models)
└── docker-compose.yml     # 2つのサービスを定義

```

### 定義するAPIインタフェース (Contract)

**Manager Service Internal API:**

* **Endpoint:** `POST /containers/ensure`
* **Input:** `{"function_name": "lambda-hello"}`
* **Output:** `{"host": "172.18.x.x", "port": 8080}`

---

## Phase 1: Manager Service の実装 (Docker操作の隔離)

**目的:** Docker操作ロジックを独立したWeb APIとして切り出します。

### Step 1.1: テスト作成 (Red)

`services/manager/tests/test_api.py` を作成し、Docker操作の呼び出しを検証します。

```python
# services/manager/tests/test_api.py
import pytest
from fastapi.testclient import TestClient
from unittest.mock import MagicMock, patch

# まだ存在しないアプリをインポート（TDD）
from services.manager.main import app 

client = TestClient(app)

@patch("services.manager.service.docker.from_env")
def test_ensure_container_starts_new(mock_docker):
    """コンテナが存在しない場合、新規起動することを確認"""
    # Docker Clientのモック構築
    mock_client = MagicMock()
    mock_docker.return_value = mock_client
    
    # 既存コンテナは見つからない設定
    mock_client.containers.list.return_value = []
    # runの戻り値（起動したコンテナ）
    mock_container = MagicMock()
    mock_container.attrs = {"NetworkSettings": {"Networks": {"dind-network": {"IPAddress": "10.0.0.5"}}}}
    mock_client.containers.run.return_value = mock_container

    # API実行
    response = client.post("/containers/ensure", json={"function_name": "lambda-hello"})

    # 検証
    assert response.status_code == 200
    assert response.json()["host"] == "10.0.0.5"
    
    # runが正しい引数で呼ばれたか厳密にチェック
    mock_client.containers.run.assert_called_once()
    args, kwargs = mock_client.containers.run.call_args
    assert kwargs["image"] == "lambda-hello:latest"
    assert kwargs["privileged"] is False # Manager自体は特権だが、Lambdaは非特権であるべき

```

### Step 1.2: 実装 (Green)

`services/manager/main.py` と `service.py` を実装し、テストを通過させます。

```python
# services/manager/main.py
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from .service import ContainerManager

app = FastAPI()
manager = ContainerManager() # シングルトン化

class EnsureRequest(BaseModel):
    function_name: str

@app.post("/containers/ensure")
async def ensure_container(req: EnsureRequest):
    try:
        # 非同期で実装すること
        host = await manager.ensure_container_running(req.function_name)
        return {"host": host, "port": 8080}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

```

---

## Phase 2: Gateway Service の実装 (Managerへの委譲)

**目的:** GatewayからDocker依存を排除し、Manager APIクライアントに置き換えます。

### Step 2.1: テスト作成 (Red)

`services/gateway/tests/test_routing.py` で、GatewayがManagerへ正しくリクエストしているか検証します。

```python
# services/gateway/tests/test_routing.py
import pytest
from fastapi.testclient import TestClient
import respx
from httpx import Response
from services.gateway.main import app

client = TestClient(app)

@respx.mock
def test_gateway_delegates_to_manager():
    """GatewayがManager APIを叩いてLambdaのホスト情報を取得するか"""
    
    # Manager API のモック定義
    manager_route = respx.post("http://manager:8081/containers/ensure").mock(
        return_value=Response(200, json={"host": "10.0.0.5", "port": 8080})
    )
    
    # Lambda RIE のモック定義 (プロキシ先の応答)
    lambda_route = respx.post("[http://10.0.0.5:8080/2015-03-31/functions/function/invocations](http://10.0.0.5:8080/2015-03-31/functions/function/invocations)").mock(
        return_value=Response(200, json={"message": "hello from lambda"})
    )

    # Gatewayへのリクエスト実行
    response = client.post("/functions/lambda-hello/invocations", json={})

    # 検証
    assert response.status_code == 200
    assert response.json() == {"message": "hello from lambda"}
    
    # Managerが呼ばれたか確認
    assert manager_route.called
    assert manager_route.calls.last.request.content == b'{"function_name":"lambda-hello"}'

```

### Step 2.2: 実装 (Green)

Gatewayの `client.py` を実装し、Managerとの通信を確立します。

```python
# services/gateway/client.py
import httpx
import os

MANAGER_URL = os.getenv("MANAGER_URL", "http://manager:8081")

async def get_lambda_host(function_name: str) -> str:
    async with httpx.AsyncClient() as client:
        resp = await client.post(
            f"{MANAGER_URL}/containers/ensure", 
            json={"function_name": function_name},
            timeout=10.0
        )
        resp.raise_for_status()
        return resp.json()["host"]

```

---

## Phase 3: ゾンビプロセス対策 (Lifecycle Management)

**目的:** Manager起動時に、管理外の古いコンテナをクリーンアップするロジックを実装します。

### Step 3.1: テスト作成 (Red)

`services/manager/tests/test_lifecycle.py` を作成します。

```python
# services/manager/tests/test_lifecycle.py
import pytest
from unittest.mock import MagicMock, patch
from services.manager.main import lifespan

@pytest.mark.asyncio
async def test_startup_cleans_zombies():
    """起動時に古いコンテナをpruneするか検証"""
    with patch("services.manager.main.docker.from_env") as mock_docker:
        mock_client = MagicMock()
        mock_docker.return_value = mock_client
        
        # ゾンビコンテナのモック
        zombie = MagicMock()
        zombie.name = "sample-dind-lambda-zombie"
        mock_client.containers.list.return_value = [zombie]
        
        # Lifespanコンテキストを実行
        async with lifespan(None):
            pass # アプリ起動中
            
        # 検証: ゾンビに対して kill と remove が呼ばれたか
        zombie.kill.assert_called_once()
        zombie.remove.assert_called_once()

```

### Step 3.2: 実装 (Green)

FastAPIの `lifespan` を実装し、自己修復機能を組み込みます。

```python
# services/manager/main.py
from contextlib import asynccontextmanager
import docker

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup Logic: ゾンビ退治
    client = docker.from_env()
    # ラベル等でフィルタリングするのが望ましい
    containers = client.containers.list(filters={"label": "created_by=sample-dind"})
    for container in containers:
        try:
            container.kill()
            container.remove()
        except:
            pass
    yield
    # Shutdown Logic (Optional)

```

---

## Phase 4: インフラ統合 (Infrastructure as Code)

`docker-compose.yml` を更新してサービスを結合します。

```yaml
services:
  gateway:
    build:
      context: .
      dockerfile: services/gateway/Dockerfile
    environment:
      - MANAGER_URL=http://manager:8081
    ports:
      - "8080:8080"
    depends_on:
      - manager
    # 注意: docker.sock のマウントは不要

  manager:
    build:
      context: .
      dockerfile: services/manager/Dockerfile
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock # ここだけが特権を持つ
    expose:
      - "8081"
    environment:
      - CONTAINERS_NETWORK=sample-dind-lambda_default

```

---

## 検証プラン

1. **Unit Tests:**
* `pytest services/gateway/tests/` (Gatewayロジック)
* `pytest services/manager/tests/` (Docker操作ロジック)
* 全テストがPassすることを確認。


2. **Integration Test:**
* `docker compose up --build`
* Gateway経由 (`http://localhost:8080`) でLambdaを実行し、Managerが裏でコンテナを起動することを確認。


3. **Resilience Test:**
* `docker compose restart manager` を実行中にGatewayへリクエストを投げ、適切にエラーハンドリング（503 Service Unavailable等）されるか確認。
* 再起動後、直ちに正常動作に復帰することを確認。

