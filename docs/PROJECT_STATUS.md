# KubeTask 项目状态

核对日期：2026-10-20（Asia/Shanghai）。基于本地 `main`、HEAD `468da87` 和当前未提交的工作区；不代表远端发布状态。

## 实现进度

| 范围 | 当前状态 | 代码依据 |
| --- | --- | --- |
| Task 核心 | 已实现 Cron / OneTime / Delay、Job 状态同步、重试参数、deadline、TTL、Cron 并发、暂停/恢复、trigger、历史与清理 | `api/v1/task_types.go`、`internal/controller/task_controller.go` |
| P2.0 同端口交付 | Gin 托管 Vue 静态文件与 API；Dockerfile 内构建前端 | `internal/api/webui.go`、`Dockerfile` |
| P2.1 Workflow | 扁平 DAG、模板/内联规格、依赖/失败分支、maxParallel、子 Task、状态与删除清理 | `api/v1/workflow_types.go`、`internal/workflow/dag.go`、`internal/controller/workflow_controller.go` |
| Workflow API / UI | 创建/列表/详情/删除；DAG、筛选、刷新、子 Task/日志跳转；无编辑或重跑 API | `internal/api/router.go`、`internal/api/handler/workflow_*`、`web/src/pages/Workflow*` |
| P2.2 身份认证 | 可选 ServiceAccount TokenReview；显式认证 Flag / 环境变量 / YAML；默认关闭 | `internal/auth/`、`internal/api/middleware/auth.go`、`internal/config/config.go`、`cmd/main.go` |
| P2.2 前端接入 | 内存 Token 设置/清除；普通 API 与 fetch SSE 使用 Authorization；旧请求取消与错误提示 | `web/src/api/{auth,http,index,logs}.js`、`AuthenticationPanel.vue` |
| P2.2 未完成 | 调用者 RBAC 资源授权、Namespace 隔离、API Key、操作审计 | 当前只挂载认证中间件，无资源授权层 |
| P2.3 / P2.4 | 通知渠道、资源感知调度待实现；Job deadline 与 Cron 并发策略已有 | Controller 与计划书对照 |
| P3 | 多集群、错峰预测、业务指标/Grafana、Serverless 为规划 | 当前无对应执行模块；clusterName / database 配置仅预留 |

## 本次检查结果

环境：Windows amd64，Go 1.25.7、controller-gen v0.21.0、Node 24.17.0、npm 11.13.0；envtest Kubernetes 1.35.0。前端依赖以 `web/package-lock.json` 为准，本次 Vite 构建输出版本为 8.1.4。

| 检查 | 结果 |
| --- | --- |
| `go build ./...` | 通过 |
| `go vet ./...` | 通过 |
| controller-gen object / RBAC / CRD（AGENTS.md Windows module 路径） | 通过，生成物无内容差异 |
| 生成后 `go build ./...` | 通过 |
| 设置绝对 `KUBEBUILDER_ASSETS` 后 `go test ./... -count=1` | 全部默认套件通过，包含真实本地 TokenRequest/TokenReview、Controller / Handler envtest 与日志连接测试 |
| `npm test`（web/） | 16/16 通过：10 个认证/SSE 测试，6 个 DAG 布局测试 |
| `npm run build`（web/） | 最终复验通过；Dashboard chunk 约 540 kB，保留 >500 kB 提示 |
| `helm lint ./charts/kubetask` | 通过，仅建议配置 Chart icon |
| Chart CRD 与 config/crd/bases 对照 | 两个 YAML 文件 SHA-256 一致 |
| Markdown 本地链接、业务路由与文档清单、`git diff --check` | 通过 |

首次前端构建时，`style.css` 引用的 `styles/pages.css` 和 `styles/responsive.css` 尚未齐备；检查期间文件补齐后构建通过。本次仅更新文档，保留工作区中的认证和界面源码修改。

默认 Go 测试不包含 `//go:build e2e` 的 Kind 套件；未开启 `KUBETASK_TEST_TOKEN_EXPIRY=true` 的自然过期慢测。本次没有运行真实集群、浏览器、Docker 镜像构建或 Helm 安装验收。已有真实 Pod / 日志证据见 [2026-10-13 minikube 验收](P2.1_VERIFICATION.md)，前端认证的历史浏览器验收见 [P2.2 记录](P2.2_FRONTEND_AUTH_VERIFICATION.md)。

## 已确认的配置与运行边界

- `router.go` 注册 15 个 `/api/v1` 业务路由和公开 `/healthz`。Manager 探针默认 `:8081`，Metrics 默认 TLS `:8443`，不在业务 `/api/v1` 下。
- Task / Workflow 是 Cluster-scoped，Job 固定在 `default`；ServiceAccount namespace 不提供租户隔离。
- TokenReview 只确认身份；业务操作仍使用后端 Kubernetes 身份。UI 的 403 处理不代表资源授权已经实现。
- 开启认证需要后端身份有 TokenReview create 权限。当前 Helm values / ConfigMap 未提供认证配置项，Chart controller role 和生成的 `config/rbac/role.yaml` 未包含该权限；需显式配置并补独立授权，步骤见 [API 文档](API.md#11-认证配置与路由接入)。不补权限时，携带格式正确 Token 的请求进入审查后返回 503。该边界由部署模板、认证代码及权限不足的 envtest 用例支持，未在 Helm 部署中复验。
- 普通配置为环境变量 > YAML > 默认值，只有显式认证 Flag 额外覆盖环境变量。Metrics/Leader/Probe 的三个旧 Flag 仅注册，未绑定配置加载器。
- 未开启认证且无法加载 Kubernetes 配置时为 Standalone，仅 `/healthz` 为 200，其余请求为 503，不挂载 Web UI 或 Manager 探针。配置可加载但集群不可达时，不保证降级。
- Job Controller 负责 Pod 重试；KubeTask 根据 Job 终态记执行结果，不实现独立指数退避。历史最多 10 条，TTL 清理 Job/Pod 后日志接口不能恢复其内容。
- Workflow 子 Task 进入 Task 列表/统计/趋势是预期行为；spec、suspend/resume、trigger 的既有取舍按 [AGENTS.md](../AGENTS.md) 保留。

## 文档同步

已核对并更新全部 8 份既有 Markdown 文档：中英文 README、PROJECT_PLAN、AGENTS、web/README、API、P2.1 与 P2.2 验收记录，并新增本文。

本轮修正文档中的旧目录、未完成勾选、配置优先级、探针端口、重试归属、认证进度与部署前提；移除重复的过时 Go 数据模型，保留历史验收原日期与证据。未改 CRD 语义、后端接口或 Controller 行为。

## 2026-10-20 前端排版复验

以下为后续 UI 调整后的工作区快照，补充上述文档核对阶段的记录：

- 依据 Apple HIG 的层级、对齐、分组和渐进展示原则，统一页面标题与操作、创建表单、工作流摘要、列表和日志工具栏；保留指定仓库的 WebGL 导航与按钮组件、纯色界面及无宣传标语要求。
- Workflow 创建入口已接入现有创建 API；前端测试为 **18/18**，包括新增的 Workflow 创建请求与失败处理测试。`npm run build` 通过，Dashboard 分块约 537 kB 的体积提示仍保留。
- 顺序运行 `go build ./...`、`go vet ./...`、controller-gen、生成后编译及设置绝对 envtest 路径的 `go test ./... -count=1`，全部通过；生成物无内容差异。
- 浏览器检查使用临时本地测试数据，检查 1280、390、320 像素宽度、创建页折叠选项、详情“更多”菜单与 Escape、列表、详情及日志工具栏；320 像素任务列表和详情页无页面横向溢出。未执行真实 Kubernetes 集群创建验收，临时数据服务不属于交付内容。
- 完整前端备份与来源说明见 [Liquid Glass 集成记录](../web/LIQUID_GLASS.md)。本次未改变后端语义，保留工作区既有认证、SSE 和其他文档修改。
