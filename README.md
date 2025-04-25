<div align="center">

# GoDoxy

[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=yusing_go-proxy&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=yusing_godoxy)
![GitHub last commit](https://img.shields.io/github/last-commit/yusing/godoxy)
[![Lines of Code](https://sonarcloud.io/api/project_badges/measure?project=yusing_go-proxy&metric=ncloc)](https://sonarcloud.io/summary/new_code?id=yusing_godoxy)
![Demo](https://img.shields.io/website?url=https%3A%2F%2Fgodoxy.demo.6uo.me&label=Demo&link=https%3A%2F%2Fgodoxy.demo.6uo.me)
[![Discord](https://dcbadge.limes.pink/api/server/umReR62nRd?style=flat)](https://discord.gg/umReR62nRd)

A lightweight, simple, and [performant](https://github.com/yusing/godoxy/wiki/Benchmarks) reverse proxy with WebUI.

For full documentation, check out **[Wiki](https://github.com/yusing/godoxy/wiki)**

**EN** | <a href="README_CHT.md">中文</a>

<img src="screenshots/webui.jpg" style="max-width: 650">

</div>

## Table of content

<!-- TOC -->

- [GoDoxy](#godoxy)
  - [Table of content](#table-of-content)
  - [Running demo](#running-demo)
  - [Key Features](#key-features)
  - [Prerequisites](#prerequisites)
  - [Setup](#setup)
  - [How does GoDoxy work](#how-does-godoxy-work)
  - [Screenshots](#screenshots)
    - [idlesleeper](#idlesleeper)
    - [Metrics and Logs](#metrics-and-logs)
  - [Manual Setup](#manual-setup)
    - [Folder structrue](#folder-structrue)
  - [Build it yourself](#build-it-yourself)

## Running demo

<https://godoxy.demo.6uo.me>

[![Deployed on Zeabur](https://zeabur.com/deployed-on-zeabur-dark.svg)](https://zeabur.com/referral?referralCode=yusing&utm_source=yusing&utm_campaign=oss)

## Key Features

- **Simple**
  - Effortless configuration with [simple labels](https://github.com/yusing/godoxy/wiki/Docker-labels-and-Route-Files) or WebUI
  - [Simple multi-node setup](https://github.com/yusing/godoxy/wiki/Configurations#multi-docker-nodes-setup)
  - Detailed error messages for easy troubleshooting.
- **ACL**: connection / request level access control
  - IP/CIDR
  - Country **(Maxmind account required)**
  - Timezone **(Maxmind account required)**
  - **Access logging**
- **Advanced Automation**
  - Automatic SSL certificate management with Let's Encrypt ([using DNS-01 Challenge](https://github.com/yusing/go-proxy/wiki/Supported-DNS%E2%80%9001-Providers))
  - Auto-configuration for Docker containers
  - Hot-reloading of configurations and container state changes
- **Idle-sleep**: stop and wake containers based on traffic _(see [screenshots](#idlesleeper))_
  - Docker containers
  - Proxmox LXCs
- **Traffic Management**
  - HTTP reserve proxy
  - TCP/UDP port forwarding
  - **OpenID Connect support**: SSO and secure your apps easily
- **Customization**
  - [HTTP middlewares](https://github.com/yusing/go-proxy/wiki/Middlewares)
  - [Custom error pages support](https://github.com/yusing/go-proxy/wiki/Middlewares#custom-error-pages)
- **Web UI**
  - App Dashboard
  - Config Editor
  - Uptime and System Metrics
  - Docker Logs Viewer
- **Cross-Platform support**
  - Supports **linux/amd64** and **linux/arm64**
- **Efficient and Performant**
  - Written in **[Go](https://go.dev)**

## Prerequisites

Configure Wildcard DNS Record(s) to point to machine running `GoDoxy`, e.g.

- A Record: `*.domain.com` -> `10.0.10.1`
- AAAA Record (if you use IPv6): `*.domain.com` -> `::ffff:a00:a01`

## Setup

> [!NOTE]
> GoDoxy is designed to be running in `host` network mode, do not change it.
>
> To change listening ports, modify `.env`.

1. Prepare a new directory for docker compose and config files.

2. Run setup script inside the directory, or [set up manually](#manual-setup)

   ```shell
   /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/yusing/godoxy/main/scripts/setup.sh)"
   ```

3. You may now do some extra configuration on WebUI `https://godoxy.yourdomain.com`

## How does GoDoxy work

1. List all the containers
2. Read container name, labels and port configurations for each of them
3. Create a route if applicable (a route is like a "Virtual Host" in NPM)
4. Watch for container / config changes and update automatically

> [!NOTE]
> GoDoxy uses the label `proxy.aliases` as the subdomain(s), if unset it defaults to the `container_name` field in docker compose.
>
> For example, with the label `proxy.aliases: qbt` you can access your app via `qbt.domain.com`.

## Screenshots

### idlesleeper

![idlesleeper](screenshots/idlesleeper.webp)

### Metrics and Logs

<div align="center">
  <table>
    <tr>
      <td align="center"><img src="screenshots/uptime.png" alt="Uptime Monitor" width="250"/></td>
      <td align="center"><img src="screenshots/docker-logs.jpg" alt="Docker Logs" width="250"/></td>
      <td align="center"><img src="screenshots/docker.jpg" alt="Server Overview" width="250"/></td>
    </tr>
    <tr>
      <td align="center"><b>Uptime Monitor</b></td>
      <td align="center"><b>Docker Logs</b></td>
      <td align="center"><b>Server Overview</b></td>
    </tr>
        <tr>
      <td align="center"><img src="screenshots/system-monitor.jpg" alt="System Monitor" width="250"/></td>
      <td align="center"><img src="screenshots/system-info-graphs.jpg" alt="Graphs" width="250"/></td>
    </tr>
    <tr>
      <td align="center"><b>System Monitor</b></td>
      <td align="center"><b>Graphs</b></td>
    </tr>
  </table>
</div>

## Manual Setup

1. Make `config` directory then grab `config.example.yml` into `config/config.yml`

   `mkdir -p config && wget https://raw.githubusercontent.com/yusing/godoxy/main/config.example.yml -O config/config.yml`

2. Grab `.env.example` into `.env`

   `wget https://raw.githubusercontent.com/yusing/godoxy/main/.env.example -O .env`

3. Grab `compose.example.yml` into `compose.yml`

   `wget https://raw.githubusercontent.com/yusing/godoxy/main/compose.example.yml -O compose.yml`

### Folder structrue

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

## Build it yourself

1. Clone the repository `git clone https://github.com/yusing/godoxy --depth=1`

2. Install / Upgrade [go (>=1.22)](https://go.dev/doc/install) and `make` if not already

3. Clear cache if you have built this before (go < 1.22) with `go clean -cache`

4. get dependencies with `make get`

5. build binary with `make build`

[🔼Back to top](#table-of-content)
