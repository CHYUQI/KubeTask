# Stage 1: 构建 Web 静态资源
FROM node:22-alpine AS web-builder

WORKDIR /workspace/web

# 先复制依赖文件，利用 Docker 缓存层
COPY web/package*.json ./
RUN npm ci

# 复制前端源码并构建
COPY web/ ./
RUN npm run build

# Stage 2: 编译 Go 二进制
FROM golang:1.25-alpine AS builder

#proxy for go mod
ENV GOPROXY=https://goproxy.cn,direct

#git 
RUN apk add --no-cache git

#CA
RUN apk add --no-cache ca-certificates

WORKDIR /workspace

# 先复制依赖文件，利用 Docker 缓存层
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o manager cmd/main.go

# Stage 3: 最小运行时
FROM alpine:3.21

# 安装 ca-certificates（K8s API 需要 HTTPS）+ tzdata（时区）
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /workspace/manager /manager
# 将前端静态资源一并打包到镜像中
COPY --from=web-builder /workspace/web/dist /web/dist

EXPOSE 8080 8081
USER 65534
ENTRYPOINT ["/manager"]
