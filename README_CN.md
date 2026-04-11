# Robust2API

`Robust2API` 是一个 AI API 网关，用于把多个上游 AI 账户统一暴露到单一的鉴权与控制平面之后。它负责认证、API Key 分发、计费、负载均衡、限流，以及向 OpenAI、Anthropic、Gemini 等上游服务转发请求。

为兼容文件系统、URL 和命令行工具，仓库中的路径、二进制名称、Helm Chart 目录以及配置示例等机器标识统一使用小写 `robust2api`。

## 核心能力

- 支持 OAuth 与 API Key 形式的多上游账号管理
- 为终端用户或内部调用方签发平台 API Key
- 基于 Token 的精细化用量统计与计费
- 支持粘性会话的智能调度与故障切换
- 用户级与账号级并发控制
- 请求级与 Token 级速率限制
- 提供管理后台用于运维和账号管理
- 可选接入 OpenTelemetry、Prometheus、Grafana、Tempo、Loki

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go, Gin, Ent |
| 前端 | Vue 3, TypeScript, Vite, TailwindCSS |
| 数据库 | PostgreSQL |
| 缓存 / 队列 | Redis |
| 基础设施 | Terraform, Helm, Flux CD |

## 架构

后端采用分层结构：

```text
handler -> service -> repository -> ent/redis -> PostgreSQL
```

主要服务：

- `gateway`：推理代理与上游请求路由
- `control`：管理 / 认证 API 与前端宿主
- `worker`：用量记录与计费等后台任务
- `bootstrap`：数据库迁移与初始管理员种子数据

## 快速开始

### 环境要求

- Go 1.21+
- Node.js 18+
- pnpm
- PostgreSQL 15+
- Redis 7+

### 从源码构建

```bash
git clone https://github.com/wchen99998/robust2api.git
cd robust2api

cd frontend
pnpm install
pnpm build

cd ../backend
go build -o robust2api-gateway ./cmd/gateway
go build -o robust2api-control ./cmd/control
go build -o robust2api-bootstrap ./cmd/bootstrap
go build -o robust2api-worker ./cmd/worker
```

先创建运行时配置文件 `/etc/robust2api/config.yaml`，再导出初始化所需环境变量：

```bash
export DATABASE_HOST=localhost
export DATABASE_PORT=5432
export DATABASE_USER=postgres
export DATABASE_PASSWORD=your_password
export DATABASE_DBNAME=robust2api
export DATABASE_SSLMODE=disable
export JWT_SECRET="$(openssl rand -hex 32)"
export TOTP_ENCRYPTION_KEY="$(openssl rand -hex 32)"
export ADMIN_EMAIL=admin@example.com
export ADMIN_PASSWORD=change-me
```

首次运行先执行 bootstrap，再启动各服务：

```bash
./robust2api-bootstrap
./robust2api-gateway
./robust2api-control
./robust2api-worker
```

## 配置说明

- 运行时 YAML 配置文件路径为 `/etc/robust2api/config.yaml`
- `bootstrap` 依赖环境变量读取初始化密钥
- `JWT_SECRET` 至少 32 字节
- `TOTP_ENCRYPTION_KEY` 必须为 64 位十六进制字符串
- 设置 `RUN_MODE=simple` 可启用简化模式

配置骨架示例：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "your_password"
  dbname: "robust2api"

redis:
  host: "localhost"
  port: 6379
  password: ""
```

## 开发

在 `backend/` 下运行后端服务：

```bash
go run ./cmd/gateway/
go run ./cmd/control/
go run ./cmd/worker/
```

在 `frontend/` 下运行前端：

```bash
pnpm install
pnpm dev
```

修改 `backend/ent/schema` 后，需要重新生成代码：

```bash
cd backend
go generate ./ent
go generate ./cmd/gateway
go generate ./cmd/control
go generate ./cmd/worker
```

## 部署

- Kubernetes 与 GitOps 部署说明见 [DEPLOY.md](DEPLOY.md)
- Terraform 基础设施位于 `infra/production/`
- Helm Chart 位于 `deploy/helm/robust2api/`
- 生产集群清单位于 `clusters/production/`

如果前面使用 Nginx 且需要兼容 Codex CLI，请在 `http` 块中加入：

```nginx
underscores_in_headers on;
```

## 项目结构

```text
robust2api/
├── backend/
│   ├── cmd/
│   ├── ent/
│   ├── internal/
│   └── resources/
├── frontend/
├── deploy/
│   ├── Caddyfile
│   └── helm/robust2api/
├── clusters/production/
├── infra/
└── docs/
```

## 健康检查

```bash
curl -I http://localhost:8080/livez
curl -I http://localhost:8080/readyz
curl -I http://localhost:8081/livez
curl -I http://localhost:8081/readyz
```

## License

MIT
