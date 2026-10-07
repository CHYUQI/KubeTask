# kubetask - AI Agent Guide

## Actual Project Structure

```
kubetask/
├── cmd/main.go                         ← 程序入口 (Controller + Gin HTTP 同进程)
├── api/v1/
│   ├── task_types.go                   ← Task CRD 定义 (+kubebuilder 标记)
│   ├── groupversion_info.go            ← GVK Scheme 注册 (自动生成)
│   └── zz_generated.deepcopy.go        ← DeepCopy 方法 (自动生成, DO NOT EDIT)
├── internal/
│   ├── config/config.go                ← Viper 三层配置 (Flag → YAML → EnvVar)
│   ├── logger/logger.go                ← Zap Logger (AtomicLevel 动态改级别)
│   ├── controller/
│   │   ├── task_controller.go          ← Task Reconciler (3 种类型 + Finalizer)
│   │   ├── task_controller_test.go     ← Controller 单元测试 (envtest)
│   │   └── suite_test.go               ← envtest 入口 (etcd + kube-apiserver)
│   └── api/
│       ├── router.go                   ← Gin 路由注册 (10 个端点)
│       └── handler/
│           ├── task_handler.go         ← Task CRUD + Stats + Trend handlers
│           ├── log_handler.go          ← SSE 流式日志 (需 kubernetes.Interface)
│           └── handler_suite_test.go   ← API 单元测试 (httptest + envtest)
├── config/
│   ├── crd/bases/*.yaml                ← CRD YAML (自动生成, DO NOT EDIT)
│   ├── rbac/role.yaml                  ← RBAC ClusterRole (自动生成, DO NOT EDIT)
│   └── ...
├── bin/k8s/                            ← envtest 二进制 (etcd, kube-apiserver, kubectl)
├── Makefile                            ← 标准 Makefile (Windows 下不可用)
├── Dockerfile
├── PROJECT                             ← Kubebuilder 元数据 (自动生成, DO NOT EDIT)
├── go.mod / go.sum
└── PROJECT_PLAN.md                     ← 项目计划书
```

## Critical Rules

### Never Edit These (Auto-Generated)
- `config/crd/bases/*.yaml` — 由 controller-gen 生成
- `config/rbac/role.yaml` — 由 controller-gen 生成
- `**/zz_generated.*.go` — DeepCopy 方法
- `PROJECT` — Kubebuilder 元数据

### Never Remove Scaffold Markers
保留所有 `// +kubebuilder:scaffold:*` 注释。Kubebuilder CLI 在这些标记处注入代码。

### Dependencies & Versions
- Go 1.25+ (当前 1.25.7，Kubebuilder 会自动升级 toolchain)
- Kubebuilder v4.14.0
- controller-gen v0.21.0
- controller-runtime v0.23.3
- K8s API v0.35.0 (envtest K8s v1.35.0)
- Gin v1.12.0, Viper v1.21.0, Zap v1.27.0, robfig/cron v3.0.1

---

## Windows-Specific Workflow (CRITICAL)

### `make` is NOT available
所有 `make` 命令必须替换为直接调用：

```bash
# 替代 make manifests + make generate:
$gopath = $(go env GOPATH)
& "$gopath\bin\controller-gen" object rbac:roleName=manager-role crd `
  paths="kubetask.io/kubetask/api/v1/...;kubetask.io/kubetask/internal/controller/..." `
  output:crd:artifacts:config=config/crd/bases

# 替代 make build:
go build -o bin/manager.exe ./cmd/

# 替代 make test:
$env:KUBEBUILDER_ASSETS = "bin/k8s/k8s/1.35.0-windows-amd64"
go test ./... -count=1

# 替代 make lint-fix / make vet:
go vet ./...
```

### controller-gen path格式
**`./...` 在 Windows 上不工作！** 必须使用完整的 Go module 路径 + 分号分隔：

```
✅ paths="kubetask.io/kubetask/api/v1/...;kubetask.io/kubetask/internal/controller/..."
❌ paths="./..."  或  paths="./api/v1/...,./internal/controller/..."
```

### kubebuilder create api 必须加 `--make=false`
```bash
kubebuilder create api --group kubetask --version v1 --kind Task --make=false
```
否则 CLI 会尝试运行 `make generate` 然后报错 `make: executable file not found`。

### envtest 初始化
```bash
# 1. 安装 setup-envtest
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.23

# 2. 下载 K8s 二进制
& "$(go env GOPATH)\bin\setup-envtest" use 1.35.0 --bin-dir bin/k8s -p path

# 3. 测试前设置环境变量
$env:KUBEBUILDER_ASSETS = "<project>\bin\k8s\k8s\1.35.0-windows-amd64"
```

### envtest 在 Windows 上必须用 testutil 清理
controller-runtime 的 `testEnv.Stop()` 内部使用 SIGTERM，而 Go 在 Windows 上不支持 SIGTERM，
直接调用会静默失败并泄漏 etcd / kube-apiserver 进程。所有 envtest 套件统一使用
`internal/testutil`：

```go
// BeforeSuite: 先清掉上次异常退出残留的 envtest 进程
testutil.KillOrphanedEnvTestProcesses(filepath.Join("..", "..", "bin", "k8s"))

// AfterSuite: 正常停止并强制清理 Windows 子进程
var _ = AfterSuite(func() {
    cancel()
    Expect(testutil.StopEnvTest(testEnv)).To(Succeed())
})
```

注意：`KillOrphanedEnvTestProcesses` 只清理父进程已退出、且可执行文件位于
`bin/k8s`（或 `KUBEBUILDER_ASSETS`）下的 etcd / kube-apiserver，不会误杀并行测试套件
或本机真实集群的进程。

### CRD 路径规则
`go test` 的工作目录是**测试文件所在的包目录**，不是项目根目录：

| 测试文件位置 | CRD 路径 |
|-------------|---------|
| `internal/controller/` | `filepath.Join("..", "..", "config", "crd", "bases")` |
| `internal/api/handler/` | `filepath.Join("..", "..", "..", "config", "crd", "bases")` |

---

## Verification Pipeline (替代 make)

每次编辑后按顺序执行：

```bash
# 1. 编译
go build ./...

# 2. 静态分析
go vet ./...

# 3. 重新生成 CRD/RBAC/DeepCopy
$gopath = $(go env GOPATH)
& "$gopath\bin\controller-gen" object rbac:roleName=manager-role crd `
  paths="kubetask.io/kubetask/api/v1/...;kubetask.io/kubetask/internal/controller/..." `
  output:crd:artifacts:config=config/crd/bases

# 4. 再次编译 (确保生成物正确)
go build ./...

# 5. 运行所有测试
$env:KUBEBUILDER_ASSETS = "bin/k8s/k8s/1.35.0-windows-amd64"
go test ./... -count=1
```

---

## Architecture Patterns

### Gin HTTP Server 与 Controller 同进程
```go
// cmd/main.go
clientset := kubernetes.NewForConfig(mgr.GetConfig())
router := api.NewRouter(mgr.GetClient(), clientset, apiAddr)

go func() { router.Run() }()   // HTTP 在 goroutine 中
mgr.Start(...)                  // Controller 阻塞主 goroutine
```

### Handler 依赖注入
- `TaskHandler` 需要 `client.Client` (controller-runtime)
- `LogHandler` 需要 `client.Client` + `kubernetes.Interface` (Pod 日志流)
- 两个客户端都从 `mgr.GetConfig()` 创建

### 测试架构
- Controller 测试: Ginkgo + envtest + 直接调用 `Reconcile()`
- API 测试: Ginkgo + envtest + Gin httptest + HTTP 请求
- 每个测试用例使用**唯一的 Task 名称** + `defer cleanupTask(name)` 避免状态污染
- 共享 envtest 实例 (BeforeSuite 启动一次, AfterSuite 关闭)

### Task CRD 关键设计
- **scope: Cluster** — Task 不使用 namespace
- **Job 需要 namespace** — 硬编码为 `"default"`，后续可配置
- **三种类型**: OneTime (一次性), Cron (定时), Delay (延迟)
- **Finalizer**: 保证删除 Task 时清理关联 Job
- **OwnerReference**: Job → Task 的归属关系 (SetControllerReference)
- **Status 子资源**: Controller 用 `Status().Update()`，API Handler 用 `Update()`

### API 端点清单

端点、请求/响应结构、错误码和示例统一维护在 **[docs/API.md](docs/API.md)**（Task + Workflow + 统计 + SSE 日志），
本文档不再内嵌接口清单。

- 事实来源是 `internal/api/router.go`；改路由或响应结构时必须同步更新 `docs/API.md`。
- 新增端点时同时补 handler 测试（`internal/api/handler/*_test.go`）。

---

## 判断原则：以已有的 CRD 与 Controller 为准

- **唯一事实来源是代码**：判断系统行为时以 `api/v1/*_types.go` 的 CRD 定义和 `internal/controller/` 的实现为准。
  计划书、README、API 文档与代码冲突时以代码为准，并在报告里指出文档需要更新，不要反过来按文档改代码。
- **先读实现、再下结论**：任何"冲突 / 不一致 / 潜在 bug"的判断都要落到具体字段和代码路径上，
  给出触发条件（什么输入、走到哪个分支、产生什么结果）。没有代码或实测证据的一律标注"待验证"，不要写成结论。
- **不夸大问题**：严格区分"理论上的不一致"与"真实会改变运行行为"。
  默认假设现有实现是正确且可用的；只处理已确认会真实发生的问题。
- **不引入多余机制**：不为假想场景新增 CEL / VAP / webhook / CRD 字段 / 新资源类型。
  确有必要时必须先说明代价与备选方案，等维护者确认后再动。
- **不迁就前端**：不能因为 UI 展示方便而修改后端的接口语义或领域逻辑。UI 适配后端，不是反过来。
- **上报格式**：现象 → 代码路径 → 是否真的会触发 → 最小改法 → 备选方案；一轮只推进一个已确认的问题。

### 已确认并接受的差异（不要再当作缺陷重复上报）

- Workflow 的子 Task 出现在 Task 列表 / 统计 / 趋势里是**预期行为**：子 Task 本身就是 Task。
- **改子 Task 的 spec 不会改变运行行为**：`handleOneTime` 在 Job 已存在时只做 `syncJobStatus`，
  任务已终态且没有 Job 时直接跳过；spec 变化只停留在对象字段层面。
- **子 Task 被 suspend 会自愈**：`handleSuspend` 只把 phase 置为 `Suspended` 后早退；
  resume 后 phase 复位 `Pending`，下一轮 `handleOneTime` 走 `syncJobStatus` 正常推进。
- **唯一会真实改变行为的是 trigger**（`kubetask.io/last-trigger` 注解，旧 Job 已被清理时会再跑一次）。
  当前决定不修改：语义上"子 Task 是某次运行的记录，要重跑就重新跑一次 Workflow"。
- **Task 名长度（Job/Pod 名限制）由 Controller 兜底，API 层不校验**：`TaskReconciler` 对超过
  `internal/naming.MaxTaskNameLength`（46）的名字直接置 `Failed` 且不创建 Job；
  Workflow 子 Task 名是拼出来的，仍在创建 Workflow 时于入口校验（`internal/workflow.ValidateChildTaskNames`）。
- 长度上限只在 `internal/naming` 定义一份（`MaxTaskNameLength`），Task 侧和编排侧都引用它，不要复制第二份。

---

## Agent Workflow

### Step 1: Explore Before Editing
- 使用 `grep` 和 `glob` 找相关代码
- 读周围文件理解风格
- 识别哪些是自动生成文件

### Step 2: Make Changes (One Logical Unit at a Time)
- 不批量改不相关的代码
- 使用 kubebuilder CLI 创建新 API (不要手建文件)
- 先备份自定义逻辑再 `--force`

### Step 3: Verify After Each Edit
- 编辑 `*_types.go`: 跑 controller-gen 重新生成
- 编辑任何 `.go`: `go build ./...` + `go vet ./...` + `go test ./...`
- 测试失败必须先修，再做其他改动

### Step 4: Final Verification
完整流水线: `go build ./... ; go vet ./... ; controller-gen ... ; go test ./...`
