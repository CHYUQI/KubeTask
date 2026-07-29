#!/bin/bash
# KubeTask k3s 一键部署脚本
# 用法: curl -fsSL https://xxx/deploy.sh | bash

set -e

echo "========================================="
echo " KubeTask v0.1.0 — k3s 部署"
echo "========================================="

# ─── 1. 安装 k3s ───
if ! command -v kubectl &>/dev/null; then
    echo "[1/5] 安装 k3s..."
    curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
    echo "等待 k3s 就绪..."
    sleep 10
else
    echo "[1/5] k3s/kubectl 已安装，跳过"
    export KUBECONFIG=${KUBECONFIG:-~/.kube/config}
fi

# ─── 2. 构建 Docker 镜像 ───
echo "[2/5] 构建 Docker 镜像..."
cd "$(dirname "$0")/../.."
docker build -t kubetask/controller:v0.1.0 .

# ─── 3. 导入镜像到 k3s ───
echo "[3/5] 导入镜像到 k3s..."
docker save kubetask/controller:v0.1.0 | sudo k3s ctr images import -

# ─── 4. 安装 Helm Chart ───
echo "[4/5] 部署 KubeTask..."
if ! command -v helm &>/dev/null; then
    curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi
helm upgrade --install kubetask ./charts/kubetask \
    --set image.repository=kubetask/controller \
    --set image.tag=v0.1.0 \
    --set image.pullPolicy=Never \
    --set config.logLevel=info \
    --set config.logFormat=console

# ─── 5. 等待就绪 ───
echo "[5/5] 等待 Pod 就绪..."
kubectl wait --for=condition=Ready pod \
    -l app.kubernetes.io/instance=kubetask \
    --timeout=60s

echo ""
echo "========================================="
echo " 部署完成！"
echo ""
echo " 访问 Web UI:"
echo "   kubectl port-forward svc/kubetask 8080:8080"
echo "   → 浏览器打开 http://localhost:8080"
echo ""
echo " 创建测试任务:"
echo "   kubectl apply -f - <<EOF"
echo "   apiVersion: kubetask.kubetask.io/v1"
echo "   kind: Task"
echo "   metadata:"
echo "     name: hello-kubetask"
echo "   spec:"
echo "     type: OneTime"
echo "     image: busybox"
echo "     command: [\"echo\", \"Hello from KubeTask!\"]"
echo "   EOF"
echo ""
echo " 查看日志:"
echo "   kubectl logs -l app.kubernetes.io/instance=kubetask"
echo "========================================="
