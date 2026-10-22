# KubeTask Web UI

最后核对：2026-10-21。Vue 3 + Vite + Vue Router + ECharts + Dagre；生产静态资源由 Gin 与 REST API 在 8080 同端口提供。当前检查结果见 [项目状态](../docs/PROJECT_STATUS.md)。

## 开发与构建

使用 Node.js 22.12+ 和 npm（本次检查：Node 24.17.0 / npm 11.13.0；Dockerfile 使用 Node 22）。在本目录执行：

```powershell
npm ci
npm run dev
```

开发地址为 `http://localhost:5173`，`vite.config.js` 将 `/api` 代理到 `http://localhost:8080`。需另行从项目根目录启动连接 Kubernetes 的后端，并先安装 `config/crd/bases/` 中的 CRD。

```powershell
# 在 web/ 中执行
npm test
npm run build
npm run preview
```

`preview` 只预览静态产物，当前配置未为它设置 API 代理；完整联调使用 Vite 开发服务，或回到项目根目录通过后端托管：

```powershell
$env:KUBETASK_WEB_DIR = 'web/dist'
go run ./cmd/main.go
```

Dockerfile 自动执行 `npm ci` / `npm run build` 并把 `dist/` 复制到镜像 `/web/dist`，生产无需单独运行 Vite。

## 页面与边界

- `/#/`：任务统计、状态分布、执行趋势。
- `/#/tasks`、`/#/tasks/create`、`/#/tasks/:name`：任务列表、创建、详情与触发/暂停/恢复/删除；当前没有独立编辑页，更新 spec 使用 API。
- `/#/tasks/:name/logs`：SSE 日志、跟踪、过滤、自动滚动、行号与级别高亮。
- `/#/workflows`、`/#/workflows/create`、`/#/workflows/:name`：工作流列表、详情、DAG、节点状态、缩放、子 Task/日志跳转和自动刷新；创建页复用现有 API，删除继续使用 API 或 kubectl。

Workflow 子 Task 出现在任务列表和统计中是预期行为。API 与领域语义以 [API 文档](../docs/API.md) 和后端代码为准。

## 认证与请求

`src/api/index.js` 为普通业务请求入口，`auth.js` 管理页面内存中的 Token 与请求取消，`http.js` 处理通用错误，`logs.js` 使用 fetch 流式读取 SSE，以支持 Authorization 请求头。

页面上方可设置、更换或清除原始 ServiceAccount Token。Token 不写入 URL、localStorage 或 sessionStorage，刷新后需重新输入；未设置时匿名访问，兼容后端默认关闭认证的状态。更换/清除会取消旧请求并重新加载当前视图，不自动重试写操作。

401 展开 Token 输入提示；403 提示权限不足，503 提示服务不可用，保留凭据。前端支持 403 不代表后端已实现 RBAC 资源授权。普通请求超时 10 秒，SSE 只限制建立连接的时间，正常 EOF 保留日志，离开页面/停止会取消流。

## 验证

`npm test` 使用 Node 原生测试，覆盖认证与请求取消、SSE 解码、DAG 布局、Workflow 创建请求、玻璃材质及表面缓存；本次 **31/31** 通过。生产构建通过，Dashboard 约 537kB 的分块提示仍保留。构建与后端回归的最终结果统一记录在 [项目状态](../docs/PROJECT_STATUS.md)。

历史浏览器与集群验收见 [P2.1](../docs/P2.1_VERIFICATION.md) 和 [P2.2 前端认证](../docs/P2.2_FRONTEND_AUTH_VERIFICATION.md)。Node 单测与静态构建不代替浏览器或真实集群验收。

界面组件来源与 UX 调整见 [Liquid Glass 集成说明](LIQUID_GLASS.md)。

## 界面材质与性能

### 引用仓库与许可证

- 上游仓库：[martin65536/liquid-glass-webgl](https://github.com/martin65536/liquid-glass-webgl)，参考演示：[Liquid Glass WebGL](https://glass.mt512.qzz.io/)。
- 固定源码版本：[`0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80`](https://github.com/martin65536/liquid-glass-webgl/tree/0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80)。来源与许可保存在 [vendor 目录](src/vendor/liquid-glass-webgl/)，上游许可证为 **AGPL-3.0**，分发副本见 [LICENSE](public/third-party/liquid-glass-webgl/LICENSE)。
- 复用范围：上游 WebGL 渲染器、导航、按钮与 Scroll Container 卡片材质。Vue 语义 DOM、业务页面、弹层定位及选择交互由本项目适配；并非直接运行上游 React 组件。

导航、按钮及内容卡片复用固定版本的 `liquid-glass-webgl` 源码。表单、表格外框、详情、DAG 节点、日志、状态提示与弹出菜单统一通过 `GlassContainer` 显示上游卡片材质，文字和交互保持语义 DOM。下拉选择使用轻量 Vue 组件，支持方向键、Home/End、输入查找、Enter/Space、Escape 与 Tab；未引入 Reka UI、React 或其他 UI 框架，也未添加渐变和宣传标语。

静态材质按尺寸复用，离屏延迟绘制，缓存最多约 400 万像素（仅缓存 RGBA 位图约 16MB，不代表应用总内存）。大容器降低材质分辨率，文字与图表保持各自分辨率；空闲时收缩绘制缓冲区。按钮仅复制变化的画面，日志滚动按帧合并。未测量 FPS，不把这些措施等同于具体帧率收益。

透明输出以纯色页面背景为采样来源，不会抓取并折射任意底层 HTML。上游版本、许可、实现细节及浏览器验证见 [当前集成记录](LIQUID_GLASS.md#shared-glass-surfaces-and-performance-2026-10-21)。
