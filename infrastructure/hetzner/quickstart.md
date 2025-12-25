# 🚀 InfraForge Dev Server - Hızlı Başlangıç

## 📋 Ön Gereksinimler

```bash
# 1. Hetzner Cloud hesabı aç (ücretsiz)
#    https://console.hetzner.cloud/

# 2. API Token oluştur
#    Console -> Security -> API Tokens -> Generate API Token
#    ⚠️ "Read & Write" seç!

# 3. SSH key'in olmalı
ls ~/.ssh/id_rsa.pub || ssh-keygen -t rsa -b 4096

# 4. OpenTofu kur (Mac)
brew install opentofu
```

## 🎯 Kullanım

### Server Oluştur (~3-5 dakika)

```bash
# 1. Klasöre git
cd infrastructure/hetzner

# 2. Token'ı export et
export HCLOUD_TOKEN="your-hetzner-api-token"

# 3. Tofu init
tofu init

# 4. Server oluştur
tofu apply

# Output'taki IP'yi not al!
```

### VS Code ile Bağlan

```bash
# 1. SSH config'e ekle (~/.ssh/config)
Host infraforge-dev
  HostName <SERVER_IP>
  User root
  ForwardAgent yes

# 2. VS Code'u aç
# 3. Cmd+Shift+P -> "Remote-SSH: Connect to Host"
# 4. "infraforge-dev" seç
# 5. Folder aç: /root/workspace/PaaS-Platform
```

### Lens ile Kubernetes'e Bağlan

```bash
# 1. Kubeconfig'i indir
scp root@<SERVER_IP>:/root/.kube/config-external ~/.kube/infraforge-dev

# 2. Lens'i aç
# 3. "Add Cluster" -> "Select kubeconfig"
# 4. ~/.kube/infraforge-dev seç
```

### Kubernetes Dashboard

```bash
# Server'da:
ssh root@<SERVER_IP>

# Token al
kubectl -n kubernetes-dashboard create token admin-user

# Port forward (local makinede)
ssh -L 8001:localhost:8001 root@<SERVER_IP> "kubectl proxy"

# Browser'da aç
open http://localhost:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/
```

### İşin Bitince - Server'ı Sil (PARA DURUR!)

```bash
cd infrastructure/hetzner
tofu destroy -auto-approve

# ✅ Server silindi, artık para yazmıyor!
```

---

## 📦 Server'da Hazır Gelen Her Şey

| Tool | Komut | Açıklama |
|------|-------|----------|
| **Docker** | `docker`, `dc` | Container runtime |
| **Docker Compose** | `docker compose` | Multi-container |
| **Kind** | `kind` | Kubernetes in Docker |
| **Kubectl** | `k`, `kubectl` | K8s CLI |
| **Helm** | `helm` | Package manager |
| **K9s** | `k9s` | Kubernetes TUI |
| **Vagrant** | `vagrant`, `v` | VM management |
| **Ansible** | `ansible`, `ap` | Config management |
| **OpenTofu** | `tofu`, `tf` | IaC |
| **Go** | `go` | Go compiler |
| **Node.js** | `node`, `npm` | JS runtime |

## ⌨️ Hazır Aliaslar

```bash
# Kubernetes
k       = kubectl
kgp     = kubectl get pods
kgs     = kubectl get svc
kgaa    = kubectl get all -A
kl      = kubectl logs
klf     = kubectl logs -f

# Docker
d       = docker
dc      = docker compose
dps     = docker ps
dprune  = docker system prune -af

# Navigation
ws      = cd /root/workspace/PaaS-Platform
infra   = cd /root/workspace/PaaS-Platform/infrastructure

# Kind
kind-reset = Kind cluster'ı yeniden oluştur
```

---

## 🔄 Günlük Workflow

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  SABAH                                                          │
│  ──────                                                         │
│  $ cd infrastructure/hetzner                                    │
│  $ export HCLOUD_TOKEN="xxx"                                    │
│  $ tofu apply -auto-approve                                     │
│  $ ssh infraforge-dev                                           │
│                                                                 │
│  GÜN BOYU                                                       │
│  ────────                                                       │
│  VS Code Remote SSH ile çalış                                   │
│  • Kod yaz                                                      │
│  • kind cluster'da test et                                      │
│  • vagrant ile VM test et                                       │
│  • Commit & push                                                │
│                                                                 │
│  AKŞAM                                                          │
│  ──────                                                         │
│  $ exit                                                         │
│  $ tofu destroy -auto-approve                                   │
│  # Para durdu! 🎉                                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 💰 Maliyet Hesabı

```
Server: CCX33 (32GB RAM, 8 vCPU)
Saatlik: €0.067

Günde 8 saat çalışırsan:   €0.54/gün
Ayda 20 gün çalışırsan:    €10.80/ay
Hafta sonu destroy:        €0

────────────────────────────────
Tahmini aylık maliyet:     €10-15
────────────────────────────────
```

## ⚠️ Önemli Notlar

1. **Her gün `tofu destroy` yapmayı unutma!** Aksi halde gece boyu para yazıyor.

2. **Git commit'leri push etmeyi unutma!** Server silinince local değişiklikler gider.

3. **SSH key'in server'da olmalı** (git push için):
   ```bash
   scp ~/.ssh/id_rsa root@<SERVER_IP>:/root/.ssh/
   ```

4. **Büyük dosyaları Git'e ekleme!** `.gitignore` kontrol et.

---

## 🆘 Sorun Giderme

### Server'a bağlanamıyorum
```bash
# IP doğru mu?
tofu output server_ip

# SSH key doğru mu?
ssh -v root@<IP>

# Firewall açık mı?
# Hetzner Console -> Firewalls
```

### Kind cluster çalışmıyor
```bash
# Yeniden oluştur
kind delete cluster --name infraforge-dev
kind create cluster --config /root/kind-config.yaml

# Docker çalışıyor mu?
systemctl status docker
```

### Lens bağlanmıyor
```bash
# Kubeconfig'i yeniden indir
scp root@<IP>:/root/.kube/config-external ~/.kube/infraforge-dev

# API server erişilebilir mi?
curl -k https://<IP>:6443
```
