#!/usr/bin/env bash
# Run once on a fresh Ubuntu 22.04 EC2 instance.
# Usage: bash setup.sh

set -e

echo "=== Installing system packages ==="
sudo apt-get update -y
sudo apt-get install -y nginx python3 python3-pip python3-venv git docker.io docker-compose-v2

sudo systemctl enable --now docker
sudo systemctl enable --now nginx

echo "=== Cloning repo ==="
cd /home/ubuntu
git clone https://github.com/YOUR_USERNAME/YOUR_REPO.git projectir
cd projectir

echo "=== Python virtual environment ==="
python3 -m venv preprocess/venv
preprocess/venv/bin/pip install --upgrade pip
preprocess/venv/bin/pip install -r preprocess/requirements.txt

echo "=== Go binary ==="
# Install Go (detect ARM64 vs AMD64 automatically)
ARCH=$(uname -m)
if [ "$ARCH" = "aarch64" ]; then
  GO_FILE="go1.22.3.linux-arm64.tar.gz"
else
  GO_FILE="go1.22.3.linux-amd64.tar.gz"
fi
wget -q "https://go.dev/dl/${GO_FILE}"
sudo tar -C /usr/local -xzf "$GO_FILE"
rm "$GO_FILE"
export PATH=$PATH:/usr/local/go/bin

cd server
/usr/local/go/bin/go build -o server main.go
cd ..

echo "=== Frontend build ==="
# Install Node
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs
cd frontend
npm install
npm run build
sudo mkdir -p /var/www/projectir
sudo cp -r dist/* /var/www/projectir/
cd ..

echo "=== Nginx ==="
sudo cp nginx.conf /etc/nginx/sites-available/projectir
sudo ln -sf /etc/nginx/sites-available/projectir /etc/nginx/sites-enabled/projectir
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx

echo "=== Systemd services ==="
sudo cp deploy/projectir-server.service /etc/systemd/system/
sudo cp deploy/projectir-recommender.service /etc/systemd/system/
sudo systemctl daemon-reload

echo "=== Elasticsearch ==="
sudo docker compose up -d

echo ""
echo "Setup complete!"
echo "Next steps:"
echo "  1. Copy your .env file:  scp .env ubuntu@YOUR_IP:/home/ubuntu/projectir/.env"
echo "  2. Copy your model:      scp preprocess/models/recommender_model.pkl ubuntu@YOUR_IP:/home/ubuntu/projectir/preprocess/models/"
echo "  3. Start services:       bash deploy/start.sh"
