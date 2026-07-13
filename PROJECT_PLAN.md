# KubeTask v0.1.0 — 智能云原生分布式任务调度平台

## 项目计划书（优化版）

---

## 一、项目概述

| 项目 | 内容 |
|------|------|
| 项目名称 | KubeTask（智能云原生任务调度系统） |
| 版本 | v0.1.0 |
| 开发语言 | Go 1.23+ |
| 核心目标 | 基于 Kubernetes 构建生产可用、易部署、智能调度的分布式任务调度平台，解决传统 CronJob 在企业中面临的可见性差、运维复杂、调度不智能、缺乏依赖编排等问题。 |
| 项目定位 | **轻量级 + 可扩展 + 学习深度优先**，从零搭建每一行代码，适用于中小团队、边缘计算（k3s）、云原生学习与生产实践 |
| 交付形态 | 开源 CLI / Helm Chart 一键部署 |

### 设计哲学

```
KISS 原则 → 先做精，再做全
稳定可靠 → 优于功能丰富
可观测 → 自带 Metrics + 日志 + 告警
渐进式 → 每个 Phase 都是独立可用的版本
```

---

## 二、核心问题与解决方案

| 问题 | 传统方案 | KubeTask 方案 |
|------|----------|--------------|
| 定时任务无统一视图 | Shell crontab / Jenkins 分散管理 | Web 管理后台 + 统一 Task CRD |
| 任务失败无告警 | 靠人盯着 | 自动重试 + Webhook/钉钉通知 |
| 任务依赖无法编排 | 手写脚本链式调用 | DAG 工作流（YAML 定义） |
| 集群资源浪费 | 固定调度 | 资源水位感知 + 错峰调度 |
| 边缘+云端割裂 | 分别管理 | 多集群统一管理（k3s + ACK） |
| 任务执行不可观测 | 无日志/无监控 | Prometheus Metrics + 实时日志流式查看 |

---

## 三、版本规划（Roadmap）

```
v0.1.0 MVP ──→ v0.2.0 进阶 ──→ v0.3.0 创新 ──→ v1.0.0 生产
  (6周)     (4周)      (4周)      (持续)
```

每个版本都是**独立可交付**的，不依赖后续版本。

---

## 四、功能需求清单（详细版）

### Phase 1：MVP（6 周）— 可交付核心

#### P1.1 项目骨架（第 1 周）
- [ ] 使用 **Kubebuilder v4** 脚手架初始化项目
- [ ] 项目目录结构（按 K8s Operator 标准布局）
- [ ] `Task` CRD 定义
  - 任务类型：`Cron`（定时）、`OneTime`（一次性）、`Delay`（延迟执行）
  - 核心字段：`Image`, `Command`, `Schedule`, `BackoffLimit`, `TTLSecondsAfterFinished`
  - 状态字段：`Phase`, `StartTime`, `CompletionTime`, `Message`
- [ ] 配置管理（命令行 Flag + 配置文件 + 环境变量，Viper）
- [ ] Zap 日志初始化（结构化日志 + 日志级别动态调整）
- [ ] 健康检查 API `/healthz`, `/readyz`

#### P1.2 核心 Controller（第 2-3 周）
- [ ] **Task Reconciler**：Watch Task CR → 创建/更新/删除 Kubernetes Job
- [ ] **Job 状态同步**：Job 状态 → Task 状态实时更新
  - `Pending` → `Running` → `Succeeded` / `Failed`
- [ ] **失败自动重试机制**
  - 指数退避：`2^n * baseDelay`（最大间隔 5 分钟）
  - 可配置最大重试次数（默认 3）
- [ ] **Finalizer 机制**：删除 Task 时自动清理关联 Job
- [ ] **Event 记录**：Task 状态变更写入 K8s Events
- [ ] **Controller 优雅关闭**（Graceful Shutdown）

#### P1.3 REST API（第 3-4 周）
- [ ] **Gin HTTP Server** 与 Controller 同进程运行
- [ ] CRUD API：
  - `POST /api/v1/tasks` — 创建任务
  - `GET /api/v1/tasks` — 列表（分页 + 筛选）
  - `GET /api/v1/tasks/:name` — 详情
  - `PUT /api/v1/tasks/:name` — 更新
  - `DELETE /api/v1/tasks/:name` — 删除
  - `POST /api/v1/tasks/:name/trigger` — 手动触发
- [ ] 任务日志 API：`GET /api/v1/tasks/:name/logs?tail=100` — **流式 SSE 返回 Job Pod 日志**
- [ ] 任务统计 API：`GET /api/v1/stats` — 各状态数量、今日执行次数

#### P1.4 Web 管理界面（第 4-5 周）
- [ ] **技术选择**：Gin 静态文件服务 + 轻量前端（推荐 Svelte 或 Vue3 + Vite）
- [ ] 页面清单：
  | 页面 | 功能 |
  |------|------|
  | 仪表盘 | 任务状态统计卡片（环形图） + 近期执行趋势折线图 |
  | 任务列表 | 表格展示所有 Task（状态标签、调度时间、最后执行时间） |
  | 任务详情 | 基本信息、YAML 展示、执行历史、操作按钮 |
  | 任务创建/编辑 | 表单创建（类型、镜像、命令、调度表达式、重试策略） |
  | 日志查看 | 实时日志流式展示（WebSocket 或 SSE） |
- [ ] 基础 UI 组件：Navbar、Sidebar、表格、表单、状态标签

#### P1.5 日志系统（第 4-5 周）
- [ ] **实时日志**：通过 K8s API `GetLogs()` 流式读取 Pod 日志
- [ ] **日志 API**：支持 `tailLines`（行数）、`sinceSeconds`（时间范围）、`follow`（实时流）
- [ ] **前端日志面板**：自动滚动、行号、日志级别高亮、搜索过滤

#### P1.6 部署与运维（第 5-6 周）
- [ ] **Dockerfile 多阶段构建**
  ```dockerfile
  Stage 1: golang:1.23 构建
  Stage 2: distroless 或 alpine:3.20 运行（~20MB）
  ```
- [ ] **Helm Chart 结构**
  ```
  charts/kubetask/
  ├── Chart.yaml
  ├── values.yaml
  ├── templates/
  │   ├── deployment.yaml
  │   ├── service.yaml
  │   ├── rbac.yaml
  │   ├── crd.yaml
  │   ├── configmap.yaml
  │   └── ingress.yaml
  ```
- [ ] **k3s 一键部署脚本** `deploy/k3s/install.sh`
- [ ] **Makefile 统一入口**
  - `make build` — 编译
  - `make docker-build` — 构建镜像
  - `make deploy` — Helm 部署
  - `make test` — 运行测试
  - `make lint` — 代码检查

#### P1.7 测试（贯穿全程）
- [ ] **单元测试**：Controller Reconciler、API Handler（envtest + httptest）
- [ ] **集成测试**：在 Kind / k3s 中创建真实 CR → 验证 Job 创建
- [ ] **End-to-End 测试**：API 创建任务 → 等待执行完成 → 验证状态 → 读取日志

---

### Phase 2：进阶功能（4 周，v0.2.0）

#### P2.1 DAG 工作流编排（2 周）
- [ ] **Workflow CRD**
  ```yaml
  apiVersion: kubetask.io/v1
  kind: Workflow
  spec:
    entrypoint: main
    templates:
    - name: main
      dag:
        tasks:
        - name: build
          template: build-job
        - name: test
          template: test-job
          dependencies: [build]
        - name: deploy
          template: deploy-job
          dependencies: [test]
    - name: build-job
      taskRef:
        name: build-task
  ```
- [ ] **Workflow Controller**：DAG 拓扑排序 → 按依赖关系依次/并行创建 Task
- [ ] **状态机**：`Pending → Running → (Succeeded | Failed | Skipped)`
- [ ] **条件分支**：依赖任务成功/失败触发不同下游
- [ ] **前端工作流视图**：DAG 图形化展示（使用 SvelteFlow 或 Dagre 布局）

#### P2.2 多租户与认证（1 周）
- [ ] **基于 K8s ServiceAccount + RBAC 的认证**
- [ ] **Namespace 隔离**：每个租户独立命名空间
- [ ] **API Key 认证**：用于 CI/CD 系统集成
- [ ] **操作审计**：关键操作记录到 K8s Events

#### P2.3 告警通知（1 周）
- [ ] **Webhook 通知**：任务失败/超时 → 发送 JSON Payload
- [ ] **内置通知渠道**
  - 钉钉机器人
  - 企业微信机器人
  - Slack Webhook
- [ ] **通知模板**：可自定义消息格式

#### P2.4 调度增强（1 周）
- [ ] **资源感知调度**：根据集群节点 CPU/Mem 负载选择执行节点
- [ ] **任务超时控制**：`ActiveDeadlineSeconds`
- [ ] **并发策略**：`Allow` / `Forbid` / `Replace`

---

### Phase 3：创新特性（4 周，v0.3.0）

#### P3.1 多集群管理（2 周）
- [ ] **Cluster CRD**：注册远程集群
  ```yaml
  apiVersion: kubetask.io/v1
  kind: Cluster
  spec:
    name: edge-k3s
    kubeconfig: ...  # 或 SecretRef
    type: k3s | ack | generic
  ```
- [ ] **多集群调度**：Task 指定 `clusterName` 或在集群间调度
- [ ] **云边协同**：边缘（k3s）+ 云端（ACK）统一管理

#### P3.2 智能错峰调度（1 周）
- [ ] **历史数据采集**：记录每次执行时长、资源消耗、时间段
- [ ] **负载预测**：基于 P50/P90 统计 + 简单指数平滑预测高峰期
- [ ] **错峰策略**：
  - 预测到高峰期 → 自动偏移执行时间（±30min 窗口内）
  - 节点负载 > 80% → 等待或迁移到低负载节点
- [ ] **调度建议**：Web UI 显示"建议执行时间"

#### P3.3 可观测性增强（1 周）
- [ ] **Prometheus Metrics**（默认暴露 `:8080/metrics`）
  | Metric | 类型 | 含义 |
  |--------|------|------|
  | `kubetask_task_total` | Counter | 任务创建总数 |
  | `kubetask_task_duration_seconds` | Histogram | 任务执行时长 |
  | `kubetask_task_status` | GaugeVec | 当前各状态任务数 |
  | `kubetask_controller_errors` | Counter | Controller 错误次数 |
- [ ] **Grafana Dashboard JSON**（预置模板）

#### P3.4 Serverless 任务模式（可选，1 周）
- [ ] **Knative Job 支持**：Task 可以选择以 Knative Job 方式运行
- [ ] **Serverless CRD 扩展字段**
  ```yaml
  spec:
    executionMode: Job | Knative | ECI
    resources:
      requests:
        cpu: "1"
        memory: "1Gi"
  ```

---

## 五、技术架构设计

### 整体架构图

```
┌─────────────────────────────────────────────────────────┐
│                    用户交互层                            │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐ │
│  │ Web UI      │  │ REST API     │  │ kubectl / CLI  │ │
│  │ (Svelte)    │  │ (Gin)        │  │ (kubetask)     │ │
│  └──────┬──────┘  └──────┬───────┘  └───────┬────────┘ │
└─────────┼────────────────┼──────────────────┼──────────┘
          │                │                  │
┌─────────┼────────────────┼──────────────────┼──────────┐
│         ▼                ▼                  ▼          │
│  ┌───────────────────────────────────────────────────┐ │
│  │              Core Engine (Controller)              │ │
│  │  ┌────────────┐  ┌──────────┐  ┌──────────────┐  │ │
│  │  │ Reconciler │  │ Scheduler│  │ AI Predictor │  │ │
│  │  │ (Task→Job) │  │ (Cron)   │  │ (轻量预测)   │  │ │
│  │  └────────────┘  └──────────┘  └──────────────┘  │ │
│  └───────────────────────────────────────────────────┘ │
│                         │                              │
│  ┌──────────────────────┼──────────────────────────┐   │
│  │         ▼            ▼              ▼           │   │
│  │  ┌────────┐  ┌──────────┐  ┌────────────────┐  │   │
│  │  │ K8s    │  │ PostgreSQL│  │ Prometheus     │  │   │
│  │  │ etcd   │  │ (tasks)  │  │ (metrics)      │  │   │
│  │  └────────┘  └──────────┘  └────────────────┘  │   │
│  └─────────────────────────────────────────────────┘   │
│                         │                              │
│  ┌──────────────────────┼──────────────────────────┐   │
│  │         ▼            ▼              ▼           │   │
│  │  ┌────────┐  ┌──────────┐  ┌────────────────┐  │   │
│  │  │ k3s    │  │ ACK      │  │ Serverless     │  │   │
│  │  │ (边缘) │  │ (云端)   │  │ (Knative/ECI)  │  │   │
│  │  └────────┘  └──────────┘  └────────────────┘  │   │
│  └─────────────────────────────────────────────────┘   │
│                    基础设施层                           │
└─────────────────────────────────────────────────────────┘
```

### 关键技术选型

| 领域 | 选择 | 理由 |
|------|------|------|
| Go 框架 | controller-runtime + client-go | K8s Operator 标准，生态最强 |
| HTTP 框架 | Gin v1.x | 高性能、中间件生态好 |
| ORM | GORM v2 | 成熟稳定，支持 PostgreSQL |
| 数据库 | PostgreSQL 15+ | 任务元数据持久化 |
| 前端 | Svelte 5 + SvelteFlow | 编译型框架，包体小，构建快 |
| 图表 | ECharts 5 | 功能最全的可视化库 |
| 日志 | Zap | 高性能结构化日志 |
| 指标 | Prometheus Client | 云原生标准 |
| 部署 | Helm v3 | K8s 标准包管理 |
| 开发集群 | k3s (单节点) | 资源占用低，启动快 |
| CRD 脚手架 | Kubebuilder v4 | 代码自动生成，减少 60% 样板代码 |
| 测试框架 | ginkgo + envtest | K8s Controller 测试标准 |

### 核心数据模型

#### Task CRD

```go
// TaskSpec 定义任务期望状态
type TaskSpec struct {
    // 任务类型：Cron | OneTime | Delay
    Type TaskType `json:"type"`

    // Cron 表达式 (Type=Cron 时必填)
    // +optional
    Schedule string `json:"schedule,omitempty"`

    // 延迟执行时长 (Type=Delay 时必填)
    // +optional
    Delay metav1.Duration `json:"delay,omitempty"`

    // 任务镜像
    Image string `json:"image"`

    // 执行命令
    // +optional
    Command []string `json:"command,omitempty"`

    // 资源限制
    // +optional
    Resources corev1.ResourceRequirements `json:"resources,omitempty"`

    // 环境变量
    // +optional
    Env []corev1.EnvVar `json:"env,omitempty"`

    // 失败重试次数 (默认 3)
    // +optional
    BackoffLimit *int32 `json:"backoffLimit,omitempty"`

    // 任务超时时间
    // +optional
    ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`

    // 并发策略：Allow | Forbid | Replace
    // +optional
    ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

    // 目标集群名称（多集群时使用）
    // +optional
    ClusterName string `json:"clusterName,omitempty"`
}

// TaskStatus 定义任务实际状态
type TaskStatus struct {
    // 当前阶段
    Phase TaskPhase `json:"phase"`

    // 上次调度时间
    // +optional
    LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

    // 上次执行开始时间
    // +optional
    LastStartTime *metav1.Time `json:"lastStartTime,omitempty"`

    // 上次执行完成时间
    // +optional
    LastCompletionTime *metav1.Time `json:"lastCompletionTime,omitempty"`

    // 执行历史摘要
    // +optional
    ExecutionHistory []ExecutionRecord `json:"executionHistory,omitempty"`

    // 关联 Job 名称列表
    // +optional
    ActiveJobs []string `json:"activeJobs,omitempty"`

    // 失败次数
    // +optional
    Failed int32 `json:"failed"`

    // 成功次数
    // +optional
    Succeeded int32 `json:"succeeded"`
}

type TaskPhase string
const (
    TaskPending   TaskPhase = "Pending"
    TaskRunning   TaskPhase = "Running"
    TaskSucceeded TaskPhase = "Succeeded"
    TaskFailed    TaskPhase = "Failed"
    TaskSuspended TaskPhase = "Suspended"
)
```

#### Workflow CRD（Phase 2）

```go
type WorkflowSpec struct {
    Entrypoint string              `json:"entrypoint"`
    Templates  []WorkflowTemplate  `json:"templates"`
}

type WorkflowTemplate struct {
    Name string   `json:"name"`
    DAG  *DAGSpec `json:"dag,omitempty"`
    Task *TaskRef `json:"task,omitempty"`  // 引用已有 Task
}

type DAGSpec struct {
    Tasks []DAGTask `json:"tasks"`
}

type DAGTask struct {
    Name         string   `json:"name"`
    Template     string   `json:"template"`
    Dependencies []string `json:"dependencies,omitempty"` // 前置依赖
}
```

---

## 六、项目目录结构

```
kubetask/
├── cmd/
│   └── manager/
│       └── main.go            # 入口: 启动 Controller + HTTP Server
├── api/
│   └── v1/
│       ├── task_types.go      # Task CRD 类型定义
│       ├── workflow_types.go  # Workflow CRD 类型定义
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go  # 自动生成
├── internal/
│   ├── controller/
│   │   ├── task_controller.go        # Task Reconciler
│   │   ├── task_controller_test.go
│   │   ├── workflow_controller.go    # Workflow Reconciler
│   │   └── workflow_controller_test.go
│   ├── api/
│   │   ├── router.go            # Gin 路由定义
│   │   ├── handler/
│   │   │   ├── task_handler.go
│   │   │   ├── stats_handler.go
│   │   │   └── log_handler.go
│   │   └── middleware/
│   │       ├── auth.go
│   │       └── logging.go
│   ├── scheduler/
│   │   ├── cron_scheduler.go    # Cron 触发器
│   │   └── delay_scheduler.go   # 延迟任务调度
│   ├── ai/
│   │   ├── predictor.go         # 时间序列预测
│   │   ├── history.go           # 历史数据采集
│   │   └── scheduler.go         # 智能调度引擎
│   ├── storage/
│   │   ├── db.go                # PostgreSQL 连接
│   │   └── migrations/          # 数据库迁移
│   ├── logger/
│   │   └── logger.go            # Zap 初始化
│   ├── config/
│   │   └── config.go            # Viper 配置管理
│   └── monitor/
│       ├── metrics.go           # Prometheus Metrics
│       └── health.go            # 健康检查
├── web/                          # 前端源码
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   └── App.svelte
│   ├── package.json
│   └── vite.config.js
├── charts/
│   └── kubetask/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│           ├── deployment.yaml
│           ├── service.yaml
│           ├── rbac.yaml
│           ├── crd.yaml
│           └── configmap.yaml
├── deploy/
│   ├── docker/
│   │   └── Dockerfile
│   └── k3s/
│       └── install.sh
├── config/
│   ├── manager/
│   │   └── manager.yaml        # K8s 部署配置
│   └── rbac/
│       ├── role.yaml
│       ├── role_binding.yaml
│       └── service_account.yaml
├── test/
│   ├── integration/
│   │   └── task_test.go        # 集成测试
│   └── e2e/
│       └── task_e2e_test.go    # E2E 测试
├── docs/
│   ├── architecture.md
│   ├── api.md
│   └── deployment.md
├── go.mod
├── go.sum
├── Makefile
└── PROJECT (Kubebuilder 项目元文件)
```

---

## 七、API 设计

### RESTful API 接口

#### Task 管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/tasks` | 创建任务 |
| `GET` | `/api/v1/tasks` | 任务列表（分页） |
| `GET` | `/api/v1/tasks/:name` | 任务详情 |
| `PUT` | `/api/v1/tasks/:name` | 更新任务 |
| `DELETE` | `/api/v1/tasks/:name` | 删除任务 |
| `POST` | `/api/v1/tasks/:name/trigger` | 手动触发执行 |
| `POST` | `/api/v1/tasks/:name/suspend` | 暂停任务 |
| `POST` | `/api/v1/tasks/:name/resume` | 恢复任务 |
| `GET` | `/api/v1/tasks/:name/logs` | 流式日志（SSE） |

#### Workflow 管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/workflows` | 创建工作流 |
| `GET` | `/api/v1/workflows` | 工作流列表 |
| `GET` | `/api/v1/workflows/:name` | 工作流详情（含 DAG 图数据） |
| `DELETE` | `/api/v1/workflows/:name` | 删除工作流 |

#### 统计与监控

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/v1/stats` | 任务统计（状态分布） |
| `GET` | `/api/v1/stats/trend` | 近期执行趋势 |
| `GET` | `/api/v1/healthz` | 健康检查 |
| `GET` | `/api/v1/readyz` | 就绪检查 |

---

## 八、开发计划（6+4+4=14 周）

### Phase 1：MVP（6 周）

| 周 | 阶段 | 任务 | 交付物 | 里程碑 |
|----|------|------|--------|--------|
| 1 | 项目初始化 | Kubebuilder 脚手架、CRD 定义、配置管理、日志 | 项目骨架 + Task CRD | ✅ 可编译运行 |
| 2 | Controller 核心 | Reconciler 逻辑、Job 创建/同步、Finalizer、Event | 基本 Controller | ✅ 创建 CR → 自动创建 Job |
| 3 | 重试与容错 | 失败重试（指数退避）、优雅关闭、错误处理 | 稳定的 Controller | ✅ 失败自动重试 3 次 |
| 4 | REST API | CRUD 端点、Stats 端点、中间件 | API 可调通 | ✅ curl 可管理任务 |
| 5 | Web UI | 仪表盘、任务列表、创建表单、日志查看 | 完整可操作 UI | ✅ 浏览器可用 |
| 6 | 部署与测试 | Dockerfile、Helm Chart、Makefile、集成测试 | MVP 可部署 | ✅ Helm install 即用 |

### Phase 2：进阶功能（4 周）

| 周 | 任务 | 交付物 |
|----|------|--------|
| 7 | Workflow CRD + Controller | DAG 解析 + 拓扑排序 + 任务编排 |
| 8 | Workflow 前端视图 | DAG 图形化显示 + 状态展示 |
| 9 | 多租户 + RBAC + 认证 | Namespace 隔离、API Key |
| 10 | 告警通知 + 调度增强 | Webhook/钉钉/企微通知、资源感知调度 |

### Phase 3：创新特性（4 周）

| 周 | 任务 | 交付物 |
|----|------|--------|
| 11 | 多集群管理 | Cluster CRD + 跨集群调度 |
| 12 | 智能错峰调度 | 历史采集 + P50/P90 统计 + 自动偏移 |
| 13 | 可观测性 | Prometheus Metrics + Grafana 模板 |
| 14 | 文档 + 演示 | API 文档、部署手册、演示视频 |

---

## 九、交付物清单

### 代码交付物
- [ ] Go 源码（Kubebuilder 规范）
- [ ] Task CRD YAML
- [ ] Workflow CRD YAML（Phase 2）
- [ ] Controller Reconciler 实现
- [ ] Gin REST API 实现
- [ ] Svelte Web UI 源码
- [ ] PostgreSQL 迁移脚本
- [ ] Dockerfile（多阶段构建）
- [ ] Helm Chart
- [ ] k3s 部署脚本

### 文档交付物
- [ ] `README.md` — 项目简介、快速开始、功能清单
- [ ] `docs/architecture.md` — 架构设计文档
- [ ] `docs/api.md` — API 接口文档（含示例）
- [ ] `docs/deployment.md` — 部署手册（k3s / ACK）
- [ ] `docs/development.md` — 本地开发指南
- [ ] `CHANGELOG.md` — 版本变更记录

### 运维交付物
- [ ] Prometheus 指标定义文档
- [ ] Grafana Dashboard JSON
- [ ] 告警规则示例（PrometheusRule）

---

## 十、技术风险与缓解措施

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| Client-Go Watch 断连 | 中 | 高 | 使用 controller-runtime 内置重连，增加 Watch 健康检查 |
| PostgreSQL 连接泄漏 | 低 | 高 | 使用 GORM 连接池 + 定期 `Ping` 检查 |
| Cron 表达式解析歧义 | 中 | 中 | 使用 `robfig/cron` v3 标准库，前端提供表达式验证 + 下次执行时间预览 |
| 任务执行耗时过长 | 中 | 中 | `ActiveDeadlineSeconds` 硬限制 + 超时告警 |
| 多集群网络不通 | 中 | 高 | Cluster 状态探活 + 失败自动 Failover |
| 预测模型不准确 | 高 | 低 | 异步模型（先有数据再预测），默认使用统计阈值 |
| 前端拖拽 DAG 复杂度 | 中 | 中 | Phase 2 先做 YAML 定义 + 自动布局，拖拽放 Phase 3 |

---

## 十一、部署方案

### 开发环境
```
集群: k3s (单节点，docker 驱动)
工具: kubectl + helm + kubebuilder
数据库: PostgreSQL 15 (Docker 运行)
```

### 生产建议
```
集群: 多节点 k3s / ACK 托管集群
高可用: Controller 副本数 ≥ 2 (leader-election)
数据库: 云 RDS PostgreSQL
监控: Prometheus + Grafana (或阿里云 SLS)
CI/CD: GitHub Actions + ArgoCD (GitOps)
```

### 一键部署（k3s 本地）
```bash
# 1. 创建数据库
docker run -d --name pg -e POSTGRES_USER=kubetask \
  -e POSTGRES_PASSWORD=kubetask -e POSTGRES_DB=kubetask \
  -p 5432:5432 postgres:15

# 2. 安装 KubeTask
helm install kubetask ./charts/kubetask \
  --set database.host=host.docker.internal

# 3. 访问 UI
kubectl port-forward svc/kubetask 8080:80

# 4. 创建测试任务
kubectl apply -f examples/cron-task.yaml
```

---

## 十二、简历价值

### 可写入简历的内容

> 自主设计并开发云原生智能任务调度平台 **KubeTask**，基于 Go + Kubernetes Operator 模式实现：
>
> - **CRD + Controller**：设计 Task/Workflow 自定义资源，使用 controller-runtime 实现声明式任务编排
> - **智能调度**：基于历史执行数据的时间序列分析实现错峰调度
> - **多集群管理**：支持 k3s 边缘集群与 ACK 云端集群统一调度
> - **DAG 工作流**：支持任务依赖编排与可视化展示
> - **可观测性**：Prometheus Metrics + Grafana + 实时日志流式查看
> - **部署运维**：Helm Chart + k3s 一键部署，生产可用
>
> 技术栈：Go、Kubernetes Operator、controller-runtime、Client-Go、Gin、GORM、PostgreSQL、Svelte、Docker、Helm、Prometheus

### 体现的技术能力

| 能力 | 证明 |
|------|------|
| Go 云原生开发 | CRD + Controller + Operator 完整实现 |
| K8s 资源管控 | Job/Pod 生命周期管理、Event 系统、RBAC |
| 系统设计 | 多集群、高可用、可扩展架构设计 |
| 前后端全栈 | Gin API + Svelte UI + ECharts 可视化 |
| DevOps | Docker 多阶段构建、Helm Chart、GitOps |
| 算法应用 | 时间序列预测、拓扑排序、负载均衡 |

---

## 十三、附录：MVP 验收标准

### 功能验收

| 功能 | 验收标准 |
|------|---------|
| 创建 Cron 任务 | API/UI 创建 → K8s Job 按计划自动创建 → 执行完成状态更新 |
| 创建一次性任务 | 立即执行 → 完成后状态为 Succeeded/Failed |
| 创建延迟任务 | 延迟指定时间后执行 |
| 任务失败重试 | 失败后等待指数退避 → 自动重试 → 达到最大次数后标记 Failed |
| 删除任务 | 删除 CR → 关联 Job 自动清理 |
| 手动触发 | 强制触发任务立即执行一次 |
| 日志查看 | 流式读取 Pod 日志，前端实时展示 |
| 仪表盘 | 统计卡片数据正确，图表能渲染 |

### 非功能验收

| 指标 | 要求 |
|------|------|
| 部署时间 | `helm install` → Ready 状态 < 30s |
| Controller 启动 | 从启动到开始 Reconcile < 5s |
| 创建任务延迟 | CR 创建 → Job 创建 < 3s |
| 镜像大小 | < 50MB (distroless) |
| 内存占用 | Controller < 100MB (空闲) |
| API 响应时间 | P99 < 500ms |

---

## 十四、说明

- 本计划书中的 Phase 1（MVP）为**最优先交付**，完成后即可用于生产
- Phase 2 和 Phase 3 为增值特性，可根据实际需求调整优先级
- 所有时间估算是基于**全职 1 人开发**的基准
- 建议开发过程中使用 **GitHub Projects + Milestone** 管理进度

---

> **版本**: v0.1.0 | **最后更新**: 2026-06-02
