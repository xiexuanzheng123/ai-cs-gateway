# CI/CD 学习指南（Step by Step）

本指南配合各仓库的 `.github/workflows/ci.yml`，从 **CI（持续集成）** 到 **CD（持续部署）**。

## 核心概念

| 术语 | 含义 |
| --- | --- |
| **CI** | 每次 push / PR，自动跑测试和构建，尽早发现错误 |
| **CD** | CI 通过后，自动部署到腾讯云 CVM |
| **Workflow** | GitHub Actions 里的一条流水线，由 YAML 定义 |
| **Job** | 流水线里的一组步骤（如在 Ubuntu 上编译） |
| **Step** | Job 内的单个动作（checkout、测试、构建） |
| **Secret** | 存放在 GitHub 的敏感配置（IP、SSH 密钥），不会出现在日志里 |

---

## Step 1：CI（自动测试 + 构建）

4 个仓库的 `ci.yml` 在 push / PR 时自动执行：

| 仓库 | CI 做什么 |
| --- | --- |
| `ai-cs-gateway` | `go test` + `go build` |
| `ai-cs-ai-service` | 安装依赖 + 语法检查 + 导入冒烟 |
| `ai-cs-web-h5` | `pnpm install` + `pnpm build` |
| `ai-cs-devops` | 校验 SQL schema + `docker compose config` |

---

## Step 2：本地先跑一遍

```bash
cd ai-cs-gateway && go test ./... && go build -o bin/server ./cmd/server
cd ai-cs-ai-service && pip install -r requirements.txt && python -m compileall app
cd ai-cs-web-h5 && pnpm install --frozen-lockfile && pnpm build
cd ai-cs-devops && python scripts/validate_schema.py && docker compose config -q
```

---

## Step 3：看懂 workflow 结构

```yaml
name: CI
on:
  push:
    branches: [master, main]
jobs:
  test-and-build:        # Job 1：CI
    runs-on: ubuntu-latest
    steps: [...]

  deploy:                # Job 2：CD（仅 push 到 master 时）
    needs: test-and-build
    if: github.event_name == 'push'
    steps:
      - uses: appleboy/ssh-action@v1.2.0   # SSH 到腾讯云
```

**关键点**：`needs` 表示 CI 通过后才跑 CD；PR 只跑 CI，不部署。

---

## Step 4：配置 CD（部署到腾讯云）

### 4.1 服务器前置条件

CD 假设你已完成**首次手工部署**（见飞书上线文档）：

- 4 个仓库已 clone 到 `~/ai-cs/`
- Docker、Go、Python、Node、Nginx、systemd 服务已就绪
- `ai-cs-gateway`、`ai-cs-ai-service` 的 systemd 单元已创建

### 4.2 生成部署专用 SSH 密钥

在**本地电脑**执行：

```bash
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/ai-cs-deploy -N ""
```

- 公钥 `~/.ssh/ai-cs-deploy.pub` → 加到服务器 `~/.ssh/authorized_keys`
- 私钥 `~/.ssh/ai-cs-deploy` → 填入 GitHub Secret

```bash
# 把公钥加到腾讯云 CVM（把 IP 和用户名换成你的）
ssh-copy-id -i ~/.ssh/ai-cs-deploy.pub ubuntu@你的公网IP
```

### 4.3 配置 sudo 免密（systemctl 需要）

在服务器上：

```bash
sudo visudo
```

追加一行（把 `ubuntu` 换成你的用户名）：

```text
ubuntu ALL=(ALL) NOPASSWD: /bin/systemctl restart ai-cs-gateway, /bin/systemctl restart ai-cs-ai-service, /bin/systemctl reload nginx, /bin/systemctl is-active ai-cs-gateway, /bin/systemctl is-active ai-cs-ai-service
```

### 4.4 在 GitHub 添加 Secrets

对 **4 个仓库分别** 操作：`Settings` → `Secrets and variables` → `Actions` → `New repository secret`

| Secret 名称 | 值 | 必填 |
| --- | --- | --- |
| `DEPLOY_HOST` | 腾讯云 CVM 公网 IP | 是 |
| `DEPLOY_USER` | SSH 用户名，如 `ubuntu` | 是 |
| `DEPLOY_SSH_KEY` | 私钥全文（`~/.ssh/ai-cs-deploy` 内容） | 是 |
| `DEPLOY_BASE_PATH` | 代码根目录，默认 `~/ai-cs` 可不填 | 否 |

### 4.5 CD 各仓库部署内容

| 仓库 | CD 自动执行 |
| --- | --- |
| `ai-cs-gateway` | `git pull` → `go build` → `systemctl restart ai-cs-gateway` |
| `ai-cs-ai-service` | `git pull` → `pip install` → `systemctl restart ai-cs-ai-service` |
| `ai-cs-web-h5` | `git pull` → `pnpm build` → `nginx reload` |
| `ai-cs-devops` | `git pull` → `docker compose up -d` |

### 4.6 验证 CD

1. 改一行代码（如 README 加个空格）
2. `git commit && git push` 到 `master`
3. 打开 GitHub **Actions**，应看到两个 Job：`test-and-build` ✓ → `deploy` ✓
4. 浏览器访问服务器，确认变更生效

---

## 完整流程图

```text
开发者 push 到 master
        ↓
   GitHub Actions 启动
        ↓
   Job 1: CI（测试 + 构建）──失败──→ 邮件通知，不部署
        ↓ 成功
   Job 2: CD（SSH 到腾讯云）
        ↓
   git pull → build → restart
        ↓
   线上服务更新完成
```

---

## 常见问题

| 现象 | 处理 |
| --- | --- |
| deploy job 被跳过 | 正常：PR 不触发 CD；只有 push 到 master/main 才部署 |
| SSH 连接失败 | 检查 `DEPLOY_HOST`、安全组 22 端口、公钥是否加到服务器 |
| `sudo: a password is required` | 按 4.3 配置 sudo 免密 |
| `go: command not found` | 服务器需安装 Go，并确保 `/usr/local/go/bin` 在 PATH |
| `git pull` 失败 | 服务器仓库需先 `git clone`，且 deploy 密钥有读权限 |

---

## Step 5：后续可进阶

- 加单元测试，让 CI 更有价值
- 用 GitHub **Environments** 区分 test / prod
- 构建产物在 CI 机器编译，SCP 到服务器（减少服务器依赖）
- 合并为 monorepo，一条流水线统一部署顺序
