# MCSGlobalStatus Probe

這個 repo 是 Probe。

它會連回 controller，接收 ping task，然後把結果送回去。

## 快速開始

先準備 secret。

最簡單的方式是直接設 env：

```bash
export PROBE_SECRET=your-secret
```

`.env.example` 只是範例，你如果有自己的 shell / process manager，再把它帶進環境就好。

也可以直接用參數帶：

```bash
go run . -server ws://127.0.0.1:3000 -id local-01 -region Local -secret your-secret
```

如果要正式編譯：

```bash
go build -o probe .
./probe -server ws://127.0.0.1:3000 -id local-01 -region Local -secret your-secret
```

## env

目前只需要一個：

| 變數 | 用途 |
| --- | --- |
| `PROBE_SECRET` | Probe 連 controller 時用的 secret |

## 參數

| 參數 | 用途 | 預設 |
| --- | --- | --- |
| `-server` | Controller WebSocket 位址 | `ws://localhost:3000` |
| `-id` | Probe ID | `local-01` |
| `-region` | 節點顯示名稱 | `Local` |
| `-secret` | 連線 secret，也可以改用 `PROBE_SECRET` | 空 |

## 要注意的事

- `id` 和 secret 要跟 API repo 的 `probes.json` 對得上，不然會被拒絕。
- Probe 本身會自動重連，但 controller 要先真的有起來。
- Probe 會定期送 heartbeat，所以 controller 的 `/health/details` 可以看最近活動時間。
- 這個程式不是對外 HTTP 服務，它只會連回 controller 的 WebSocket。
