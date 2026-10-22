# KubeTask 项目状态

核对日期：2026-10-20（Asia/Shanghai）。基于本地 `main`、HEAD `468da87` 和当前未提交的工作区；不代表远端发布状态。

最新前端收尾记录见 [2026-10-21 全站玻璃容器与交付收尾](#2026-10-21-全站玻璃容器与交付收尾)。以下各日期记录对应当时快照，保留原验证边界。

当前 Go 基线为 **1.27.3**；升级与验证见 [2026-10-22 Go 版本升级](#2026-10-22-go-版本升级)。

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

## 2026-10-21 前端轻量性能优化

- 导航仅渲染可见栏及阴影，逻辑画布高度从 208 降至 112；同宽度、同 DPR 下画布像素面积减少约 46%。保留上游材质、透明效果和交互动画，未修改 vendored 源码。
- 导航与按钮在尺寸未变时跳过重复 resize；按钮仅在尺寸或色调变化时重建材质，Tab 切换仅使指示器分组缓存失效。软件渲染环境的材质 DPR 上限为 1.5，硬件渲染仍为 2，文字继续由 DOM 显示。
- `npm test` **21/21**、`npm run build` 通过；新增测试确认裁剪后材质和背板阴影仍在画布范围内。构建仍保留 Dashboard 分块超过 500 kB 的提示。
- 当前快照的 Windows Go 流水线（编译、vet、controller-gen、生成后编译、默认全部测试）通过；生成物无内容差异。
- 浏览器检查确认材质正常显示、拖动可切换至 Workflow、创建页可打开；未连接后端，未执行真实集群操作。本次未量化测量 FPS，不将画布面积降幅等同于帧率提升。

## 2026-10-21 玻璃按钮接入修正

- 全站 `GlassButton` 直接使用既有固定版本的上游按钮配置、文字光栅化、阴影和按压/拖动弹簧。移除按钮额外的 CSS 明暗滤镜、伪元素背板、透明度与阴影覆盖；按上游几何计算阴影留白。
- 多个按钮共享一个上游 WebGL 渲染器，跨页保留资源；只在原渲染器请求新帧时更新展示画布。透明输出适配保留原 RGB 绘制，避免整块不透明背景遮住弹层文字；新增测试防止阴影透明度被平方、加法高光出现黑边。
- 前端测试 **25/25** 与生产构建通过；Dashboard 约 537 kB 分块提示仍保留。浏览器检查了创建页按钮、跨页加载及认证弹层禁用按钮；未执行真实集群写操作。实现边界详见 [集成记录](../web/LIQUID_GLASS.md#button-rendering-correction-2026-10-21)。
- 本次 Windows Go 完整流水线再次通过，生成物无内容差异；浏览器 Enter / Space 激活、Escape 关闭及透明弹层绘制正常。

## 2026-10-21 原版 Tab 底板与菜单组件接入

- 按最新要求恢复原版 Tab 底板：保留上游 `tabs3` 分组和按压/拖动变换，撤掉固定背板覆盖和 CSS 背板；透明画布高度调为 160，容纳展开阴影。主页移除“创建任务”，标题操作仅保留刷新。
- 原仓库的菜单为 React/Radix；当前 Vue 项目接入 Reka UI 2.11.0，按仓库 dropdown-menu / popover / select 的结构和样式做 Vue 移植。Task“更多”、认证弹层和所有 BaseSelect 均使用库的 Portal、焦点、键盘、关闭及碰撞避让逻辑，空字符串“全部”筛选语义不变。不是直接运行原版 React 组件，未引入 React 运行时。
- `npm test` **25/25**、生产构建及 Windows Go 完整流水线通过；生成物无内容差异。浏览器验证了主页、Tab 拖动、选择菜单和 Cron 字段联动、筛选恢复、认证焦点和 Escape、“更多”的方向键/End/Escape。菜单详情和筛选用隔离的只读临时数据验证，未执行 Kubernetes 写操作；详见 [集成记录](../web/LIQUID_GLASS.md#original-tab-backing-and-popup-primitives-2026-10-21)。

## 2026-10-21 移除弹层组件框架

- 按最新要求卸载 Reka UI 及其带入的 18 个依赖包，源码、package.json 和 lockfile 不再引用该库；未引入替代框架。上一节保留为历史快照，当前实现以本节为准。
- BaseSelect 改用原生 HTML select，保留标签、禁用状态、描述提示、change 事件及空字符串“全部”筛选值；展开菜单与键盘操作由浏览器提供。“更多”和认证弹层共用轻量 Vue PopupPanel，保留方向键、Escape、外部关闭、焦点返回及视口边界定位。原有 WebGL 按钮和 Tab 未修改。
- 当前快照 `npm test` **25/25**、生产构建以及 Windows Go 完整流水线通过，生成物无内容差异；既有 Dashboard 大分块提示保留。浏览器检查覆盖原生选择键盘操作与 Cron 字段联动、“全部”筛选恢复、“更多”键盘及外部关闭、认证输入焦点与 Escape 返回，以及 320 像素弹层边界。菜单/筛选使用隔离只读数据，未执行 Kubernetes 写操作；详见 [集成记录](../web/LIQUID_GLASS.md#remove-the-popup-framework-2026-10-21)。

## 2026-10-21 看板玻璃容器与缓存整理

- 六张统计卡、两个图表卡及断连/错误提示改用固定版本上游 `buildScrollContainer` 的卡片材质，保留其 32px 圆角、折射、高光和阴影；Vue 仅适配布局尺寸和语义内容。静态卡片共用独立的按需 WebGL 渲染器，避免按钮动画重绘大卡片，离开看板后释放资源；未引入新包。
- `npm test` **27/27**、生产构建和 Windows Go 完整流水线通过，生成物无内容差异。浏览器检查了八张材质、两张图表、刷新、统计筛选跳转、跨页重新挂载、断连状态七张材质及 320 像素无横向溢出。图表使用隔离只读测试数据，未执行 Kubernetes 写操作；Dashboard 约 541kB 的既有分块提示保留。
- `npm prune` 确认无冗余依赖；`npm cache verify` 回收无效内容并压缩索引，实测 npm 缓存净减少 **1,127,235,443 字节（约 1.05GiB）**。三个 SSR 材质测试服务器关闭不需要的客户端预构建，复测未再产生 Vite 临时目录。
- 旧 Vite 缓存目录删除被工具自动审批拦截，仅返回策略阻止，未提供具体原因；约 **96.79MiB** 仍保留。源码、当前构建产物及原前端备份保留。详情见 [集成记录](../web/LIQUID_GLASS.md#dashboard-glass-containers-and-cache-cleanup-2026-10-21)。

## 2026-10-21 全站玻璃容器与交付收尾

本次对应本地 HEAD `ea815d6` 加当前未提交工作区；未创建提交或发布。按用户要求结束功能扩展，更新 [前端说明](../web/README.md) 与 [当前集成及性能记录](../web/LIQUID_GLASS.md#shared-glass-surfaces-and-performance-2026-10-21)。

- 创建表单、列表/详情卡片、状态提示、Workflow 图及节点、日志面板、下拉选择与弹出菜单接入固定版本上游卡片材质。紧凑控件限制圆角半径，保留语义 DOM、透明效果和原有按钮/导航动画。下拉选择由轻量 Vue combobox/listbox 实现，取代此前原生 select；保留空值筛选、禁用项、键盘选择与取消。未增加 UI 框架、渐变或宣传文案。
- 静态材质按尺寸缓存复用，离屏延迟绘制，限制大容器栅格像素，空闲收缩 GPU 缓冲区；按钮仅复制变化帧，日志滚动按帧合并。400 万像素缓存约 16MB 仅指缓存 RGBA 位图，不代表应用总内存；未测量 FPS。透明输出不抓取任意底层 HTML，完整边界见集成记录。
- 最终 `npm test` **31/31**、生产构建通过；Dashboard 分块 537.08kB 的既有提示保留。Windows 完整 Go 流水线（编译、vet、controller-gen、再次编译、绝对 envtest 路径的默认全部测试）通过。生成物与 package.json/lockfile 对 HEAD 无内容差异；`git diff --check` 通过。
- 浏览器验证覆盖创建页选择与字段联动、筛选恢复、更多菜单、认证焦点、列表/详情/图节点及日志。图缩放 100%→110% 时节点材质位图保持 405×216；500 条测试日志完整显示并自动滚动到底部。320px 下菜单与触发器同宽且无页面横向溢出，空闲后再次打开材质正常，无捕获到的控制台错误/警告。
- 验证使用生产构建和隔离只读临时数据服务（18080，拒绝非 GET，不代理 Kubernetes），未执行集群写操作或新的真实集群验收。临时服务已关闭，开发预览保留在 5173；原前端备份、历史验证日期与缓存清理限制保持原记录。本次未改变后端接口或领域语义。

### 筛选复核与来源声明补充

- 补充 `web/README.md` 的直接上游仓库/演示链接、固定提交、AGPL-3.0 许可副本及 Vue 适配范围。
- 重新使用实际响应 `type` / `phase` 查询的只读数据复核：任务选择 Running 发出 `phase=Running` 并从 1 条变为 0 条；恢复全部后重新显示 1 条；选择 Cron 发出 `type=Cron` 并变为 0 条。工作流选择 Failed 发出 `phase=Failed` 并变为 0 条。选择控件、请求参数及结果更新在该环境下正常，未据此修改业务筛选逻辑。
- 本机 5173 开发服务的 `/api` 代理目标为 `localhost:8080`；复核时 8080 无监听，5173 任务页显示“未连接 Kubernetes 集群 / 无法连接到后端服务”。因此当时无法在真实后端数据上复验筛选，不能将其作为真实集群筛选通过的证据。
- 先前用于材质展示的临时 fixture 返回固定列表，不处理筛选参数；它只能证明选择交互，不能证明列表过滤。上述新检查补充了这一验证缺口，临时服务不进入交付内容。

## 2026-10-22 Go 版本升级

本次对应本地 HEAD `ea815d6` 加当前未提交工作区；保留已有前端修改及各日期的历史验收记录。

- Go 基线从 1.25.7 升至 **1.27.3**，即 [官方发布记录](https://go.dev/doc/devel/release#go1.27.3) 中 2026-10-19 发布的 1.27 系列最新补丁版。`go.mod`、Docker builder、DevContainer、开发文档及工作流示例/测试中的 Go 镜像同步版本；三个 CI workflow 已使用 `go-version-file: go.mod`，自动跟随升级。
- Windows amd64 本地工具链已下载；保留 `GOTOOLCHAIN=auto`，项目目录直接执行 `go version` 返回 `go1.27.3 windows/amd64`。`go mod tidy -diff` 通过，无需调整其他模块版本或 `go.sum`。
- Windows Go 完整流水线通过：`go build ./...`、`go vet ./...`、controller-gen v0.21.0（Windows module 路径）、生成后再次编译，以及设置绝对 `KUBEBUILDER_ASSETS` 后的 `go test ./... -count=1`。默认全部套件通过，包含 Controller / Handler envtest 和真实本地 TokenRequest/TokenReview 测试；CRD / RBAC / DeepCopy 与 Chart CRD 无内容差异，`git diff --check` 通过。
- Docker Hub 官方 `golang:1.27.3` 与 `golang:1.27.3-alpine` 标签存在。本机无 Docker CLI，本次未执行镜像构建、DevContainer 启动或真实集群验收；默认 Go 测试仍不含 `e2e` build tag 套件和 Token 自然过期慢测。
