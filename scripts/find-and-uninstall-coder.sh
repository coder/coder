#!/bin/bash

# Coder Uninstall/Reinstall Script
# Najde a odinstaluje existující Coder instalaci, volitelně nainstaluje novou

set -euo pipefail

# Barvy pro výstup
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

info() {
    echo -e "${BLUE}[DETECT]${NC} $1"
}

success() {
    echo -e "${CYAN}[SUCCESS]${NC} $1"
}

echo "============================================"
echo "  Coder Uninstall/Reinstall Script"
echo "============================================"
echo ""

# MENU NA ZAČÁTKU
echo "Vyber možnost:"
echo ""
echo "  1) Úplný uninstall - odstranit Coder z počítače"
echo "  2) Uninstall + Reinstall - odinstalovat starý a nainstalovat nový"
echo ""
read -p "Tvoje volba (1/2): " CHOICE
echo ""

case "$CHOICE" in
    1)
        MODE="uninstall"
        log "Režim: Úplný uninstall"
        ;;
    2)
        MODE="reinstall"
        log "Režim: Uninstall + Reinstall"
        ;;
    *)
        error "Neplatná volba!"
        exit 1
        ;;
esac

echo ""

# Kontrola zda je spuštěn jako root
if [ "$EUID" -ne 0 ]; then
    warn "Tento skript není spuštěn jako root. Některé operace mohou selhat."
    warn "Pro úplný uninstall doporučuji: sudo $0"
    echo ""
fi

# 1. NAJDI BĚŽÍCÍ CODER PROCESY
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "1. Hledám běžící Coder procesy..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

CODER_PIDS=$(pgrep -f "coder.*server" || true)
if [ -n "$CODER_PIDS" ]; then
    info "Nalezeny běžící Coder procesy:"
    ps aux | grep -E "coder.*server" | grep -v grep || true
    echo ""

    # Najdi binárku z běžícího procesu
    for PID in $CODER_PIDS; do
        BINARY_PATH=$(readlink -f "/proc/$PID/exe" 2>/dev/null || true)
        if [ -n "$BINARY_PATH" ]; then
            info "Běžící binárka: $BINARY_PATH"
        fi
    done
else
    warn "Žádné běžící Coder procesy nenalezeny"
fi
echo ""

# 2. NAJDI SYSTEMD SERVICE
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "2. Hledám systemd služby..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

SYSTEMD_SERVICES=$(systemctl list-unit-files | grep -E "coder.*service" | awk '{print $1}' || true)
if [ -n "$SYSTEMD_SERVICES" ]; then
    info "Nalezené systemd služby:"
    echo "$SYSTEMD_SERVICES"
    echo ""

    for SERVICE in $SYSTEMD_SERVICES; do
        SERVICE_FILE=$(systemctl show -p FragmentPath "$SERVICE" | cut -d= -f2)
        if [ -f "$SERVICE_FILE" ]; then
            info "Service file: $SERVICE_FILE"

            # Extrahuj cestu k binárce ze service file
            EXEC_START=$(grep "^ExecStart=" "$SERVICE_FILE" | cut -d= -f2 | awk '{print $1}')
            if [ -n "$EXEC_START" ]; then
                info "ExecStart binárka: $EXEC_START"
            fi
        fi
    done
else
    warn "Žádné systemd služby nenalezeny"
fi
echo ""

# 3. NAJDI CODER BINÁRKY
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "3. Hledám Coder binárky..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Běžné lokace
COMMON_PATHS=(
    "/usr/bin/coder"
    "/usr/local/bin/coder"
    "/opt/coder/bin/coder"
    "/home/*/bin/coder"
    "$HOME/bin/coder"
)

FOUND_BINARIES=()
for PATH_PATTERN in "${COMMON_PATHS[@]}"; do
    for BINARY in $PATH_PATTERN; do
        if [ -f "$BINARY" ]; then
            info "Nalezena binárka: $BINARY"
            VERSION=$("$BINARY" version 2>/dev/null | head -1 || echo "unknown")
            echo "   Verze: $VERSION"
            FOUND_BINARIES+=("$BINARY")
        fi
    done
done

# Hledej v celém systému (pokud nic nenalezeno)
if [ ${#FOUND_BINARIES[@]} -eq 0 ]; then
    warn "Hledám v celém systému (může trvat déle)..."
    SYSTEM_BINARIES=$(find /usr /opt /home -name "coder" -type f -executable 2>/dev/null || true)
    if [ -n "$SYSTEM_BINARIES" ]; then
        info "Nalezené binárky:"
        echo "$SYSTEM_BINARIES"
        while IFS= read -r BINARY; do
            FOUND_BINARIES+=("$BINARY")
        done <<< "$SYSTEM_BINARIES"
    fi
fi

if [ ${#FOUND_BINARIES[@]} -eq 0 ]; then
    warn "Žádné Coder binárky nenalezeny"
fi
echo ""

# 4. NAJDI CODER DATA
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "4. Hledám Coder data (.coderv2)..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

DATA_DIRS=$(find /home /root /var/lib -name ".coderv2" -type d 2>/dev/null || true)
if [ -n "$DATA_DIRS" ]; then
    info "Nalezené datové adresáře:"
    echo "$DATA_DIRS"

    while IFS= read -r DIR; do
        SIZE=$(du -sh "$DIR" 2>/dev/null | awk '{print $1}')
        echo "   Velikost: $SIZE"
    done <<< "$DATA_DIRS"
else
    warn "Žádné .coderv2 adresáře nenalezeny"
fi
echo ""

# 5. NAJDI CODER CONFIG
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "5. Hledám Coder konfigurační soubory..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

CONFIG_FILES=$(find /etc /home /root -name "coder.yaml" -o -name "coder.env" 2>/dev/null || true)
if [ -n "$CONFIG_FILES" ]; then
    info "Nalezené konfigurační soubory:"
    echo "$CONFIG_FILES"
else
    warn "Žádné konfigurační soubory nenalezeny"
fi
echo ""

# 6. SHRNUTÍ
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "SHRNUTÍ NALEZENÝCH INSTALACÍ"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

HAS_INSTALLATION=false

if [ -n "$CODER_PIDS" ]; then
    echo "✓ Běžící procesy: ANO"
    HAS_INSTALLATION=true
else
    echo "✗ Běžící procesy: NE"
fi

if [ -n "$SYSTEMD_SERVICES" ]; then
    echo "✓ Systemd služby: ANO"
    HAS_INSTALLATION=true
else
    echo "✗ Systemd služby: NE"
fi

if [ ${#FOUND_BINARIES[@]} -gt 0 ]; then
    echo "✓ Binárky nalezeny: ${#FOUND_BINARIES[@]}"
    HAS_INSTALLATION=true
else
    echo "✗ Binárky nalezeny: 0"
fi

if [ -n "$DATA_DIRS" ]; then
    echo "✓ Data adresáře (.coderv2): ANO"
    HAS_INSTALLATION=true
else
    echo "✗ Data adresáře (.coderv2): NE"
fi

echo ""

if [ "$HAS_INSTALLATION" = false ]; then
    log "Žádná Coder instalace nenalezena!"

    if [ "$MODE" = "reinstall" ]; then
        log "Můžeš pokračovat s instalací nové verze..."
    else
        log "Nic k uninstallu. Hotovo!"
        exit 0
    fi
fi

# 7. PROVEDENÍ UNINSTALLU
if [ "$HAS_INSTALLATION" = true ]; then
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "UNINSTALL"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    warn "VAROVÁNÍ: Tato operace odstraní Coder a ZAZÁLOHUJE data!"
    echo ""
    echo "Co se stane:"
    echo "  1. Zastavení běžících procesů"
    echo "  2. Zastavení systemd služeb"
    echo "  3. Smazání binárních souborů"
    echo "  4. ZÁLOHA .coderv2 do /tmp/coder-backup-$(date +%Y%m%d-%H%M%S)"
    echo "  5. Odstranění systemd služeb"
    echo ""

    if [ "$MODE" = "reinstall" ]; then
        echo "Po uninstallu bude následovat instalace nové verze."
        echo ""
    fi

    read -p "Pokračovat s uninstallem? (ano/ne): " CONFIRM

    if [ "$CONFIRM" != "ano" ]; then
        warn "Uninstall zrušen."
        exit 0
    fi

    echo ""
    log "Spouštím uninstall..."
    echo ""

    # 7.1 Zastavení procesů
    if [ -n "$CODER_PIDS" ]; then
        log "Zastavuji běžící procesy..."
        for PID in $CODER_PIDS; do
            kill "$PID" 2>/dev/null || true
            sleep 2
            # Force kill pokud stále běží
            if kill -0 "$PID" 2>/dev/null; then
                warn "Proces $PID neodpovídá, nuceně ukončuji..."
                kill -9 "$PID" 2>/dev/null || true
            fi
        done
        log "✓ Procesy zastaveny"
    fi

    # 7.2 Zastavení systemd služeb
    if [ -n "$SYSTEMD_SERVICES" ]; then
        log "Zastavuji systemd služby..."
        for SERVICE in $SYSTEMD_SERVICES; do
            systemctl stop "$SERVICE" 2>/dev/null || true
            systemctl disable "$SERVICE" 2>/dev/null || true
            log "✓ Služba $SERVICE zastavena a vypnuta"
        done
    fi

    # 7.3 Záloha dat
    BACKUP_DIR="/tmp/coder-backup-$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$BACKUP_DIR"

    if [ -n "$DATA_DIRS" ]; then
        log "Zálohuji .coderv2 data do $BACKUP_DIR..."
        while IFS= read -r DIR; do
            BACKUP_NAME=$(echo "$DIR" | sed 's/\//_/g')
            cp -r "$DIR" "$BACKUP_DIR/$BACKUP_NAME" 2>/dev/null || true
            log "✓ Zazálohováno: $DIR"
        done <<< "$DATA_DIRS"
    fi

    # 7.4 Smazání binárních souborů
    if [ ${#FOUND_BINARIES[@]} -gt 0 ]; then
        log "Odstraňuji binární soubory..."
        for BINARY in "${FOUND_BINARIES[@]}"; do
            rm -f "$BINARY" 2>/dev/null || true
            log "✓ Smazáno: $BINARY"
        done
    fi

    # 7.5 Odstranění systemd service files
    if [ -n "$SYSTEMD_SERVICES" ]; then
        log "Odstraňuji systemd service soubory..."
        for SERVICE in $SYSTEMD_SERVICES; do
            SERVICE_FILE=$(systemctl show -p FragmentPath "$SERVICE" | cut -d= -f2)
            if [ -f "$SERVICE_FILE" ]; then
                rm -f "$SERVICE_FILE" 2>/dev/null || true
                log "✓ Smazáno: $SERVICE_FILE"
            fi
        done
        systemctl daemon-reload
    fi

    # 7.6 Odstranění config files
    if [ -n "$CONFIG_FILES" ]; then
        log "Odstraňuji konfigurační soubory..."
        while IFS= read -r CONFIG; do
            rm -f "$CONFIG" 2>/dev/null || true
            log "✓ Smazáno: $CONFIG"
        done <<< "$CONFIG_FILES"
    fi

    # 7.7 Odstranění .coderv2 (volitelné v režimu uninstall)
    if [ "$MODE" = "uninstall" ]; then
        echo ""
        warn "Data v .coderv2 jsou zazálohována v: $BACKUP_DIR"
        read -p "Chceš smazat také .coderv2 adresáře? (ano/ne): " DELETE_DATA

        if [ "$DELETE_DATA" = "ano" ]; then
            if [ -n "$DATA_DIRS" ]; then
                while IFS= read -r DIR; do
                    rm -rf "$DIR" 2>/dev/null || true
                    log "✓ Smazáno: $DIR"
                done <<< "$DATA_DIRS"
            fi
        fi
    else
        # V režimu reinstall ponecháme .coderv2 (data workspace, uživatelů, atd.)
        log "Data v .coderv2 ponechána pro novou instalaci"
        log "Záloha uložena v: $BACKUP_DIR"
    fi

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    success "✓ Uninstall dokončen!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
fi

# 8. REINSTALL (pokud zvolen režim 2)
if [ "$MODE" = "reinstall" ]; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "INSTALACE NOVÉ VERZE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    log "Nyní nainstalujeme nový Coder z tvého repozitáře"
    echo ""

    read -p "Zadej cestu k repozitáři (např. /opt/coder): " REPO_PATH

    # Kontrola že cesta existuje
    if [ ! -d "$REPO_PATH" ]; then
        error "Adresář $REPO_PATH neexistuje!"
        error "Musíš nejdřív naklonovat repozitář:"
        echo "  git clone https://github.com/milhy545/coder.git $REPO_PATH"
        exit 1
    fi

    # Kontrola že je to git repozitář
    if [ ! -d "$REPO_PATH/.git" ]; then
        error "$REPO_PATH není git repozitář!"
        exit 1
    fi

    echo ""
    log "Instaluji z: $REPO_PATH"

    cd "$REPO_PATH"

    # Build
    log "Spouštím build (může trvat několik minut)..."
    if make build; then
        success "✓ Build dokončen"
    else
        error "Build selhal!"
        exit 1
    fi

    # Najdi vytvořenou binárku
    BUILT_BINARY=$(find ./build -name "coder" -type f -executable | head -1)

    if [ -z "$BUILT_BINARY" ]; then
        error "Nenalezena vytvořená binárka v ./build"
        exit 1
    fi

    log "Nalezena binárka: $BUILT_BINARY"

    # Instalace
    log "Instaluji do /usr/local/bin/coder..."
    cp "$BUILT_BINARY" /usr/local/bin/coder
    chmod +x /usr/local/bin/coder

    # Verifikace
    NEW_VERSION=$(/usr/local/bin/coder version | head -1)
    success "✓ Nainstalováno: $NEW_VERSION"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    success "✓✓✓ INSTALACE DOKONČENA ✓✓✓"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    log "Jak spustit Coder:"
    echo ""
    echo "  # Jednorázově:"
    echo "  coder server --access-url https://tvoje-url.cz"
    echo ""
    echo "  # Nebo vytvoř systemd službu:"
    echo "  sudo bash $REPO_PATH/scripts/create-systemd-service.sh"
    echo ""

    if [ -n "$BACKUP_DIR" ]; then
        log "Záloha starých dat: $BACKUP_DIR"
    fi

elif [ "$MODE" = "uninstall" ]; then
    echo ""
    if [ -n "$BACKUP_DIR" ]; then
        log "Záloha dat: $BACKUP_DIR"
    fi
    log "Pro instalaci nového Coderu:"
    echo "  git clone https://github.com/milhy545/coder.git /opt/coder"
    echo "  cd /opt/coder"
    echo "  make build"
    echo "  sudo cp ./build/coder_*/bin/coder /usr/local/bin/"
fi

echo ""
success "Hotovo!"
