#!/usr/bin/env bash
set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Token Ray 一键部署脚本 ===${NC}"

if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}请使用 root 权限 (sudo) 运行此脚本。${NC}"
  exit 1
fi

if ! command -v go &> /dev/null; then
    echo -e "${RED}未检测到 Go 环境，请先安装 Go: https://golang.org/dl/${NC}"
    exit 1
fi

echo -e "${YELLOW}1. 编译二进制文件...${NC}"
go build -o dashboard ./cmd/dashboard
cp dashboard /usr/local/bin/token-ray
chmod +x /usr/local/bin/token-ray

echo -e "${YELLOW}2. 选择部署模式:${NC}"
echo "  1) 主控服务端 (Receiver) - 接收并展示面板"
echo "  2) 探针端 (Forwarder) - 仅收集当前机器数据并上报"
read -p "请输入模式编号 [1/2]: " MODE

if [ "$MODE" != "1" ] && [ "$MODE" != "2" ]; then
    echo -e "${RED}无效的输入。${NC}"
    exit 1
fi

read -p "请输入通信密钥 Token (用于接口校验): " TOKEN
if [ -z "$TOKEN" ]; then
    echo -e "${RED}Token 不能为空。${NC}"
    exit 1
fi

if [ "$MODE" == "1" ]; then
    read -p "请输入 Web 服务监听端口 [默认 19100]: " PORT
    PORT=${PORT:-19100}
    EXEC_START="/usr/local/bin/token-ray --data-dir /var/lib/token-ray --web --port ${PORT}"
    echo -e "${GREEN}配置为: 主控服务端 (端口: ${PORT})${NC}"
else
    HOSTNAME=$(hostname)
    read -p "请输入本设备名称 Device ID [默认 ${HOSTNAME}]: " DEVICE_ID
    DEVICE_ID=${DEVICE_ID:-$HOSTNAME}
    read -p "请输入主控上报地址 (例如 http://192.168.1.100:19100/api/track): " FORWARD_URL
    if [ -z "$FORWARD_URL" ]; then
        echo -e "${RED}上报地址不能为空。${NC}"
        exit 1
    fi
    EXEC_START="/usr/local/bin/token-ray --data-dir /var/lib/token-ray --device-id \"${DEVICE_ID}\" --forward-to \"${FORWARD_URL}\""
    echo -e "${GREEN}配置为: 探针端 (设备名: ${DEVICE_ID}, 上报至: ${FORWARD_URL})${NC}"
fi

# 准备 Systemd 服务和环境文件
echo -e "${YELLOW}3. 配置 Systemd 服务...${NC}"

# 安全地写入 Token 到环境文件
echo "DASHBOARD_TOKEN=\"${TOKEN}\"" > /etc/token-ray.env
chmod 600 /etc/token-ray.env

cat <<EOF > /etc/systemd/system/token-ray.service
[Unit]
Description=Token Ray
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/var/lib/token-ray
EnvironmentFile=/etc/token-ray.env
ExecStart=${EXEC_START}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

mkdir -p /var/lib/token-ray

systemctl daemon-reload
systemctl enable --now token-ray

echo -e "${GREEN}=== 部署完成！===${NC}"
echo -e "服务已后台运行并设置开机自启。"
echo -e "你可以使用以下命令查看状态和日志："
echo -e "  systemctl status token-ray"
echo -e "  journalctl -fu token-ray"
if [ "$MODE" == "1" ]; then
    echo -e "\n请在浏览器访问 http://<你的IP>:${PORT} 查看主控面板。"
fi
