#!/bin/sh
# Community server installer (docs/operations/install.md).
# curl -fsSL https://raw.githubusercontent.com/opencollective/community/main/install.sh | sh
set -eu

REPO="opencollective/community"
PREFIX="/opt/community"
USER="community"

say() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = "0" ] || die "run as root (use sudo)."
[ "$(uname -s)" = "Linux" ] || die "Community installs on Linux only."
command -v systemctl >/dev/null 2>&1 || die "systemd is required."

case "$(uname -m)" in
	x86_64|amd64) ARCH=amd64 ;;
	aarch64|arm64) ARCH=arm64 ;;
	*) die "unsupported architecture: $(uname -m)" ;;
esac

# --uninstall stops and removes the services and binaries, never the data.
if [ "${1:-}" = "--uninstall" ]; then
	say "stopping services"
	systemctl disable --now communityd zooid 2>/dev/null || true
	rm -f /etc/systemd/system/communityd.service /etc/systemd/system/zooid.service
	rm -f /usr/local/bin/community
	rm -rf "$PREFIX/bin"
	systemctl daemon-reload
	say "removed. Your data is untouched at $PREFIX (delete it by hand if you mean to)."
	exit 0
fi

VERSION="${VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
	say "finding the latest release"
	VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
		| grep -m1 '"tag_name"' | cut -d'"' -f4)
	[ -n "$VERSION" ] || die "could not find a release. Pass VERSION=vX.Y.Z."
fi
TARBALL="community-${VERSION}-linux-${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VERSION"

say "installing Community $VERSION ($ARCH)"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$BASE/$TARBALL" -o "$TMP/$TARBALL" || die "download failed: $BASE/$TARBALL"
curl -fsSL "$BASE/checksums.txt" -o "$TMP/checksums.txt" || die "checksums download failed"

say "verifying checksum"
( cd "$TMP" && grep " $TARBALL\$" checksums.txt | sha256sum -c - ) || die "checksum mismatch"

id "$USER" >/dev/null 2>&1 || useradd --system --home "$PREFIX" --shell /usr/sbin/nologin "$USER"

say "laying out $PREFIX"
mkdir -p "$PREFIX/bin" "$PREFIX/data/zooid" "$PREFIX/communities" \
	"$PREFIX/config/zooid" "$PREFIX/media" "$PREFIX/acme"
tar -xzf "$TMP/$TARBALL" -C "$PREFIX/bin"
chmod +x "$PREFIX/bin/communityd" "$PREFIX/bin/zooid"
ln -sf "$PREFIX/bin/communityd" /usr/local/bin/community
chown -R "$USER:$USER" "$PREFIX"

say "installing systemd units"
for unit in zooid communityd; do
	curl -fsSL "$BASE/$unit.service" -o "/etc/systemd/system/$unit.service" 2>/dev/null \
		|| die "could not fetch $unit.service"
done
systemctl daemon-reload

if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q active; then
	say "opening ports 80 and 443 (ufw)"; ufw allow 80/tcp >/dev/null; ufw allow 443/tcp >/dev/null
elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
	say "opening ports 80 and 443 (firewalld)"
	firewall-cmd --permanent --add-service=http >/dev/null
	firewall-cmd --permanent --add-service=https >/dev/null
	firewall-cmd --reload >/dev/null
fi

say "starting services"
systemctl enable --now zooid communityd

IP=$(curl -fsSL https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')
printf '\n\033[1;32mCommunity is running.\033[0m\n'
printf 'Open the setup wizard:  \033[1mhttp://%s/setup\033[0m\n\n' "$IP"
printf 'Logs:    journalctl -u communityd -f\n'
printf 'Status:  systemctl status communityd zooid\n'
