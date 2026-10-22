# KubeTask 项目状态

## 2026-10-22 原版容器折射与导航文字修正

- 重新核对固定提交的上游实现，撤掉中间尝试的自定义边缘 shader 与 Tab 按下态容器。恢复 `sc-card-0` 原版折射、默认高光/外阴影及无内阴影配置；维持 0.22 主体透明度、静态缓存与饼图圆角。纯色采样背景缺乏可见形变，当前仍不捕获或折射任意底层 HTML。
- 导航此前清空上游文字并叠加 DOM 文字，导致文字不参与折射。现将标签保留在上游前景纹理与指示器 CombinedBackdrop 内，透明输出补充文字绘制；语义链接继续提供键盘操作、无障碍名称与失效回退。几何重建时补齐有界的初始化预热帧。
- 9 项针对性材质/输出/导航检查、生产构建通过；只读预览确认容器边缘、首次加载文字及导航拖动/释放，检查中无捕获警告或错误。按用户要求未重复完整前后端流水线。18080 仍为隔离只读展示数据，未操作 Kubernetes。详情见 [集成记录](../web/LIQUID_GLASS.md#original-container-optics-and-navigation-text-2026-10-22)。

## 2026-10-22 重启后视觉复验及容器透明度

- 重启后浏览器检查工具恢复。已确认图表、图例开关、键盘数据提示、按钮按压/释放和拖动后的恢复；认证弹层正常尺寸不裁剪阴影，320×340 下内容可滚动（滚动量 42px），320px 页面无横向溢出。此前工具失败的记录保留为历史快照。
- 按用户要求增强全站容器透明度：上游卡片表面原本为透明，但适配器输出了不透明的采样背景；现仅把容器主体的预乘 RGB/alpha 缩放至 0.22，保留文字及原版高光/阴影。弹层用更强的半透明底色和局部背景模糊维持可读性，未修改上游源码、按钮或导航输出。可透出真实 DOM，但不宣称对底层 HTML 做 WebGL 折射。
- 按“无需过多检测”的要求，仅运行两个透明输出针对性检查、生产构建和一页视觉确认，均通过；不把前一快照的 37 项完整回归写作此次全部复跑。18080 为保留用于审阅的隔离只读预览，拒绝非 GET，不代表真实集群数据。实现说明见 [容器透明度记录](../web/LIQUID_GLASS.md#container-translucency-2026-10-22)。

## 2026-10-22 前端产物瘦身与按钮阴影修正

本次对应 HEAD `8768a98` 加当前未提交工作区；保留并行出现的 Go 认证、日志、lint 工具及其他工作区修改。本次只改前端和相关文档。

- 主页趋势和类型占比改用原生 SVG，保留数据提示、图例开关、键盘查看与响应式布局。卸载 ECharts、zrender 及两个嵌套 tslib 包，无新增依赖。构建模块数 729→143，Dashboard 分块 537.08kB→**8.614kB**（gzip 3.816kB），已无 >500kB 分块提示。最终全部 JS/CSS 产物合计 **546,319 字节**（逐文件 gzip 测量 **178,433 字节**）；相对上一轮构建表约 1,072.78kB，原始资源约减少 **49%**。该比较不含 HTML、public 和许可证，不等于实测加载时间或 FPS。
- 确认认证表单 `overflow: clip` 和弹层始终滚动裁剪了按钮阴影，改为按实际正常流内容高度判断滚动，装饰画布不参与高度判断。捕获的触摸/笔拖动超出按钮时限制输入坐标，避免上游形变超出预留绘制区。回归测试覆盖远距离拖动、非中心按压、分数宽度和回弹；未改上游阴影/弹簧参数或源码。
- 按钮复用模板、跳过无变化布局，并清理热更新监听器、渲染器及卸载画布。日志复用不可变记录与未变化行，查询复用正则，修复 `<`、`&` 等原文字符高亮；不截断日志或改变 SSE。任务列表重试前清除旧连接错误，恢复连接后允许显示返回列表。
- 前端测试 **37/37**、生产构建、`npm ls --depth=0` 和 `git diff --check` 通过。当前工作区完整 Windows Go 流水线（build、vet、controller-gen、生成后 build、绝对 envtest 路径的默认全部测试）通过，生成物无内容差异；该 Go 结果包含既有后端修改，并非本次更改这些实现。
- 浏览器和 Windows UI 检查入口均因系统路径缺失而初始化失败。本次没有完成按压视觉、弹层滚动及真实集群复验，不将几何测试写作截图验收。剩余主要体积是按需加载的原版 WebGL 渲染器（268.72kB）；保留其实现以满足原组件要求。当前日志仍保留完整 DOM 行，极长日志的窗口化渲染可作为后续优化，但本次不丢弃记录。
- 细节、测量口径与限制见 [集成记录](../web/LIQUID_GLASS.md#bundle-reduction-and-clipped-button-shadows-2026-10-22)；当前技术栈已同步至前端、中英文 README、AGENTS 和计划书，历史验收段落保持原日期与数值。

## 2026-10-22 CI linter 与 Go 1.27 兼容修正

- 用户提供的 GitHub Actions 日志显示：Go 1.27.3 下执行 `make lint`，v2.11.4 的类型检查器读取标准库导出数据失败（`export data version 5 is greater than maximum supported version 2`）。该错误发生在标准库导入阶段，无法通过 gofmt 修复。
- `Makefile` 与 `.custom-gcl.yml` 的 golangci-lint 版本统一从 v2.11.4 升至 v2.15.0，保留 logcheck 插件及现有检查规则。官方从 v2.13.0 起支持 Go 1.27，见 [发布记录](https://github.com/golangci/golangci-lint/blob/main/CHANGELOG.md)。本次未修改业务 Go 源码，保留工作区既有格式和前端修改。
- 当前 Windows 工作区完整 Go 验证流水线通过：`go build ./...`、`go vet ./...`、controller-gen、生成后编译，以及使用绝对 envtest 资源路径的 `go test ./... -count=1`；生成物无内容差异。结果对应本地 HEAD `8768a98` 加当前未提交工作区，不代表远端 CI 已通过。
- 使用 Go 1.27.3 构建 v2.15.0 及含 logcheck 的自定义二进制成功，`config verify` 通过。Windows 下 `run --timeout 5m` 已无标准库 typecheck 错误，但仍未通过 lint：50 项 goconst、12 项 modernize、3 项 Windows 专属 errcheck，共 65 项；本轮只修复工具兼容问题，不批量改写业务及测试源码，也不关闭规则掩盖结果。
- 随后在同一 Windows 主机设置 `GOOS=linux` 运行该自定义 linter，完成并报告 **62 项**（goconst 50、modernize 12），无原 typecheck 错误；该检查选择 Linux 构建文件，不能替代 Ubuntu GitHub Actions 实跑。完整 lint 仍未通过，推送格式修改或仅更新工具版本均不足以让该检查变绿。

最新源码整理记录见 [2026-10-22 前端文件精简](#2026-10-22-前端文件精简)。

## 2026-10-22 前端文件精简

对应本地 HEAD `8768a98` 加当前工作区；本次只整理前端文件与文档，保留既有 Go 认证和日志相关修改。

- 从业务源码及测试出发解析导入关系，删除 25 个未被依赖的上游演示/辅助模块；vendor 从 105 个文件降至 80 个。正在使用的渲染器、导航、按钮、卡片及传递依赖保留，保留源码无内容修改，固定提交和许可证副本保留。裁剪范围见 [来源记录](../web/src/vendor/liquid-glass-webgl/UPSTREAM.md)。
- 删除两个未使用的 Vue/Vite 模板 SVG；弹层、选择器、玻璃容器样式按原顺序合并为 `controls.css`，样式文件从 7 个减至 5 个。逐规则比较确认仅空白和新增分段注释有差异；业务组件及独立测试不合并。
- 净减少 **29 个文件**，删除的未使用源码和图标合计 261,510 字节（约 255KiB，不计文档变化）。未使用模块原本已被构建排除，因此不将目录精简视为运行性能或产物体积提升；未添加依赖。
- `npm test` **31/31**、`npm run build` 通过；Dashboard 537.08kB 的既有分块提示保留。`git diff --check` 通过，package.json/lockfile 无变化。本次未进行新的浏览器或真实集群验收，CSS 规则比较、测试和构建仅证明本次整理的静态及回归边界。
- 当前工作区的完整 Windows Go 流水线也通过：Go 1.27.3 编译、vet、controller-gen、生成后编译，以及设置绝对 envtest 路径的默认全部测试；生成物无内容差异。该结果包含工作区既有后端修改，未由本次前端整理改动这些实现。
- 维护说明已同步至 [前端 README](../web/README.md#源码组织) 与 [集成记录](../web/LIQUID_GLASS.md#source-cleanup-2026-10-22)。

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
