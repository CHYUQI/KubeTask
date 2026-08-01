# Stage 1: 编译 Go 二进制
FROM golang:1.25-alpine AS builder

# 国内 Go 代理
ENV GOPROXY=https://goproxy.cn,direct

# 使用国内 Alpine 镜像源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git ca-certificates

WORKDIR /workspace

# 先复制依赖文件，利用 Docker 缓存层
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o manager cmd/main.go

# Stage 2: 最小运行时
FROM alpine:3.21

# 国内 Alpine 镜像源 + 运行时依赖
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata

COPY --from=builder /workspace/manager /manager

EXPOSE 8080 8081
USER 65534
ENTRYPOINT ["/manager"]
