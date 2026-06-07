# CI/CD 学习指南（Step by Step）

本指南配合各仓库的 `.github/workflows/ci.yml`，从 **CI（持续集成）** 开始，逐步过渡到 **CD（持续部署）**。

## 核心概念

| 术语 | 含义 |
| --- | --- |
| **CI** | 每次 push / PR，自动跑测试和构建，尽早发现错误 |
| **CD** | CI 通过后，自动部署到服务器 |
| **Workflow** | GitHub Actions 里的一条流水线，由 YAML 定义 |
| **Job** | 流水线里的一组步骤（如在 Ubuntu 上编译） |
| **Step** | Job 内的单个动作（checkout、测试、构建） |

## Step 1：CI 已就绪（当前阶段）

4 个仓库均已添加 `ci.yml`：

| 仓库 | CI 做什么 |
| --- | --- |
| `ai-cs-gateway` | `go test` + `go build` |
| `ai-cs-ai-service` | 安装依赖 + 语法检查 + 导入冒烟 |
| `ai-cs-web-h5` | `pnpm install` + `pnpm build` |
| `ai-cs-devops` | 校验 SQL schema + `docker compose config` |

### 如何触发

1. 将 workflow 文件提交并 push 到 GitHub
2. 打开仓库 → **Actions** 标签页
3. 看到绿色 ✓ 表示 CI 通过

### 触发时机

- 向 `master` / `main` push
- 向 `master` / `main` 提 Pull Request

## Step 2：本地先跑一遍（推荐）

推送前在本地执行与 CI 相同的命令：

```bash
# gateway
cd ai-cs-gateway && go test ./... && go build -o bin/server ./cmd/server

# ai-service
cd ai-cs-ai-service && pip install -r requirements.txt && python -m compileall app

# web-h5
cd ai-cs-web-h5 && pnpm install --frozen-lockfile && pnpm build

# devops
cd ai-cs-devops && python scripts/validate_schema.py && docker compose config -q
```

## Step 3：看懂 workflow 结构

以 `ai-cs-gateway/.github/workflows/ci.yml` 为例：

```yaml
name: CI                    # 流水线名称
on:                         # 什么时候跑
  push:
    branches: [master, main]
jobs:                       # 要执行的任务
  test-and-build:
    runs-on: ubuntu-latest  # 在 GitHub 提供的 Ubuntu 虚拟机上跑
    steps:                  # 按顺序执行的步骤
      - uses: actions/checkout@v4    # 拉代码
      - uses: actions/setup-go@v5    # 装 Go
      - run: go test ./...           # 跑测试
```

## Step 4：下一步 CD（暂未启用）

CI 稳定后，可增加 `deploy.yml`：

1. 在 GitHub **Settings → Secrets** 配置：
   - `DEPLOY_HOST`：腾讯云 CVM 公网 IP
   - `DEPLOY_USER`：如 `ubuntu`
   - `DEPLOY_SSH_KEY`：SSH 私钥
2. CI 通过后 SSH 到服务器执行 `git pull && build && systemctl restart`

> CD 会在 CI 全部绿灯后再做，避免把坏代码部署到线上。

## 常见问题

| 现象 | 处理 |
| --- | --- |
| Actions 页没有记录 | 确认 `.github/workflows/ci.yml` 已 push 到 GitHub |
| `pnpm install` 失败 | 本地先 `pnpm install` 更新 `pnpm-lock.yaml` 再提交 |
| `go test` 失败 | 本地先 `go test ./...` 排查 |
| devops CI 报 compose 错误 | 本地 `docker compose config -q` 检查 YAML |
