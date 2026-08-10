#!/bin/bash
# distro/build-ci.sh
#
# Собирает лабораторный ISO внутри CI-контейнера fedora:44 БЕЗ podman.
# Linux есть Linux: apk-tools, syslinux и xorriso ставятся прямо в fedora,
# после чего прогоняется оригинальный distro/build-inside.sh без единой правки
# (он и так работает через apk --root + chroot — никаких вложенных контейнеров).
#
# Использование (из корня репо):
#   INTERMASQ_RELEASE=v1.0.0a ./distro/build-ci.sh
#
# Переменные окружения:
#   INTERMASQ_RELEASE    — тег версии (v1.0.0a); влияет на имя ISO. Обязателен.
#   INTERMASQ_CI_BINARY  — путь к локально собранному бинарнику Intermasq
#                           (по умолчанию $REPO_DIR/intermasq-ci).
#   ALPINE_VERSION       — версия Alpine для rootfs (по умолчанию 3.24.1).
#   OUTPUT_DIR           — куда положить готовый ISO (по умолчанию distro/output).
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BINARY="${INTERMASQ_CI_BINARY:-$REPO_DIR/intermasq-ci}"
RELEASE="${INTERMASQ_RELEASE:?INTERMASQ_RELEASE must be set (e.g. v1.0.0a)}"
OUTPUT_DIR="${OUTPUT_DIR:-$REPO_DIR/distro/output}"
ALPINE_VERSION="${ALPINE_VERSION:-3.24.1}"
ALPINE_BRANCH="v${ALPINE_VERSION%.*}"   # 3.24.1 → v3.24

if [ ! -x "$BINARY" ]; then
	echo "::error::binary not found or not executable: $BINARY" >&2
	exit 1
fi

echo "::group::Install Alpine build toolchain on fedora (apk, cpio, xorriso, ...)"
dnf install -y --setopt=install_weak_deps=False \
	apk-tools cpio gzip xorriso curl tar findutils
apk --version
echo "::endgroup::"

echo "::group::Configure Alpine apk keyring + repositories"
# fedora не имеет /etc/apk/keys. Каталог /alpine/keys/ на CDN отдаёт 404,
# поэтому ключи берём напрямую из пакета alpine-keys из main-репозитория:
# это .apk (конкатенация gzip-потоков sig+control+data), тащим curl'ом и
# распаковываем gunzip + tar --ignore-zeros (читает мимо EOF-маркеров).
cat > /etc/apk/repositories <<EOF
https://dl-cdn.alpinelinux.org/alpine/${ALPINE_BRANCH}/main
https://dl-cdn.alpinelinux.org/alpine/${ALPINE_BRANCH}/community
EOF

REPO_BASE="https://dl-cdn.alpinelinux.org/alpine/${ALPINE_BRANCH}/main/x86_64"
KEYS_PKG="$(curl -fsSL "$REPO_BASE/" \
	| grep -oE 'alpine-keys-[0-9][^"[:space:]]*\.apk' \
	| grep -vE -- '-(doc|dbg|dev|openrc)' \
	| sort -V | tail -1)"
if [ -z "$KEYS_PKG" ]; then
	echo "::error::could not find alpine-keys package in $REPO_BASE" >&2
	exit 1
fi
echo "Downloading $KEYS_PKG ..."
curl -fsSL "$REPO_BASE/$KEYS_PKG" -o /tmp/alpine-keys.apk
mkdir -p /etc/apk/keys /tmp/alpine-keys-extract
# .apk = несколько gzip tarball'ов подряд; gunzip сливает все потоки,
# --ignore-zeros заставляет tar читать за пределами энд-маркеров.
gunzip -c /tmp/alpine-keys.apk \
	| tar -xf - -C /tmp/alpine-keys-extract --ignore-zeros
cp /tmp/alpine-keys-extract/usr/share/apk/keys/*.pub /etc/apk/keys/ 2>/dev/null || {
	echo "::error::no .rsa.pub found inside alpine-keys package" >&2
	find /tmp/alpine-keys-extract -type f | head -50
	exit 1
}
rm -rf /tmp/alpine-keys.apk /tmp/alpine-keys-extract
echo "Installed $(ls /etc/apk/keys/*.pub 2>/dev/null | wc -l) signing keys:"
ls /etc/apk/keys/
cat /etc/apk/repositories
echo "::endgroup::"

echo "::group::Stage syslinux bootloader files"
# build-inside.sh копирует isolinux.bin / ldlinux.c32 / isohdpfx.bin из
# /usr/share/syslinux/ ХОСТА (не из rootfs — в rootfs syslinux не ставится).
# В fedora их нет, поэтому ставим Alpine-пакет syslinux во временный rootfs
# и вынимаем файлы оттуда. Проверяем все три нужных файла.
need_syslinux=0
for f in isolinux.bin ldlinux.c32 isohdpfx.bin; do
	[ -f "/usr/share/syslinux/$f" ] || need_syslinux=1
done
if [ "$need_syslinux" -eq 1 ]; then
	SYSLINUX_TMP="$(mktemp -d)"
	apk add --no-cache --root "$SYSLINUX_TMP" --initdb \
		--keys-dir /etc/apk/keys \
		--repositories-file /etc/apk/repositories syslinux
	mkdir -p /usr/share/syslinux
	cp -a "$SYSLINUX_TMP"/usr/share/syslinux/. /usr/share/syslinux/
	rm -rf "$SYSLINUX_TMP"
fi
ls -l /usr/share/syslinux/isolinux.bin \
      /usr/share/syslinux/ldlinux.c32 \
      /usr/share/syslinux/isohdpfx.bin
echo "::endgroup::"

echo "::group::Run distro/build-inside.sh (original recipe, unmodified)"
# build-inside.sh хардкодит SCRIPT_DIR=/src/distro и OUTPUT_DIR=/out.
# Делаем симлинк /src → репо (read-only там не нужно: скрипт пишет в /tmp и /out)
# и пустой каталог /out под результат.
ln -sfn "$REPO_DIR" /src
mkdir -p /out

BINARY_SHA256="$(sha256sum "$BINARY" | awk '{print $1}')"
BINARY_BASENAME="$(basename "$BINARY")"
echo "Injecting local binary into recipe via env overrides:"
echo "  INTERMASQ_BINARY_URL=file:///src/$BINARY_BASENAME"
echo "  INTERMASQ_BINARY_SHA256=$BINARY_SHA256"
echo "  INTERMASQ_RELEASE=$RELEASE"

# build.env теперь использует ${VAR:-default}, поэтому env-вары имеют приоритет.
# curl умеет file://, проверка sha256sum -c внутри рецепта проходит корректно:
# хэш наш, бинарник наш.
INTERMASQ_RELEASE="$RELEASE" \
INTERMASQ_BINARY_URL="file:///src/$BINARY_BASENAME" \
INTERMASQ_BINARY_SHA256="$BINARY_SHA256" \
ALPINE_ARCH=x86_64 \
sh /src/distro/build-inside.sh
echo "::endgroup::"

echo "::group::Collect output"
# build-inside.sh пишет: $OUTPUT_DIR/intermasq-lab-${RELEASE#v}-${ALPINE_ARCH}.iso
ISO_NAME="intermasq-lab-${RELEASE#v}-x86_64.iso"
if [ ! -f "/out/$ISO_NAME" ]; then
	echo "::error::ISO not found at /out/$ISO_NAME" >&2
	ls -la /out/ || true
	exit 1
fi
mkdir -p "$OUTPUT_DIR"
cp "/out/$ISO_NAME" "$OUTPUT_DIR/$ISO_NAME"
if [ -f "/out/$ISO_NAME.sha256" ]; then
	cp "/out/$ISO_NAME.sha256" "$OUTPUT_DIR/$ISO_NAME.sha256"
else
	sha256sum "$OUTPUT_DIR/$ISO_NAME" | awk '{print $1}' > "$OUTPUT_DIR/$ISO_NAME.sha256"
fi
ls -lh "$OUTPUT_DIR/$ISO_NAME" "$OUTPUT_DIR/$ISO_NAME.sha256"

# Пробрасываем имя ISO в $GITHUB_ENV для следующих шагов публикации (если запущено в CI).
if [ -n "${GITHUB_ENV:-}" ]; then
	echo "ISO_NAME=$ISO_NAME" >> "$GITHUB_ENV"
fi
echo "::endgroup::"
