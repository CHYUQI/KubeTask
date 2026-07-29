# Stage 1: 编译 Go 二进制
FROM golang:1.25-alpine AS builder

WORKDIR /workspace

# 先复制依赖文件，利用 Docker 缓存层（代码不变不重新下载）
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o manager cmd/main.go

# Stage 2: 最小运行时
FROM alpine:3.21

# 安装 ca-certificates（K8s API 需要 HTTPS）+ tzdata（时区）
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /workspace/manager .

# 可选配置文件目录
RUN mkdir -p /app/config

EXPOSE 8080 8081

ENTRYPOINT ["/app/manager"]
