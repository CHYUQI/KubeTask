# KubeTask API 文档

本文档描述 KubeTask 当前实现的 HTTP API。接口以 `internal/api/router.go` 和
`internal/api/handler/` 中的代码为准。Workflow（DAG 工作流）接口见第 9 节。

最后核对：2026-10-20。共有 15 个 `/api/v1` 业务路由和公开 `/healthz`；检查结果见 [项目状态](PROJECT_STATUS.md)。

## 1. 基本信息

- 默认 API 地址：`http://localhost:8080`
- API 前缀：`/api/v1`
- 数据格式：JSON
- HTTP 认证默认关闭，可通过 `auth-enabled` 开启 ServiceAccount Bearer 认证；资源授权与租户隔离尚未实现。
- Task 是 Kubernetes Cluster-scoped 资源，不需要在 URL 中携带 namespace。
- Controller 创建的 Job 当前固定在 `default` namespace 中。

除日志流接口外，成功和失败响应均为 JSON。常见错误响应格式如下：

```json
{
  "error": "task not found"
}
```

### 1.1 认证配置与路由接入

默认 `auth-enabled: false`，业务 API 允许匿名访问。开启认证时必须指定非空
`auth-audience`，例如：

```yaml
auth-enabled: true
auth-audience: kubetask
```

认证项支持以下配置来源，优先级为**显式命令行参数 > 环境变量 > YAML > 默认值**：

| 配置项 | 命令行参数 | 环境变量 | 默认值 |
| --- | --- | --- | --- |
| `auth-enabled` | `--auth-enabled=true` / `--auth-enabled=false` | `KUBETASK_AUTH_ENABLED` | `false` |
| `auth-audience` | `--auth-audience=kubetask` | `KUBETASK_AUTH_AUDIENCE` | 空字符串 |

只注册而未显式传入的 flag 默认值不会覆盖环境变量或 YAML；显式的
`--auth-enabled=false` 可以覆盖其他来源的开启设置。audience 会去除首尾空白。
开启时 audience 为空、开关值无法解析、或无法加载 Kubernetes 配置，启动会失败。

启动入口使用已有 Kubernetes 客户端的 `AuthenticationV1().TokenReviews()`
构造校验器，按开关将认证中间件传入路由。构造时不调用 TokenReview；运行时
无法完成 TokenReview（包括权限不足）返回下述通用 503，不会转为匿名处理。

部署时，**后端进程使用的 Kubernetes 身份**必须有
`authentication.k8s.io/tokenreviews` 的 `create` 权限；这与调用者是否能读写
Task 无关。当前 Helm values / ConfigMap 没有认证配置项，Helm controller role
与 `config/rbac/role.yaml` 也未包含该权限，单设 `--set auth.enabled=true`
不会开启认证。需要在 Deployment 中显式配置环境变量/启动参数或 YAML，
并由集群管理员为后端身份单独绑定审查权限，例如：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubetask-token-reviewer
rules:
  - apiGroups: [authentication.k8s.io]
    resources: [tokenreviews]
    verbs: [create]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubetask-token-reviewer
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubetask-token-reviewer
subjects:
  - kind: ServiceAccount
    name: kubetask                 # 替换为 Deployment 的 serviceAccountName
    namespace: default             # 替换为部署 namespace
```

为客户端 ServiceAccount 申请 Token 时，应使用与 `auth-audience` 相同的 audience
（例如 `kubectl create token <account> -n <namespace> --audience=kubetask`），
再通过请求头或 UI 输入。以上部署步骤本轮未在真实集群执行。

`NewRouter` 可接收可选的 Gin 中间件，并在注册端点前按传入顺序统一挂到
`/api/v1` 路由组。调用方可显式传入已有的认证中间件：

```go
router := api.NewRouter(k8sClient, clientset, addr,
    middleware.Authentication(authenticator),
)
```

挂载认证后，所有已注册的 Task、Workflow、统计、趋势和 SSE 日志端点均要求
`Authorization: Bearer <token>`，不接受 URL 参数传 Token。缺失或格式错误的
请求头、未通过校验的凭据返回 `401 {"error":"Unauthorized"}`，并携带
`WWW-Authenticate: Bearer`；认证服务不可用、非预期校验错误、认证超时或取消
返回 `503 {"error":"Service Unavailable"}`。认证响应不暴露 Token 或底层错误。

`/healthz` 与网页静态资源不经过该认证中间件；未注册的 API 路径仍返回 404。
认证最多使用 3 秒的子 context，调用结束立即释放，后续业务请求及 SSE
继续使用原请求 context。认证只确认身份，不限制资源访问范围；ServiceAccount
所属 namespace 不代表 Task 或 Workflow 的隔离边界。

当前只实现身份认证，尚未接入 SubjectAccessReview 授权、API Key 或操作审计。
前端在页面上方提供「设置 Token」入口，输入原始 ServiceAccount Token（不含 `Bearer` 前缀）。
普通 API 与 SSE 日志请求均通过 `Authorization` 请求头发送 Token，不放入 URL。
Token 仅保存在当前页面内存中，不写入 localStorage/sessionStorage，刷新或关闭页面后需重新输入。
未设置 Token 时保持匿名请求，兼容后端默认关闭认证的配置。
更换或清除 Token 会取消旧请求、关闭日志流并重新加载当前视图（未提交表单会重置）；写操作不会自动重试。
401 会展开 Token 输入提示；403 提示权限不足，503 提示服务不可用，均不会清除 Token。
日志页通过 fetch 流式读取原有 SSE 协议，以支持请求头；正常 EOF 保留日志，停止或离开页面会取消连接。
下文无 Token 的调用示例对应默认关闭认证的状态；开启后需要为 API 请求附加
`Authorization` 请求头，例如：

```bash
curl http://localhost:8080/api/v1/tasks \
  -H "Authorization: Bearer ${KUBETASK_TOKEN}"
```

`KUBETASK_TOKEN` 在这里仅为客户端调用示例使用的环境变量，不是服务端配置项。

真实 Token 的自动化联调位于 `internal/api/auth_envtest_test.go`：强制使用本地
envtest，创建临时 ServiceAccount 并通过 TokenRequest 签发 Token，再调用真实
TokenReview 和业务路由。覆盖匹配 audience 的 200、错误 audience/无效 Token/
缺少凭据的 401，以及审查客户端缺少 TokenReview 权限时的 503。
单独运行需要设置 `KUBEBUILDER_ASSETS`。测试还验证了当前业务 RBAC 边界：
SubjectAccessReview 和直接 Kubernetes 请求均拒绝调用者列出 Task，但认证后的
业务 HTTP 请求仍返回 200，因为业务 API 尚未接入调用者授权。

Token 自然过期是可选慢测：设置 `KUBETASK_TEST_TOKEN_EXPIRY=true`，并使用
`go test ./internal/api -run '^TestServiceAccountTokenAuthentication$' -count=1 -timeout=15m`。
测试申请 600 秒的真实 Token，先验证 200，再按服务器返回的过期时间等待其自然
变为 401，允许额外两分钟的校验容差；不修改 Token 或系统时间。常规流水线跳过
这项慢测，不能把普通套件通过描述为自然过期验证通过。

`internal/api/log_stream_test.go` 使用真实 HTTP 连接与可控的上游日志服务，验证
首条事件立即送达、超过认证的三秒期限后仍可接收事件、认证每个请求只执行一次，
以及客户端取消会关闭上游日志请求。该测试不启动真实 Pod，也未验证流内 Token
自然过期后的行为。这些测试不代表实际部署已开启认证或已实现租户隔离。

## 2. 接口总览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/healthz` | API 进程健康检查 |
| `POST` | `/api/v1/tasks` | 创建任务 |
| `GET` | `/api/v1/tasks` | 查询任务列表 |
| `GET` | `/api/v1/tasks/:name` | 查询任务详情 |
| `PUT` | `/api/v1/tasks/:name` | 更新任务 |
| `DELETE` | `/api/v1/tasks/:name` | 删除任务 |
| `POST` | `/api/v1/tasks/:name/trigger` | 手动触发任务 |
| `POST` | `/api/v1/tasks/:name/suspend` | 暂停任务调度 |
| `POST` | `/api/v1/tasks/:name/resume` | 恢复任务调度 |
| `GET` | `/api/v1/tasks/:name/logs` | 获取 Job Pod 日志，SSE |
| `GET` | `/api/v1/stats` | 查询任务统计 |
| `GET` | `/api/v1/stats/trend` | 查询执行趋势 |
| `POST` | `/api/v1/workflows` | 创建工作流 |
| `GET` | `/api/v1/workflows` | 查询工作流列表 |
| `GET` | `/api/v1/workflows/:name` | 查询工作流详情（含 DAG 图数据） |
| `DELETE` | `/api/v1/workflows/:name` | 删除工作流（异步清理子 Task） |

### 健康与就绪探针

Controller-runtime Manager 还会在 `health-probe-bind-address` 配置的端口
（默认 `:8081`）提供：

- `GET /healthz`：健康检查
- `GET /readyz`：就绪检查

这两个探针不属于 `/api/v1` 路由组。API 端口上的 `/healthz` 只返回 HTTP
服务自身的健康状态。

## 3. Task 数据模型

### 3.1 Task 请求/响应对象

```json
{
  "apiVersion": "kubetask.kubetask.io/v1",
  "kind": "Task",
  "metadata": {
    "name": "hello-kubetask",
    "labels": {
      "team": "platform"
    }
  },
  "spec": {
    "type": "OneTime",
    "image": "busybox:latest",
    "command": ["echo", "hello"],
    "args": [],
    "backoffLimit": 3,
    "activeDeadlineSeconds": 300,
    "ttlSecondsAfterFinished": 60,
    "concurrencyPolicy": "Allow",
    "suspend": false,
    "env": [
      {
        "name": "ENV",
        "value": "production"
      }
    ]
  }
}
```

### 3.2 `spec` 字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `type` | string | 是 | `OneTime`、`Cron` 或 `Delay` |
| `schedule` | string | Cron 时是 | 标准 5 字段 Cron：分、时、日、月、周 |
| `delay` | string | Delay 时建议 | 延迟时长，例如 `30s`、`5m`、`1h` |
| `image` | string | 是 | 执行容器镜像 |
| `command` | string[] | 否 | 覆盖镜像默认 ENTRYPOINT |
| `args` | string[] | 否 | 传给容器命令的参数 |
| `resources` | object | 否 | Kubernetes 容器资源 requests/limits |
| `env` | object[] | 否 | Kubernetes `EnvVar` 数组 |
| `backoffLimit` | int32 | 否 | 失败后的最大重试次数，默认 `3` |
| `activeDeadlineSeconds` | int64 | 否 | 单次执行最长时间，单位为秒 |
| `ttlSecondsAfterFinished` | int32 | 否 | Job 完成后保留时间，单位为秒 |
| `concurrencyPolicy` | string | 否 | Cron 调度采用 `Allow`、`Forbid` 或 `Replace`，默认 `Allow` |
| `suspend` | bool | 否 | `true` 时暂停调度，不删除已运行 Job |
| `clusterName` | string | 否 | 多集群场景预留，当前 Controller 未使用 |

`resources` 使用 Kubernetes 标准结构，例如：

```json
{
  "resources": {
    "requests": {
      "cpu": "100m",
      "memory": "128Mi"
    },
    "limits": {
      "cpu": "500m",
      "memory": "512Mi"
    }
  }
}
```

### 3.3 `status` 字段

`status` 主要由 Controller 异步维护，API 查询任务时会原样返回。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `phase` | string | `Pending`、`Running`、`Succeeded`、`Failed` 或 `Suspended` |
| `message` | string | 人类可读的状态信息 |
| `lastScheduleTime` | timestamp | 最近一次调度时间 |
| `lastStartTime` | timestamp | 最近一次执行开始时间 |
| `lastCompletionTime` | timestamp | 最近一次执行完成时间 |
| `executionHistory` | object[] | 最近最多 10 条执行记录 |
| `activeJobs` | string[] | 当前运行中的 Job 名称 |
| `failed` | int32 | 累计执行失败次数，按 Job 最终失败计数，不含 Pod 重试失败次数 |
| `succeeded` | int32 | 累计执行成功次数，按 Job 最终成功计数 |
| `conditions` | object[] | Kubernetes 风格的状态条件 |

执行记录结构：

```json
{
  "jobName": "hello-kubetask-1710000000",
  "startTime": "2026-08-25T10:00:00Z",
  "stopTime": "2026-08-25T10:00:03Z",
  "phase": "Succeeded",
  "exitCode": 0
}
```

## 4. Task 接口

### 4.1 创建任务

```http
POST /api/v1/tasks
Content-Type: application/json
```

请求体为 Task 对象。`apiVersion` 和 `kind` 为空时，服务端分别补为
`kubetask.kubetask.io/v1` 和 `Task`。`metadata.name` 为空时，服务端使用
`kubetask-` 作为 `generateName` 前缀，由 Kubernetes 自动生成名称。

创建时服务端会检查：

- `spec.type` 必须存在；
- `spec.image` 必须存在；
- `type=Cron` 时，`spec.schedule` 必须是合法的标准 5 字段 Cron 表达式。

> **名称长度不由 API 校验**（只校验 Kubernetes 名字本身的合法性）。超长名字会在 Controller
> 侧被判失败，见下面的说明。

#### Task 名称长度限制（Controller 兜底）

Task 名会被 TaskReconciler 拼成 Job 名（`<task 名>-<10 位 Unix 秒>`），Job 控制器再用它拼 Pod 名
（再追加 5 位随机后缀），而 Pod 名不能超过 63 字符。因此 **Task 名上限是 46 字符**：

- ≤ 46 字符：正常创建 Job 执行。
- \> 46 字符：`POST /api/v1/tasks` 会返回 `201`（名字本身是合法 Kubernetes 名称），
  但 Controller 不会创建 Job，而是把 Task 置为 `Failed`，`status.message` 形如：

  ```text
  task name is too long: 60 characters, limit is 46 (derived Job/Pod name would exceed 63 characters)
  ```

- Workflow 的子 Task 名（`<workflow>-<node>`）是拼出来的，因此在**创建 Workflow 时**就按同一条
  上限校验，超限直接返回 `400` + `reason=InvalidTask`（见 9.1 节）。

响应：`201 Created`

```json
{
  "apiVersion": "kubetask.kubetask.io/v1",
  "kind": "Task",
  "metadata": {
    "name": "hello-kubetask"
  },
  "spec": {
    "type": "OneTime",
    "image": "busybox",
    "command": ["echo", "hello"]
  }
}
```

可能的错误：

| 状态码 | 场景 |
| --- | --- |
| `400` | JSON 无法解析、缺少 `spec.type`/`spec.image` 或 Cron 表达式非法 |
| `409` | 同名 Task 已存在 |
| `500` | Kubernetes API 写入失败 |

示例：

```bash
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {"name": "hello-kubetask"},
    "spec": {
      "type": "OneTime",
      "image": "busybox:latest",
      "command": ["echo", "Hello from KubeTask!"]
    }
  }'
```

### 4.2 查询任务列表

```http
GET /api/v1/tasks
```

查询参数：

| 参数 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `page` | int | `1` | 页码，小于 1 时按 `1` 处理 |
| `pageSize` | int | `20` | 每页数量，调用方应传正整数 |
| `type` | string | 空 | 精确筛选：`OneTime`、`Cron`、`Delay` |
| `phase` | string | 空 | 精确筛选：`Pending`、`Running`、`Succeeded`、`Failed`、`Suspended` |

筛选和分页在服务端内存中完成。建议始终传入正整数 `pageSize`。

响应：`200 OK`

```json
{
  "items": [],
  "total": 0,
  "page": 1,
  "pageSize": 20
}
```

示例：

```bash
curl "http://localhost:8080/api/v1/tasks?page=1&pageSize=20&type=Cron&phase=Running"
```

### 4.3 查询任务详情

```http
GET /api/v1/tasks/:name
```

响应：`200 OK`，返回完整 Task 对象。

可能的错误：

| 状态码 | 场景 |
| --- | --- |
| `404` | Task 不存在 |
| `500` | Kubernetes API 查询失败 |

### 4.4 更新任务

```http
PUT /api/v1/tasks/:name
Content-Type: application/json
```

请求体至少应包含 `spec`。服务端根据路径中的 `:name` 查找目标 Task，然后：

- 用请求体中的 `spec` 整体替换原有 `spec`；
- 用请求体中的 `metadata.labels` 替换原有 labels；
- 用请求体中的 `metadata.annotations` 替换原有 annotations；
- 保留服务端已有的名称、UID、ResourceVersion、Finalizer 和 Status。

请求体中的 `metadata.name` 不用于选择目标资源，目标名称以 URL 为准。更新
接口只额外校验 `type=Cron` 时的 Cron 表达式；调用方应自行保证 `type`、
`image` 以及对应类型所需字段完整。

响应：`200 OK`，返回更新后的完整 Task 对象。

可能的错误：

| 状态码 | 场景 |
| --- | --- |
| `400` | JSON 无法解析或 Cron 表达式非法 |
| `404` | Task 不存在 |
| `500` | Kubernetes API 读写失败 |

示例：

```bash
curl -X PUT http://localhost:8080/api/v1/tasks/hello-kubetask \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {
      "labels": {"team": "platform"},
      "annotations": {"owner": "scheduler"}
    },
    "spec": {
      "type": "Delay",
      "delay": "5m",
      "image": "busybox:latest",
      "command": ["echo", "delayed"],
      "backoffLimit": 3
    }
  }'
```

### 4.5 删除任务

```http
DELETE /api/v1/tasks/:name
```

响应：`200 OK`

```json
{
  "message": "task deleted"
}
```

删除 Task 后，Controller 会通过 Finalizer 删除该 Task 关联的 Job。这个过程
是异步的，因此 HTTP 成功只表示删除请求已提交。

可能的错误：

| 状态码 | 场景 |
| --- | --- |
| `404` | Task 不存在 |
| `500` | Kubernetes API 删除失败 |

### 4.6 手动触发任务

```http
POST /api/v1/tasks/:name/trigger
```

不需要请求体。服务端写入 `kubetask.io/last-trigger` 注解，并清理
`lastScheduleTime` 和 `activeJobs`，由 Controller 异步按原任务类型处理。
OneTime / Delay 若仍有关联 Job，只同步已有 Job；旧 Job 已清理且 trigger
比上次执行新的情况下才会再次创建 Job。该接口不会删除旧 Job，也不绕过 suspend、
Delay 的目标时间或 Cron 的调度规则。Workflow 子 Task 同样可调用，属于现有语义；
重新运行整个工作流应创建新的 Workflow。

响应：`200 OK`

```json
{
  "message": "task triggered",
  "task": "hello-kubetask"
}
```

可能的错误：

| 状态码 | 场景 |
| --- | --- |
| `404` | Task 不存在 |
| `500` | Kubernetes API 更新失败 |

### 4.7 暂停任务

```http
POST /api/v1/tasks/:name/suspend
```

不需要请求体。服务端将 `spec.suspend` 设置为 `true`。Controller 随后将任务
状态更新为 `Suspended` 并跳过后续调度；已经运行的 Job 不会因该接口被删除。

响应：`200 OK`

```json
{
  "message": "task suspended",
  "task": "hello-kubetask"
}
```

### 4.8 恢复任务

```http
POST /api/v1/tasks/:name/resume
```

不需要请求体。服务端将 `spec.suspend` 设置为 `false`。Controller 会异步将
`Suspended` 状态恢复为 `Pending`，之后继续按任务类型调度。

响应：`200 OK`

```json
{
  "message": "task resumed",
  "task": "hello-kubetask"
}
```

暂停和恢复接口的错误状态：

| 状态码 | 场景 |
| --- | --- |
| `404` | Task 不存在 |
| `500` | Kubernetes API 更新失败 |

## 5. 日志接口

### 5.1 获取任务日志

```http
GET /api/v1/tasks/:name/logs
Accept: text/event-stream
```

查询参数：

| 参数 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `tail` | int64 | `100` | 返回的日志尾部行数 |
| `follow` | bool | `false` | 仅传字符串 `true` 时持续跟踪 |
| `sinceSeconds` | int64 | 空 | 仅返回最近指定秒数内的日志 |

服务端按照 `kubetask.io/task=<task name>` 标签查找 Job，再查找该 Job 的
Pod，并读取名为 `task` 的容器日志。当前实现使用 Job 列表返回结果中的最后
一个 Job；接口没有显式指定按时间排序。

响应头：

```http
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

每一行日志作为一个 SSE `data` 事件发送：

```text
data: Hello from KubeTask!

data: task completed

```

使用 `curl` 查看：

```bash
curl -N "http://localhost:8080/api/v1/tasks/hello-kubetask/logs?tail=100&follow=true"
```

可能的错误：

| 状态码 | 场景 |
| --- | --- |
| `404` | Task 不存在、没有关联 Job 或 Job 没有 Pod |
| `500` | Kubernetes API 查询失败或日志流打开失败 |

## 6. 统计接口

### 6.1 任务统计

```http
GET /api/v1/stats
```

响应：`200 OK`

```json
{
  "total": 3,
  "pending": 0,
  "running": 1,
  "succeeded": 1,
  "failed": 0,
  "suspended": 1,
  "onetime": 1,
  "cron": 1,
  "delay": 1
}
```

其中状态字段按 `status.phase` 统计，类型字段按 `spec.type` 统计。没有任务
时所有字段均为 `0`。

### 6.2 执行趋势

```http
GET /api/v1/stats/trend
```

接口遍历所有 Task 的 `status.executionHistory`，按照执行记录的
`startTime` 聚合到日期，日期格式为 `YYYY-MM-DD`。

响应：`200 OK`

```json
[
  {
    "date": "2026-08-25",
    "total": 4,
    "succeeded": 3,
    "failed": 1
  }
]
```

无执行记录时返回空数组 `[]`。当前实现使用内存 map 聚合，返回数组的日期
顺序未保证，客户端如需绘图应自行按 `date` 排序。

## 7. 调用流程示例

```bash
# 1. 创建任务
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {"name": "demo-task"},
    "spec": {
      "type": "OneTime",
      "image": "busybox:latest",
      "command": ["sh", "-c", "echo start; sleep 5; echo done"]
    }
  }'

# 2. 查询任务
curl http://localhost:8080/api/v1/tasks/demo-task

# 3. 读取最近日志
curl -N "http://localhost:8080/api/v1/tasks/demo-task/logs?tail=100"

# 4. 手动触发
curl -X POST http://localhost:8080/api/v1/tasks/demo-task/trigger

# 5. 暂停或恢复
curl -X POST http://localhost:8080/api/v1/tasks/demo-task/suspend
curl -X POST http://localhost:8080/api/v1/tasks/demo-task/resume

# 6. 查询统计和趋势
curl http://localhost:8080/api/v1/stats
curl http://localhost:8080/api/v1/stats/trend
```

## 8. Standalone 模式

当 `auth-enabled: false` 且程序无法加载 Kubernetes 配置时，会进入 Standalone 模式。此时 API 端口仍
会启动，`GET /healthz` 返回正常，但除健康检查外的未注册路由会返回：

```http
503 Service Unavailable
```

```json
{
  "error": "no kubernetes cluster available",
  "message": "请先连接 Kubernetes 集群。确保 ~/.kube/config 存在且集群可达。"
}
```

若 `auth-enabled: true` 且无法加载 Kubernetes 配置，程序直接退出，不进入
Standalone 模式。

## 9. Workflow 接口

Workflow 是 Cluster-scoped 的 DAG 编排资源：一个 Workflow CR 代表一次运行，
Controller 按 `dependsOn` 依次创建子 Task（命名为 `<workflow>-<node>`）。
数据模型以 `api/v1/workflow_types.go` 为准，运行语义与样例见
[PROJECT_PLAN.md](../PROJECT_PLAN.md) 的 P2.1 节。

### 9.1 创建工作流

```http
POST /api/v1/workflows
Content-Type: application/json
```

请求体是一个完整的 Workflow 对象（`metadata.name` 可省略，省略时按 `workflow-` 前缀自动生成）：

```json
{
  "apiVersion": "kubetask.kubetask.io/v1",
  "kind": "Workflow",
  "metadata": { "name": "ci-pipeline" },
  "spec": {
    "maxParallel": 2,
    "tasks": [
      { "name": "build", "template": "go-build" },
      { "name": "test", "dependsOn": ["build"], "template": "go-test" },
      { "name": "notify-failure", "dependsOn": ["test"], "runOnFailure": true, "template": "notify" }
    ],
    "taskTemplates": {
      "go-build": { "type": "OneTime", "image": "golang:1.27.3", "command": ["go", "build", "./..."] },
      "go-test": { "type": "OneTime", "image": "golang:1.27.3", "command": ["go", "test", "./..."] },
      "notify": { "type": "OneTime", "image": "curlimages/curl", "command": ["sh", "-c", "echo workflow failed"] }
    }
  }
}
```

成功返回 `201 Created` 和创建后的 Workflow 对象。错误响应：

| 状态码 | 场景 | 响应示例 |
| --- | --- | --- |
| `400` | 模板/依赖引用不存在、依赖成环、子 Task 名超长、CRD 结构或 CEL 校验失败 | `{"error":"task \"build\" references unknown template \"nope\"","reason":"MissingTemplate"}` |
| `409` | 同名 Workflow 已存在 | `{"error":"workflow already exists"}` |

> `reason` 取自 `internal/workflow` 的稳定错误码（如 `MissingTemplate`、`UnknownDependency`、
> `CyclicDependency`、`InvalidTask`），CEL 规则覆盖的字段（`taskSpec.type`、`schedule`、
> `delay`、`suspend`、`template`/`taskSpec` 互斥）由 API Server 直接拒绝。

内联 `taskSpec` 只允许 OneTime，不允许 schedule/delay 或 `suspend: true`；
`suspend: false` 可接受。命名模板中的调度字段不受相同 CEL 约束，Controller
生成子 Task 时强制 OneTime 并清空 schedule/delay/suspend。
子 Task 名称上限为 `internal/naming.MaxTaskNameLength`（46）；明确指定 Workflow
名称时在 HTTP 入口校验，自动生成名称时入口跳过名称相关检查，由 Controller 兜底。

### 9.2 查询工作流列表

```http
GET /api/v1/workflows?page=1&pageSize=20&phase=Running
```

支持的查询参数：`page`（默认 1）、`pageSize`（默认 20）、`phase`
（`Pending` / `Running` / `Succeeded` / `Failed`）。
`page < 1` 按 1，`pageSize < 1` 按 20 处理；空 phase 不筛选。

```json
{
  "items": [ { "metadata": { "name": "ci-pipeline" }, "spec": { }, "status": { "phase": "Running" } } ],
  "total": 1,
  "page": 1,
  "pageSize": 20
}
```

### 9.3 查询工作流详情（含 DAG 图数据）

```http
GET /api/v1/workflows/ci-pipeline
```

```json
{
  "workflow": { "metadata": { "name": "ci-pipeline" }, "status": { "phase": "Running" } },
  "graph": {
    "nodes": [
      { "name": "build", "dependsOn": [], "phase": "Succeeded", "taskName": "ci-pipeline-build" },
      { "name": "test", "dependsOn": ["build"], "phase": "Running", "taskName": "ci-pipeline-test" },
      { "name": "notify-failure", "dependsOn": ["test"], "runOnFailure": true, "phase": "Pending" }
    ],
    "edges": [
      { "from": "build", "to": "test" },
      { "from": "test", "to": "notify-failure" }
    ],
    "order": ["build", "test", "notify-failure"],
    "levels": [["build"], ["test"], ["notify-failure"]]
  }
}
```

- `nodes` / `edges` 来自 spec，并合并 `status.nodes` 的运行时状态（无状态时 `phase` 默认 `Pending`）。
- `order` 是拓扑序，`levels` 是并行层；当前前端使用 Dagre 布局与 Vue SVG 展示。
- 若 spec 被 `kubectl` 改成非法（例如引用了不存在的模板），接口仍返回 `200`，
  但 `graph.validationError` 会带上原因，前端可以据此提示用户。
- 工作流不存在时返回 `404`：`{"error":"workflow not found"}`。

### 9.4 删除工作流

```http
DELETE /api/v1/workflows/ci-pipeline
```

返回 `200 OK`：`{"message":"workflow deleting","workflow":"ci-pipeline"}`。
删除是异步的：Workflow 带 finalizer，Controller 会先删除全部子 Task，
清理完成后对象才真正消失；期间 `GET` 仍能查到（`metadata.deletionTimestamp` 非空）。
