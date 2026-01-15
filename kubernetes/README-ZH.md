# OpenSandbox Kubernetes控制器

[English](README.md) | [中文](README-ZH.md)

OpenSandbox Kubernetes控制器，通过自定义资源管理沙箱环境。它在Kubernetes集群中提供**自动化沙箱生命周期管理**、**资源池化以实现快速供应**、**批处理沙箱创建**和**可选的任务编排**功能。

## 关键特性

- **灵活的沙箱创建**：在池化和非池化沙箱创建模式之间选择
- **批处理和单个交付**：支持单个沙箱（用于真实用户交互）和批处理沙箱交付（用于高吞吐量智能体强化学习场景）
- **可选任务调度**：集成任务编排，支持可选的分片任务模板以实现异构任务分发和定制化沙箱交付（例如，进程注入）
- **资源池化**：维护预热的资源池以实现快速沙箱供应
- **全面监控**：实时跟踪沙箱和任务状态

## 功能特性

### 批处理沙箱管理
BatchSandbox自定义资源允许您创建和管理多个相同的沙箱环境。主要功能包括：
- **灵活的创建模式**：支持池化（使用资源池）和非池化沙箱创建
- **单个和批处理交付**：根据需要创建单个沙箱（replicas=1）或批处理沙箱（replicas=N）
- **可扩展的副本管理**：通过副本配置轻松控制沙箱实例数量
- **自动过期**：设置TTL（生存时间）以自动清理过期沙箱
- **可选任务调度**：内置任务执行引擎，支持可选任务模板
- **详细状态报告**：关于副本、分配和任务状态的综合指标

### 资源池化
Pool自定义资源维护一个预热的计算资源池，以实现快速沙箱供应：
- 可配置的缓冲区大小（最小和最大）以平衡资源可用性和成本
- 池容量限制以控制总体资源消耗
- 基于需求的自动资源分配和释放
- 实时状态监控，显示总数、已分配和可用资源

### 任务编排
集成的任务管理系统，在沙箱内执行自定义工作负载：
- **可选执行**：任务调度完全可选 - 可以在不带任务的情况下创建沙箱
- **基于进程的任务**：支持在沙箱环境中执行基于进程的任务
- **异构任务分发**：使用shardTaskPatches为批处理中的每个沙箱定制单独的任务

### 高级调度
智能资源管理功能：
- 最小和最大缓冲区设置，以确保资源可用性同时控制成本
- 池范围的容量限制，防止资源耗尽
- 基于需求的自动扩展

## 入门指南

![](images/deploy-example.gif)
### 先决条件
- go版本v1.24.0+
- docker版本17.03+。
- kubectl版本v1.11.3+。
- 访问Kubernetes v1.22.4+集群。

如果您没有Kubernetes集群的访问权限，可以使用[kind](https://kind.sigs.k8s.io/)创建一个本地Kubernetes集群进行测试。Kind在Docker容器中运行Kubernetes节点，使得设置本地开发环境变得容易。

安装kind：
- 从[发布页面](https://github.com/kubernetes-sigs/kind/releases)下载适用于您操作系统的发布二进制文件并将其移动到您的`$PATH`中
- 或使用包管理器：
  - macOS (Homebrew): `brew install kind`
  - Windows (winget): `winget install Kubernetes.kind`

安装kind后，使用以下命令创建集群：
```sh
kind create cluster
```

此命令默认创建单节点集群。要与其交互，请使用生成的kubeconfig运行`kubectl`。

**Kind用户的重要说明**：如果您使用的是kind集群，在使用`make docker-build`构建镜像后，需要将控制器和任务执行器镜像加载到kind节点中。这是因为kind在Docker容器中运行Kubernetes节点，无法直接访问本地Docker守护进程中的镜像。

使用以下命令将镜像加载到kind集群中：
```sh
kind load docker-image <controller-image-name>:<tag>
kind load docker-image <task-executor-image-name>:<tag>
```

例如，如果您使用`make docker-build IMG=my-controller:latest`构建镜像，则使用以下命令加载：
```sh
kind load docker-image my-controller:latest
```

完成后使用以下命令删除集群：
```sh
kind delete cluster
```

有关使用kind的更多详细说明，请参阅[官方kind文档](https://kind.sigs.k8s.io/docs/user/quick-start/)。

### 部署

此项目需要两个独立的镜像 - 一个用于控制器，另一个用于任务执行器组件。

1. **构建和推送您的镜像：**
   ```sh
   # 构建和推送控制器镜像
   make docker-build docker-push IMG=<some-registry>/opensandbox-controller:tag
   
   # 构建和推送任务执行器镜像
   make docker-build-task-executor docker-push-task-executor IMG=<some-registry>/opensandbox-task-executor:tag
   ```

   **注意：** 这些镜像应该发布在您指定的个人注册表中。需要能够从工作环境中拉取镜像。如果上述命令不起作用，请确保您对注册表具有适当的权限。

2. **将CRD安装到集群中：**
   ```sh
   make install
   ```

3. **将管理器部署到集群：**
   ```sh
   make deploy IMG=<some-registry>/opensandbox-controller:tag TASK_EXECUTOR_IMG=<some-registry>/opensandbox-task-executor:tag
   ```

   **注意**：您可能需要授予自己集群管理员权限或以管理员身份登录以确保您在运行命令之前具有集群管理员权限。

**Kind用户的重要说明**：如果您使用的是kind集群，需要在构建镜像后将两个镜像都加载到kind节点中：
```sh
kind load docker-image <controller-image-name>:<tag>
kind load docker-image <task-executor-image-name>:<tag>
```

### 创建BatchSandbox和Pool资源

#### 基础示例
创建一个简单的非池化沙箱，不带任务调度：

```yaml
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: basic-batch-sandbox
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: sandbox-container
        image: nginx:latest
        ports:
        - containerPort: 80
```

应用批处理沙箱配置：
```sh
kubectl apply -f basic-batch-sandbox.yaml
```

检查批处理沙箱状态：
```sh
kubectl get batchsandbox basic-batch-sandbox -o wide
```

示例输出：
```sh
NAME                   DESIRED   TOTAL   ALLOCATED   READY   EXPIRE   AGE
basic-batch-sandbox    2         2       2           2       <none>   5m
```

状态字段说明：
- **DESIRED**：请求的沙箱数量
- **TOTAL**：创建的沙箱总数
- **ALLOCATED**：成功分配的沙箱数量
- **READY**：准备使用的沙箱数量
- **EXPIRE**：过期时间（未设置时为空）
- **AGE**：资源创建以来的时间

沙箱准备好后，您可以在注解中找到端点信息：
```sh
kubectl get batchsandbox basic-batch-sandbox -o jsonpath='{.metadata.annotations.sandbox\.opensandbox\.io/endpoints}'
```

这将显示交付沙箱的IP地址。

#### 高级示例

##### 不带任务的池化沙箱
首先，创建一个资源池：

```yaml
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: Pool
metadata:
  name: example-pool
spec:
  template:
    spec:
      containers:
      - name: sandbox-container
        image: nginx:latest
        ports:
        - containerPort: 80
  capacitySpec:
    bufferMax: 10
    bufferMin: 2
    poolMax: 20
    poolMin: 5
```

应用资源池配置：
```sh
kubectl apply -f pool-example.yaml
```

使用资源池创建一批沙箱：

```yaml
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: pooled-batch-sandbox
spec:
  replicas: 3
  poolRef: example-pool
```

应用批处理沙箱配置：
```sh
kubectl apply -f pooled-batch-sandbox.yaml
```

##### 带异构任务的池化沙箱
创建一批带有基于进程的异构任务的沙箱。为了使任务执行正常工作，任务执行器必须作为sidecar容器部署在资源池模板中，并与沙箱容器共享进程命名空间：

首先，创建一个带有任务执行器sidecar的资源池：

```yaml
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: Pool
metadata:
  name: task-example-pool
spec:
  template:
    spec:
      shareProcessNamespace: true
      containers:
      - name: sandbox-container
        image: ubuntu:latest
        command: ["sleep", "3600"]
      - name: task-executor
        image: <task-executor-image>:<tag>
        securityContext:
          capabilities:
            add: ["SYS_PTRACE"]
  capacitySpec:
    bufferMax: 10
    bufferMin: 2
    poolMax: 20
    poolMin: 5
```

使用我们刚刚创建的资源池创建一批带有基于进程的异构任务的沙箱：

```yaml
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: task-batch-sandbox
spec:
  replicas: 2
  poolRef: task-example-pool
  taskTemplate:
    spec:
      process:
        command: ["echo", "Default task"]
  shardTaskPatches:
  - spec:
      process:
        command: ["echo", "Custom task for sandbox 1"]
  - spec:
      process:
        command: ["echo", "Custom task for sandbox 2"]
        args: ["with", "additional", "arguments"]
```

应用批处理沙箱配置：
```sh
kubectl apply -f task-batch-sandbox.yaml
```

检查带任务的批处理沙箱状态：
```sh
kubectl get batchsandbox task-batch-sandbox -o wide
```

示例输出：
```sh
NAME                   DESIRED   TOTAL   ALLOCATED   READY   TASK_RUNNING   TASK_SUCCEED   TASK_FAILED   TASK_UNKNOWN   EXPIRE   AGE
task-batch-sandbox     2         2       2           2       0              2              0             0              <none>   5m
```

任务状态字段说明：
- **TASK_RUNNING**：当前正在执行的任务数
- **TASK_SUCCEED**：成功完成的任务数
- **TASK_FAILED**：失败的任务数
- **TASK_UNKNOWN**：状态未知的任务数

当您删除带有运行任务的BatchSandbox时，控制器将首先停止所有任务，然后删除BatchSandbox资源。一旦所有任务都成功终止，BatchSandbox将被完全删除，沙箱将返回到资源池中以供重用。

删除BatchSandbox：
```sh
kubectl delete batchsandbox task-batch-sandbox
```

您可以通过观察BatchSandbox状态来监控删除过程：
```sh
kubectl get batchsandbox task-batch-sandbox -w
```

### 监控资源
检查资源池和批处理沙箱的状态：
```sh
# 查看资源池状态
kubectl get pools

# 查看批处理沙箱状态
kubectl get batchsandboxes

# 获取特定资源的详细信息
kubectl describe pool example-pool
kubectl describe batchsandbox example-batch-sandbox
```

## 项目结构

```
├── api/
│   └── v1alpha1/          # 自定义资源定义（BatchSandbox, Pool）
├── cmd/
│   ├── controller/         # 主控制器管理器入口点
│   └── task-executor/     # 任务执行器二进制文件
├── config/
│   ├── crd/               # 自定义资源定义清单
│   ├── default/           # 控制器部署的默认配置
│   ├── manager/           # 控制器管理器配置
│   ├── rbac/              # 基于角色的访问控制清单
│   └── samples/           # 资源的示例YAML清单
├── hack/                  # 开发脚本和工具
├── images/                # 文档图片
├── internal/
│   ├── controller/        # 核心控制器实现
│   ├── scheduler/         # 资源分配和调度逻辑
│   ├── task-executor/     # 任务执行引擎内部实现
│   └── utils/             # 实用函数和助手
├── pkg/
│   └── task-executor/     # 共享的任务执行器包
└── test/                  # 测试套件
```

## 贡献
欢迎为OpenSandbox Kubernetes控制器项目做出贡献。请随时提交问题、功能请求和拉取请求。

**注意：** 运行`make help`以获取所有潜在`make`目标的更多信息

更多信息请参见[Kubebuilder文档](https://book.kubebuilder.io/introduction.html)

## 许可证
此项目在Apache 2.0许可证下开源。

您可以将OpenSandbox用于个人或商业项目，只要遵守许可证条款即可。