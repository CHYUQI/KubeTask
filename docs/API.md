# KubeTask API 文档

本文档描述 KubeTask 当前实现的 HTTP API。接口以 `internal/api/router.go` 和
`internal/api/handler/` 中的代码为准，规划中的 Workflow API 不包含在本文档内。

## 1. 基本信息

- 默认 API 地址：`http://localhost:8080`
- API 前缀：`/api/v1`
- 数据格式：JSON
- 当前版本未实现认证与授权，部署时应通过 Ingress、Service 或网关限制访问范围。
- Task 是 Kubernetes Cluster-scoped 资源，不需要在 URL 中携带 namespace。
- Controller 创建的 Job 当前固定在 `default` namespace 中。

除日志流接口外，成功和失败响应均为 JSON。常见错误响应格式如下：

```json
{
  "error": "task not found"
}
```

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
| `concurrencyPolicy` | string | 否 | `Allow`、`Forbid` 或 `Replace`，默认 `Allow` |
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
| `failed` | int32 | 累计失败次数 |
| `succeeded` | int32 | 累计成功次数 |
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
`lastScheduleTime` 和 `activeJobs`，由 Controller 异步创建下一次执行。

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

当程序未连接到 Kubernetes 集群时，会进入 Standalone 模式。此时 API 端口仍
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

