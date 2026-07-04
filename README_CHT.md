<div align="center">

<img src="assets/godoxy.png" width="200">

[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=yusing_go-proxy&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=yusing_go-proxy)
![GitHub last commit](https://img.shields.io/github/last-commit/yusing/godoxy)
[![Lines of Code](https://sonarcloud.io/api/project_badges/measure?project=yusing_go-proxy&metric=ncloc)](https://sonarcloud.io/summary/new_code?id=go-proxy)

![Demo](https://img.shields.io/website?url=https%3A%2F%2Fdemo.godoxy.dev&label=Demo&link=https%3A%2F%2Fdemo.godoxy.dev)
[![Discord](https://dcbadge.limes.pink/api/server/umReR62nRd?style=flat)](https://discord.gg/umReR62nRd)

輕量、易用、高效能，且帶有 WebUI 的反向代理。

<h5>
<a href="https://docs.godoxy.dev">網站</a> | <a href="https://docs.godoxy.dev/Home.html">文檔</a> | <a href="https://discord.gg/umReR62nRd">Discord</a>
</h5>

<h5><a href="README.md">EN</a> | 中文</h5>

<img src="screenshots/webui.jpg" style="max-width: 650">

有疑問? 問 [ChatGPT](https://chatgpt.com/g/g-6825390374b481919ad482f2e48936a1-godoxy-assistant)！（鳴謝 [@ismesid](https://github.com/arevindh)）

</div>

## 運行示例

<https://demo.godoxy.dev>

## 快速開始

設置 DNS 記錄指向運行 `GoDoxy` 的機器，例如：

- A 記錄：`*.domain.com` -> `10.0.10.1`
- AAAA 記錄：`*.domain.com` -> `::ffff:a00:a01`

> [!NOTE]
> GoDoxy 僅在 `host` 網路模式下運作，請勿更改。
>
> 如需更改監聽埠，請修改 `.env`。

1. 準備一個新目錄用於 Docker Compose 和設定檔。

2. 在目錄內運行安裝腳本，或[手動安裝](#手動安裝)

   ```shell
   /bin/sh -c "$(curl -fsSL https://raw.githubusercontent.com/yusing/godoxy/main/scripts/setup.sh)"
   ```

3. 從產生的 `compose.yml` 啟動 Docker Compose 服務：

   ```shell
   docker compose up -d
   ```

4. 現在可以在 WebUI `https://godoxy.yourdomain.com` 進行額外配置

## 主要特點

- **簡單安裝**
  - 透過 [Docker 標籤或路由檔](https://docs.godoxy.dev/Docker-labels-and-Route-Files)設定路由
  - 從 WebUI 管理路由、設定、容器、日誌、指標和上線時間
  - 使用[多節點 Docker 設定](https://docs.godoxy.dev/Configurations#multi-docker-nodes-setup)
- **自動路由**
  - 探索 Docker 和 Podman 容器
  - 設定檔與容器狀態變更時自動熱重載
  - 使用 [DNS-01 提供者](https://docs.godoxy.dev/DNS-01-Providers)管理 Let's Encrypt 憑證
- **流量管理**
  - HTTP 反向代理
  - TCP/UDP 連接埠轉送
  - OpenID Connect SSO
  - ForwardAuth 整合，例如 TinyAuth
  - [HTTP 中介軟體](https://docs.godoxy.dev/Middlewares)
  - [自訂錯誤頁面](https://docs.godoxy.dev/Custom-Error-Pages)
- **存取控制**
  - IP/CIDR 規則
  - 需 MaxMind 帳戶的國家和時區規則
  - 存取日誌
  - 定時存取摘要
- **閒置休眠**
  - 根據流量停止和喚醒 Docker 容器
  - 根據流量停止和喚醒 Proxmox LXC 容器
- **Proxmox 整合**
  - 將路由自動綁定至節點或 LXC 容器
  - 從 WebUI 啟動、停止和重新啟動 LXC 容器
  - 透過 WebSocket 串流節點和 LXC 日誌
- **平台支援**
  - Linux amd64
  - Linux arm64

## GoDoxy 如何運作

1. 列出所有容器
2. 讀取每個容器的名稱、標籤和連接埠設定
3. 在適用時建立路由 (類似 NPM 的「Virtual Host」)
4. 監看容器與設定變更並自動更新

> [!NOTE]
> GoDoxy 使用 `proxy.aliases` 標籤作為子網域；若未設定，則預設使用 Docker Compose 的 `container_name` 欄位。
>
> 例如設定標籤 `proxy.aliases: qbt` 後，可透過 `qbt.domain.com` 存取應用程式。

## 截圖

### 閒置休眠

![閒置休眠](screenshots/idlesleeper.webp)

### 指標與日誌

<div align="center">
  <table>
    <tr>
      <td align="center"><img src="screenshots/routes.jpg" alt="Routes" width="350"/></td>
      <td align="center"><img src="screenshots/servers.jpg" alt="Servers" width="350"/></td>
    </tr>
    <tr>
      <td align="center"><b>路由</b></td>
      <td align="center"><b>伺服器</b></td>
    </tr>
  </table>
</div>

## Proxmox 整合

GoDoxy 可透過配置的提供者自動探索和管理 Proxmox 節點和 LXC 容器。

### 自動路由綁定

路由透過反向查詢自動連結至 Proxmox 資源：

1. **節點級路由** (VMID = 0)：當主機名稱、IP 或別名符合 Proxmox 節點名稱或 IP 時
2. **容器級路由** (VMID > 0)：當主機名稱、IP 或別名符合 LXC 容器時

這可實現無需手動綁定的無縫代理配置：

```yaml
routes:
  pve-node-01:
    host: pve-node-01.internal
    port: 8006
    # 自動連結至 Proxmox 節點 pve-node-01
```

### WebUI 管理

您可以從 WebUI：

- **LXC 生命週期控制**：啟動、停止、重新啟動容器
- **節點日誌**：串流節點的即時 journalctl 或日誌檔案輸出
- **LXC 日誌**：串流容器的即時 journalctl 或日誌檔案輸出

## 更新 / 卸載系統代理 (System Agent)

安裝腳本同時支援 systemd 與 Alpine/OpenRC（`rc-service`）主機。

更新：

```bash
sh -c "$(curl -fsSL https://github.com/yusing/godoxy/raw/refs/heads/main/scripts/install-agent.sh)" -- update
```

卸載：

```bash
sh -c "$(curl -fsSL https://github.com/yusing/godoxy/raw/refs/heads/main/scripts/install-agent.sh)" -- uninstall
```

## 手動安裝

1. 建立 `config` 目錄，然後將 `config.example.yml` 下載到 `config/config.yml`

   `mkdir -p config && wget https://raw.githubusercontent.com/yusing/godoxy/main/config.example.yml -O config/config.yml`

2. 將 `.env.example` 下載到 `.env`

   `wget https://raw.githubusercontent.com/yusing/godoxy/main/.env.example -O .env`

3. 將 `compose.example.yml` 下載到 `compose.yml`

   `wget https://raw.githubusercontent.com/yusing/godoxy/main/compose.example.yml -O compose.yml`

### 資料夾結構

```shell
├── certs
│   ├── cert.crt
│   └── priv.key
├── compose.yml
├── config
│   ├── config.yml
│   ├── middlewares
│   │   ├── middleware1.yml
│   │   ├── middleware2.yml
│   ├── provider1.yml
│   └── provider2.yml
├── data
│   ├── metrics # metrics data
│   │   ├── uptime.json
│   │   └── system_info.json
└── .env
```

## 從原始碼編譯

1. 克隆儲存庫 `git clone https://github.com/yusing/godoxy --depth=1`

2. 如果尚未安裝，請安裝/升級 [go (>=1.22)](https://go.dev/doc/install) 和 [`shadowtree`](https://github.com/yusing/shadowtree)

3. 如果之前編譯過（go < 1.22），請使用 `go clean -cache` 清除快取

4. 使用 `shadowtree mod-tidy` 獲取依賴

5. 使用 `shadowtree build` 編譯二進制檔案

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=yusing/godoxy&type=Date)](https://www.star-history.com/#yusing/godoxy&Date)
