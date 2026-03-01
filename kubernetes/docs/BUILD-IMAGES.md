# 镜像构建指南

本文档介绍如何构建 OpenSandbox Kubernetes Controller 和 Task Executor 镜像。

## 方式一: 使用构建脚本（推荐）

### 本地构建

```bash
cd kubernetes

# 构建 controller 镜像
COMPONENT=controller TAG=v0.1.0 PUSH=false ./build.sh

# 构建 task-executor 镜像
COMPONENT=task-executor TAG=v0.1.0 PUSH=false ./build.sh
```

### 构建并推送到镜像仓库

```bash
# 确保已登录阿里云 ACR
docker login sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com

# 构建并推送 controller 镜像
COMPONENT=controller TAG=v0.1.0 ./build.sh

# 构建并推送 task-executor 镜像
COMPONENT=task-executor TAG=v0.1.0 ./build.sh
```

### 环境变量说明

- `COMPONENT`: 要构建的组件，可选值: `controller`, `task-executor`
- `TAG`: 镜像标签，默认为 `latest`
- `PUSH`: 是否推送到远程仓库，默认为 `true`

## 方式二: 使用 GitHub Actions

### 手动触发工作流

1. 打开 [Actions 页面](https://github.com/alibaba/OpenSandbox/actions)
2. 选择 "Publish Components Image" 工作流
3. 点击 "Run workflow"
4. 选择组件和镜像标签:
   - Component: 在下拉菜单中选择组件名称
     - Controller: `controller`
     - Task Executor: `task-executor`
   - Image tag: 输入镜像标签，例如 `v0.1.0`
5. 点击 "Run workflow" 开始构建

### 通过 Git Tag 触发（推荐）

创建带有特定前缀的 tag 即可自动触发构建:

```bash
# 构建 controller v0.1.0
git tag k8s/controller/v0.1.0
git push origin k8s/controller/v0.1.0

# 构建 task-executor v0.1.0
git tag k8s/task-executor/v0.1.0
git push origin k8s/task-executor/v0.1.0
```

**Tag 命名规则**: `k8s/<component>/<version>`
- `<component>`: 组件名称 `controller` 或 `task-executor`
- `<version>`: 镜像版本号，例如 `v0.1.0`

## 方式三: 使用 Makefile

```bash
cd kubernetes

# 构建 controller 镜像（仅本地）
make docker-build IMG=myregistry/opensandbox-controller:v0.1.0

# 构建 task-executor 镜像（仅本地）
make docker-build-task-executor TASK_EXECUTOR_IMG=myregistry/opensandbox-task-executor:v0.1.0

# 推送镜像
make docker-push IMG=myregistry/opensandbox-controller:v0.1.0
make docker-push-task-executor TASK_EXECUTOR_IMG=myregistry/opensandbox-task-executor:v0.1.0
```

## 镜像仓库

构建的镜像会推送到以下仓库:

### 阿里云容器镜像服务 (ACR)
- Controller: `sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/controller:<tag>`
- Task Executor: `sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/task-executor:<tag>`

## 多架构支持

构建脚本默认支持以下架构:
- `linux/amd64`
- `linux/arm64`

如需构建其他架构，请修改 `build.sh` 中的 `PLATFORMS` 变量。

## 本地测试

如果只想在本地测试镜像而不推送:

```bash
# 构建本地镜像
COMPONENT=controller TAG=test PUSH=false ./build.sh

# 加载到 kind 集群测试
kind load docker-image opensandbox-controller:test

# 或加载到 minikube 测试
minikube image load opensandbox-controller:test
```

## 故障排查

### 权限问题

如果遇到 Docker 权限问题:
```bash
sudo usermod -aG docker $USER
newgrp docker
```

### Buildx 不可用

确保启用 Docker Buildx:
```bash
docker buildx create --use
docker buildx inspect --bootstrap
```

### 磁盘空间不足

清理 Docker 缓存:
```bash
docker system prune -a
docker builder prune -a
```

## 配置私有镜像仓库

如需使用自己的镜像仓库，修改 `build.sh` 中的仓库地址:

```bash
# 编辑 build.sh
ACR_REPO="your-acr-registry.cr.aliyuncs.com/your-namespace"
```

或者直接在构建时使用环境变量:
```bash
ACR_REPO=myregistry.com/myrepo COMPONENT=controller TAG=v0.1.0 ./build.sh
```
