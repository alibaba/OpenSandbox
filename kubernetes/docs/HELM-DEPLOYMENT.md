# Helm Chart 部署方式

本文档介绍如何使用 Helm Chart 部署 OpenSandbox Controller。

## 前置要求

- Kubernetes 1.22.4+
- Helm 3.0+
- kubectl 已配置并可访问目标集群

## 快速开始

### 方式一: 直接从 GitHub Release 安装 (推荐)

直接下载并安装发布的 Chart 包:

```bash
# 安装最新版本 (0.1.0)
helm install opensandbox-controller \
  https://github.com/alibaba/OpenSandbox/releases/download/helm/opensandbox-controller/0.1.0/opensandbox-controller-0.1.0.tgz \
  --namespace opensandbox-system \
  --create-namespace
```

如需使用自定义镜像:

```bash
helm install opensandbox-controller \
  https://github.com/alibaba/OpenSandbox/releases/download/helm/opensandbox-controller/0.1.0/opensandbox-controller-0.1.0.tgz \
  --set controller.image.repository=<your-registry>/controller \
  --set controller.image.tag=v0.0.1 \
  --namespace opensandbox-system \
  --create-namespace
```

### 方式二: 本地 Chart 安装

如果您从源码构建,可以使用本地 Chart:

#### 1. 构建镜像

首先构建 controller 和 task-executor 镜像:

```bash
# 构建 controller 镜像
cd kubernetes
COMPONENT=controller TAG=v0.0.1 ./build.sh

# 构建 task-executor 镜像
COMPONENT=task-executor TAG=v0.0.1 ./build.sh
```

#### 2. 安装本地 Helm Chart

```bash
helm install opensandbox ./charts/opensandbox-controller \
  --set controller.image.repository=<your-registry>/controller \
  --set controller.image.tag=v0.0.1 \
  --namespace opensandbox-system \
  --create-namespace
```

或者使用 Makefile:

```bash
make helm-install \
  IMAGE_TAG_BASE=<your-registry>/controller \
  VERSION=v0.0.1
```

### 3. 验证安装

```bash
# 检查 Pod 状态
kubectl get pods -n opensandbox-system

# 检查 CRD
kubectl get crd | grep opensandbox

# 查看安装状态
helm status opensandbox-controller -n opensandbox-system

# 查看已安装的 Chart 版本
helm list -n opensandbox-system
```

## 版本管理

### 查看可用版本

访问 GitHub Releases 查看所有可用版本:
https://github.com/alibaba/OpenSandbox/releases

查找以 `helm/opensandbox-controller/` 开头的 tag,如 `helm/opensandbox-controller/0.1.0`

### 升级到指定版本

```bash
# 直接从 GitHub Release 升级
helm upgrade opensandbox-controller \
  https://github.com/alibaba/OpenSandbox/releases/download/helm/opensandbox-controller/0.2.0/opensandbox-controller-0.2.0.tgz \
  --namespace opensandbox-system
```

## 自定义配置

### 使用自定义 values 文件

创建自定义 values 文件 `custom-values.yaml`:

```yaml
controller:
  image:
    repository: myregistry.example.com/opensandbox-controller
    tag: v0.1.0
  
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
    requests:
      cpu: 100m
      memory: 128Mi
  
  logLevel: 5

imagePullSecrets:
  - name: myregistrykey
```

使用自定义配置安装:

```bash
helm install opensandbox ./charts/opensandbox-controller \
  -f custom-values.yaml \
  --namespace opensandbox-system \
  --create-namespace
```

### 常用配置示例

#### 1. 调整资源配置

```bash
helm install opensandbox ./charts/opensandbox-controller \
  --set controller.resources.limits.cpu=1000m \
  --set controller.resources.limits.memory=512Mi \
  --namespace opensandbox-system
```

#### 3. 配置节点亲和性

创建 `affinity-values.yaml`:

```yaml
controller:
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: node-role.kubernetes.io/control-plane
            operator: Exists
```

```bash
helm install opensandbox ./charts/opensandbox-controller \
  -f affinity-values.yaml \
  --namespace opensandbox-system
```

## 升级

### 升级 Helm Release

从 GitHub Release 升级:

```bash
# 升级到指定版本
helm upgrade opensandbox-controller \
  https://github.com/alibaba/OpenSandbox/releases/download/helm/opensandbox-controller/0.2.0/opensandbox-controller-0.2.0.tgz \
  --namespace opensandbox-system
```

从本地 Chart 升级:

```bash
helm upgrade opensandbox-controller ./charts/opensandbox-controller \
  --set controller.image.tag=v0.0.2 \
  --namespace opensandbox-system
```

或使用 Makefile:

```bash
make helm-upgrade VERSION=v0.0.2
```

### 查看升级历史

```bash
helm history opensandbox-controller -n opensandbox-system
```

### 回滚

```bash
# 回滚到上一个版本
helm rollback opensandbox-controller -n opensandbox-system

# 回滚到指定版本
helm rollback opensandbox-controller 1 -n opensandbox-system
```

## 卸载

### 卸载 Helm Release

```bash
helm uninstall opensandbox-controller -n opensandbox-system
```

或使用 Makefile:

```bash
make helm-uninstall
```

**注意**: 默认情况下,CRD 会被保留。如需删除 CRD:

```bash
kubectl delete crd batchsandboxes.sandbox.opensandbox.io
kubectl delete crd pools.sandbox.opensandbox.io
```

### 清理 Namespace

如果要完全清理:

```bash
kubectl delete namespace opensandbox-system
```

## Makefile 命令

项目提供了一系列 Makefile 命令来简化 Helm 操作:

```bash
# 检查 Helm Chart 语法
make helm-lint

# 生成 Kubernetes 清单(不安装)
make helm-template

# 生成清单并显示调试信息
make helm-template-debug

# 打包 Helm Chart
make helm-package

# 安装 Helm Chart
make helm-install

# 升级 Helm Chart
make helm-upgrade

# 卸载 Helm Chart
make helm-uninstall

# 测试已安装的 Chart
make helm-test

# 执行 dry-run 安装
make helm-dry-run

# 执行所有 Helm 相关任务
make helm-all
```

## 验证部署

### 1. 检查 Controller 状态

```bash
kubectl get deployment -n opensandbox-system
kubectl get pods -n opensandbox-system
kubectl logs -n opensandbox-system -l control-plane=controller-manager -f
```

### 2. 验证 CRD

```bash
kubectl get crd batchsandboxes.sandbox.opensandbox.io -o yaml
kubectl get crd pools.sandbox.opensandbox.io -o yaml
```

### 3. 创建测试资源

```bash
# 创建 Pool
kubectl apply -f config/samples/sandbox_v1alpha1_pool.yaml

# 创建 BatchSandbox
kubectl apply -f config/samples/sandbox_v1alpha1_batchsandbox.yaml

# 查看状态
kubectl get pools -n opensandbox-system
kubectl get batchsandboxes -n opensandbox-system
```

## 故障排查

### Chart 验证失败

```bash
# 检查 Chart 语法
make helm-lint

# 查看详细模板输出
make helm-template-debug
```

### Controller 无法启动

```bash
# 查看 Pod 状态
kubectl describe pod -n opensandbox-system -l control-plane=controller-manager

# 查看日志
kubectl logs -n opensandbox-system -l control-plane=controller-manager

# 检查 RBAC 权限
kubectl auth can-i --as=system:serviceaccount:opensandbox-system:opensandbox-opensandbox-controller-controller-manager create pods
```

### 镜像拉取失败

```bash
# 检查镜像配置
helm get values opensandbox -n opensandbox-system

# 添加镜像拉取密钥
kubectl create secret docker-registry myregistrykey \
  --docker-server=<your-registry> \
  --docker-username=<username> \
  --docker-password=<password> \
  -n opensandbox-system

# 使用密钥重新安装
helm upgrade opensandbox ./charts/opensandbox-controller \
  --set imagePullSecrets[0].name=myregistrykey \
  --namespace opensandbox-system
```

## 高级配置

### 多环境部署

为不同环境创建专用的 values 文件:

#### values-dev.yaml
```yaml
controller:
  logLevel: 5
  resources:
    limits:
      cpu: 200m
      memory: 128Mi
```

#### values-prod.yaml
```yaml
controller:
  logLevel: 2
  replicaCount: 3
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: control-plane
            operator: In
            values:
            - controller-manager
        topologyKey: kubernetes.io/hostname
```

部署到不同环境:

```bash
# 开发环境
helm install opensandbox ./charts/opensandbox-controller \
  -f values-dev.yaml \
  --namespace opensandbox-dev

# 生产环境
helm install opensandbox ./charts/opensandbox-controller \
  -f values-prod.yaml \
  --namespace opensandbox-prod
```

## 发布 Helm Chart (维护者使用)

### 自动发布

通过 GitHub Actions 自动发布 Helm Chart:

#### 方式一: 通过 Git Tag 触发

```bash
# 发布 opensandbox-controller chart 版本 0.1.0
git tag helm/opensandbox-controller/0.1.0
git push origin helm/opensandbox-controller/0.1.0
```

Tag 命名规则: `helm/{component}/{version}`
- `helm`: 前缀,表示这是 Helm Chart 发布
- `{component}`: 组件名称,如 `opensandbox-controller`
- `{version}`: 版本号,如 `0.1.0`

这将自动触发 workflow:
1. 解析 tag 获取 component 和 version
2. 更新对应 Chart.yaml 中的版本号
3. 打包 Helm Chart
4. 创建 GitHub Release
5. 发布 .tgz 包到 Release

#### 方式二: 手动触发

1. 访问 GitHub Actions 页面
2. 选择 "Publish Helm Chart" workflow
3. 点击 "Run workflow"
4. 选择 component (如: opensandbox-controller)
5. 输入 chart_version (如: 0.1.0) 和 app_version (如: 0.0.1)
6. 点击运行

### 发布后的 URL 格式

发布后,用户可以通过以下 URL 访问 Helm Chart:

```
https://github.com/alibaba/OpenSandbox/releases/download/helm/{COMPONENT}/{VERSION}/{COMPONENT}-{VERSION}.tgz
```

例如:
```
https://github.com/alibaba/OpenSandbox/releases/download/helm/opensandbox-controller/0.1.0/opensandbox-controller-0.1.0.tgz
```

### 添加新的 Helm Chart 组件

如果需要为新组件添加 Helm Chart 发布支持:

1. 在 `charts/` 目录下创建新组件的 chart 目录
2. 更新 `.github/workflows/publish-helm-chart.yml`:
   - 在 `workflow_dispatch.inputs.component.options` 中添加新组件
   - 在 "Set chart path" step 中添加组件路径映射

示例:
```yaml
# 在 workflow_dispatch inputs 中添加
options:
  - opensandbox-controller
  - new-component  # 新增

# 在 Set chart path step 中添加
if [ "$COMPONENT" == "opensandbox-controller" ]; then
  CHART_PATH="kubernetes/charts/opensandbox-controller"
elif [ "$COMPONENT" == "new-component" ]; then
  CHART_PATH="path/to/new-component/chart"
fi
```

### 本地测试发布流程

在发布前,建议本地测试:

```bash
# 打包 Chart
make helm-package

# 验证打包的 Chart
helm lint opensandbox-controller-*.tgz

# 测试安装
helm install test-release opensandbox-controller-*.tgz \
  --namespace test \
  --create-namespace \
  --dry-run
```

## 参考资料

- [Helm Chart README](charts/opensandbox-controller/README.md) - 完整的参数列表
- [OpenSandbox 文档](README.md) - 项目主文档
- [配置示例](config/samples/) - 资源配置示例
