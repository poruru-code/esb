<!--
Where: services/gateway/docs/security.md
What: Gateway authentication behavior and security-related settings.
Why: Consolidate auth endpoint specification and related controls.
-->
# セキュリティ / 認証

## 認証エンドポイント（UserAuthenticateExecutor 互換）
### パス
`POST /user/auth/v1`（`AUTH_ENDPOINT_PATH` で変更可能）

### ヘッダ
- `x-api-key`: API キー
- `Content-Type: application/json`

### リクエスト
```json
{
  "AuthParameters": {
    "USERNAME": "testuser",
    "PASSWORD": "testpass"
  }
}
```

### 成功レスポンス（200）
```json
{
  "AuthenticationResult": {
    "IdToken": "eyJ0eXAiOiJKV1QiLCJhbGc..."
  }
}
```

### エラー仕様
| 条件 | ステータス | `PADMA_USER_AUTHORIZED` | 説明 |
| --- | --- | --- | --- |
| x-api-key 不正/なし | `401` | なし | プロキシ認証エラー |
| USERNAME/PASSWORD 不正 | `401` | `true` | ユーザー認証エラー |

### 付与ヘッダ
- `PADMA_USER_AUTHORIZED: true`（API キー検証が成功した場合）

## 関連設定
| 変数 | 説明 |
| --- | --- |
| `X_API_KEY` | API キー |
| `AUTH_USER` / `AUTH_PASS` | 認証ユーザー |
| `AUTH_ENDPOINT_PATH` | 認証パス |
| `JWT_SECRET_KEY` | JWT 署名キー |

---

## Implementation references
- `services/gateway/main.py`
- `services/gateway/config.py`
- `services/gateway/core/security.py`
