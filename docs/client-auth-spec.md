# クライアント仕様対応 - 認証エンドポイント

## 概要

`UserAuthenticatExecutor`クライアントの仕様に合わせて、認証エンドポイントを実装しました。

## 実装内容

### 📌 エンドポイント仕様

**パス:** `POST /user/auth/v1`（`AUTH_ENDPOINT_PATH` で変更可能）

**リクエストヘッダ:**
- `x-api-key`: API GatewayのAPIキー
- `Content-Type`: `application/json`

**リクエストボディ:**
```json
{
  "AuthParameters": {
    "USERNAME": "testuser",
    "PASSWORD": "testpass"
  }
}
```

**成功レスポンス (200 OK):**
```json
{
  "AuthenticationResult": {
    "IdToken": "eyJ0eXAiOiJKV1QiLCJhbGc..."
  }
}
```

---

### ✅ 実装した機能

#### 1. **Pydanticモデルの定義**

```python
class AuthParameters(BaseModel):
    USERNAME: str
    PASSWORD: str

class AuthRequest(BaseModel):
    AuthParameters: AuthParameters

class AuthenticationResult(BaseModel):
    IdToken: str

class AuthResponse(BaseModel):
    AuthenticationResult: AuthenticationResult
```

#### 2. **x-api-keyヘッダーの検証**

- 環境変数 `X_API_KEY` で設定されたAPIキーと照合
- 不一致の場合、`401 Unauthorized`（`PADMA_USER_AUTHORIZED`ヘッダーなし）
- これによりプロキシ認証エラーとして扱われる

#### 3. **PADMA_USER_AUTHORIZEDヘッダーの設定**

- APIキー検証後、レスポンスヘッダーに `PADMA_USER_AUTHORIZED: true` を設定
- これによりユーザー認証段階であることをクライアントが識別可能

#### 4. **エラーハンドリング**

| 条件                      | ステータス | PADMA_USER_AUTHORIZEDヘッダー | エラー種別         |
| :------------------------ | :--------- | :---------------------------- | :----------------- |
| x-api-key不正/なし        | 401        | なし                          | プロキシ認証エラー |
| ユーザー名/パスワード不正 | 401        | あり                          | ユーザー認証エラー |

---

### 🔧 設定変更

#### docker-compose.yml

```yaml
    environment:
      - JWT_SECRET_KEY=dev-secret-key-change-in-production
      - X_API_KEY=dev-api-key-change-in-production
      - AUTH_ENDPOINT_PATH=/user/auth/v1
```

#### 環境変数

- `JWT_SECRET_KEY`: JWTトークンの署名キー
- `X_API_KEY`: API Gatewayのアクセスキー
- `AUTH_ENDPOINT_PATH`: 認証エンドポイントのパス (デフォルト: `/user/auth/v1`)

---

### 📝 使用例

#### 認証リクエスト

```bash
curl -X POST http://localhost:8000/user/auth/v1 \
  -H "x-api-key: dev-api-key-change-in-production" \
  -H "Content-Type: application/json" \
  -d '{
    "AuthParameters": {
      "USERNAME": "testuser",
      "PASSWORD": "testpass"
    }
  }'
```

#### 成功レスポンス

```json
{
  "AuthenticationResult": {
    "IdToken": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9..."
  }
}
```

#### Lambda呼び出し

取得したIdTokenを使用：

```bash
curl -X POST http://localhost:8000/invoke/hello \
  -H "Authorization: Bearer eyJ0eXAiOiJKV1QiLC..." \
  -H "Content-Type: application/json" \
  -d '{"message": "test"}'
```

---

## クライアント仕様との対応

### ✅ 実装済み

- [x] `/user/auth/ver1.0` エンドポイント
- [x] `AuthParameters` リクエスト形式
- [x] `AuthenticationResult.IdToken` レスポンス形式
- [x] `x-api-key` ヘッダー検証
- [x] `PADMA_USER_AUTHORIZED` ヘッダー対応
- [x] プロキシ認証エラーとユーザー認証エラーの区別

### 📋 TODO（将来的な拡張）

- [ ] データベース照合ロジックの実装（現在は固定認証）
- [ ] 403 Forbidden対応
- [ ] 503 Service Unavailable対応
- [ ] IDトークンのキャッシュ機能（クライアント側で実装済み）
