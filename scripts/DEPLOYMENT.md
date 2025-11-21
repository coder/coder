# Automatic Deployment Setup

Tento systém automaticky aktualizuje Coder na serveru při každém push do `main` branch.

## 🚀 Quick Setup

### 1. Nastavení GitHub Secrets

V GitHubu (Settings → Secrets and variables → Actions) přidej:

```
DEPLOY_HOST=<IP_adresa_serveru>
DEPLOY_USER=<uzivatel_s_pristupem>
DEPLOY_SSH_KEY=<privátní_SSH_klíč>
DEPLOY_PORT=22  # (optional, default: 22)
DEPLOY_PATH=/opt/coder  # (optional, default: /opt/coder)
```

### 2. Příprava serveru

Na serveru:

```bash
# 1. Vytvoř deployment directory
sudo mkdir -p /opt/coder
sudo chown $USER:$USER /opt/coder

# 2. Naklonuj repository
cd /opt/coder
git clone https://github.com/milhy545/coder.git .

# 3. Nastav SSH klíč pro GitHub Actions
# (veřejný klíč z páru, kde soukromý jsi dal do DEPLOY_SSH_KEY)
mkdir -p ~/.ssh
echo "ssh-ed25519 AAAA..." >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# 4. (Optional) Setup systemd service
sudo tee /etc/systemd/system/coder.service > /dev/null <<EOF
[Unit]
Description=Coder Development Platform
After=network.target postgresql.service

[Service]
Type=simple
User=$USER
WorkingDirectory=/opt/coder
ExecStart=/opt/coder/build/coder_linux_amd64 server
Restart=always
RestartSec=10

# Environment variables
Environment="CODER_ACCESS_URL=https://your-domain.com"
Environment="CODER_PG_CONNECTION_URL=postgresql://user:pass@localhost/coder?sslmode=disable"

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable coder
sudo systemctl start coder
```

### 3. Generování SSH klíče (pokud nemáš)

```bash
# Na svém počítači:
ssh-keygen -t ed25519 -C "github-actions-coder" -f ~/.ssh/github_actions_coder

# Veřejný klíč (přidej na server do ~/.ssh/authorized_keys):
cat ~/.ssh/github_actions_coder.pub

# Soukromý klíč (přidej do GitHub Secrets jako DEPLOY_SSH_KEY):
cat ~/.ssh/github_actions_coder
```

## 🔄 Jak to funguje

### Workflow (.github/workflows/deploy.yml)

1. **Trigger**: Push do `main` branch nebo manuální spuštění
2. **Actions**:
   - Checkout kódu
   - SSH připojení na server
   - Spuštění `scripts/deploy.sh`

### Deployment Script (scripts/deploy.sh)

Automaticky:
- ✅ Vytvoří backup současné verze
- ✅ Stáhne nejnovější změny z GitHubu
- ✅ Detekuje potřebu rebuildu (Go/frontend změny)
- ✅ Sestaví novou verzi (pokud nutné)
- ✅ Restartuje Coder service
- ✅ Loguje vše do `/var/log/coder-deploy.log`

## 📋 Manuální nasazení

Pokud chceš spustit deployment ručně:

```bash
cd /opt/coder
bash scripts/deploy.sh
```

## 🔍 Monitorování

### Logy deployment
```bash
tail -f /var/log/coder-deploy.log
```

### Logy Coder služby
```bash
# Systemd
journalctl -u coder -f

# Nebo přímo
tail -f ~/.cache/coder/coder.log
```

### Status služby
```bash
systemctl status coder
```

## 🛡️ Bezpečnost

- SSH klíč je uložen jako GitHub Secret (šifrovaný)
- Deployment script běží s právy uživatele (ne root)
- Backupy uchovávají posledních 5 verzí
- Rollback možný přes backupy

## 🔙 Rollback

Pokud nová verze nefunguje:

```bash
cd /opt/coder-backups
ls -lt  # zobraz backupy

# Obnov backup
sudo systemctl stop coder
cd /opt/coder
rm -rf .coderv2
cp -r /opt/coder-backups/coder-backup-XXXXX/.coderv2 .
sudo systemctl start coder
```

## 📝 Co se deployuje automaticky

- ✅ **Backend změny** (Go code) → rebuild + restart
- ✅ **Frontend změny** (site/) → rebuild + restart
- ✅ **Config změny** (YAML, env) → restart
- ✅ **Dokumentace** → pouze pull (bez restartu)
- ✅ **Database migrations** → automaticky při startu

## ⚡ Performance Tips

**První build může trvat 5-15 minut** (kompilace Go + frontend).
Další deploymenty jsou rychlejší (využívají cache).

**Optimalizace:**
- Incremental builds (make používá cache)
- Frontend hot reload pro development
- Database migrations paralelně

## 🐛 Troubleshooting

### Deployment selhal?

1. **Zkontroluj GitHub Actions log**
2. **Zkontroluj server log**: `tail -f /var/log/coder-deploy.log`
3. **Zkontroluj SSH přístup**: `ssh -i ~/.ssh/key user@server`
4. **Zkontroluj permissions**: `ls -la /opt/coder`

### Build selže?

```bash
# Manuální rebuild
cd /opt/coder
make clean
make build
```

### Service se nespustí?

```bash
# Debug mode
cd /opt/coder
./build/coder_linux_amd64 server --verbose
```

## 📞 Support

Při problémech:
- GitHub Issues: https://github.com/milhy545/coder/issues
- Deployment logs: `/var/log/coder-deploy.log`
- Build logs: `make build 2>&1 | tee build.log`
