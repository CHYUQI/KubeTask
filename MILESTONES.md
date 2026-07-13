# KubeTask v0.1.0 — 里程碑状态

## 已完成 ✅

### P1.1  项目骨架
- `go build ./...` / `go vet ./...` 通过
- Kubebuilder v4 脚手架 (PROJECT, Makefile, Dockerfile, config/)
- `api/v1/task_types.go` — Task CRD (3 种类型 + 12 字段 + kubebuilder 校验标记)
- `internal/config/config.go` — Viper 三层配置 (Flag → YAML → EnvVar)
- `internal/logger/logger.go` — Zap (AtomicLevel 动态改日志级别)
- `zz_generated.deepcopy.go` / CRD YAML / RBAC 自动生成

### P1.2  核心 Controller
- `internal/controller/task_controller.go` — Reconciler
  - OneTime (立即创建 Job)
  - Cron (cron 表达式 + 并发策略 Allow/Forbid/Replace)
  - Delay (等 N 秒后创建)
  - Finalizer 机制 (删除 Task 时清理关联 Job)
  - Suspend/Resume
  - Job 状态同步 → Task Status (Phase + ExecutionHistory + 计数器)
  - OwnerReference (SetControllerReference) + Label 索引

### P1.3  REST API
- Gin HTTP Server 与 Controller **同进程运行** (`cmd/main.go`)
- 10 个端点:

| 方法 | 路径 | Handler |
|------|------|---------|
| POST | `/api/v1/tasks` | Create |
| GET | `/api/v1/tasks?page=1&type=Cron` | List (分页+筛选) |
| GET | `/api/v1/tasks/:name` | Get |
| PUT | `/api/v1/tasks/:name` | Update |
| DELETE | `/api/v1/tasks/:name` | Delete |
| POST | `/api/v1/tasks/:name/trigger` | Trigger |
| POST | `/api/v1/tasks/:name/suspend` | Suspend |
| POST | `/api/v1/tasks/:name/resume` | Resume |
| GET | `/api/v1/tasks/:name/logs?tail=100&follow=true` | SSE 流式日志 |
| GET | `/api/v1/stats` + `/api/v1/stats/trend` | 统计 |

### P1.7  单元测试 (31 tests)
- Controller 测试: **19 pass** (envtest: 真实 etcd + kube-apiserver)
- API Handler 测试: **12 pass** (httptest → Gin → K8s Client → API Server)

---

## 未完成 ❌

### P1.4  Web 管理界面
- [ ] Vue 3 + Vite 脚手架已初始化 (`web/`)，功能代码未写
- [ ] 仪表盘（状态卡片 + ECharts 图表）
- [ ] 任务列表（表格 + 分页 + 筛选 + 操作按钮）
- [ ] 任务创建/编辑表单
- [ ] 任务详情页
- [ ] 日志查看页（SSE 实时流）

### P1.5  日志前端面板
- [ ] 自动滚动、行号、级别高亮、搜索过滤

### P1.6  部署与运维
- [ ] Dockerfile（已有脚手架模板，未定制）
- [ ] Helm Chart (`charts/kubetask/`)
- [ ] k3s 一键部署脚本

### P1.7  集成测试
- [ ] E2E 测试（Kind 集群中完整链路）
- [ ] 集成测试（真实 k3s/Kind 环境）

---

## 项目文件树 (当前)

```
kubetask/
├── cmd/main.go
├── api/v1/
│   ├── task_types.go
│   ├── groupversion_info.go
│   └── zz_generated.deepcopy.go
├── internal/
│   ├── config/config.go
│   ├── logger/logger.go
│   ├── controller/
│   │   ├── task_controller.go
│   │   ├── task_controller_test.go (19 tests)
│   │   └── suite_test.go
│   └── api/
│       ├── router.go
│       └── handler/
│           ├── task_handler.go
│           ├── log_handler.go
│           └── handler_suite_test.go (12 tests)
├── config/
│   ├── crd/bases/kubetask.kubetask.io_tasks.yaml
│   ├── rbac/role.yaml
│   └── ...
├── web/                              ← Vue 脚手架 (功能待开发)
├── bin/k8s/                          ← envtest 二进制
├── AGENTS.md
├── PROJECT_PLAN.md
└── go.mod
```
