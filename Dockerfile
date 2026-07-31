# Stage 1: 编译 Go 二进制
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

# Stage 2: 最小运行时
FROM alpine:3.21

# 安装 ca-certificates（K8s API 需要 HTTPS）+ tzdata（时区）
RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /workspace/manager /manager

EXPOSE 8080 8081
USER 65534
ENTRYPOINT ["/manager"]
