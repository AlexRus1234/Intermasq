#!/bin/sh
set -eu

SCRIPT_DIR=/src/distro
OUTPUT_DIR=/out
BUILD_DIR=/tmp/intermasq-lab-build
ROOTFS="$BUILD_DIR/rootfs"
INITRAMFS="$BUILD_DIR/initramfs"
ISO_ROOT="$BUILD_DIR/iso"

. "$SCRIPT_DIR/build.env"

rm -rf "$BUILD_DIR"
mkdir -p "$ROOTFS" "$INITRAMFS" "$ISO_ROOT/boot" "$ISO_ROOT/isolinux" "$OUTPUT_DIR"

apk --root "$ROOTFS" --initdb --keys-dir /etc/apk/keys \
	--repositories-file /etc/apk/repositories \
	add --no-cache \
	alpine-base apk-tools bash ca-certificates curl dnsmasq iproute2 \
	linux-virt mkinitfs openssh openssl openrc socat

cp -a "$SCRIPT_DIR/rootfs/." "$ROOTFS/"
install -d -m 0755 "$ROOTFS/usr/local/lib" "$ROOTFS/usr/local/sbin"
install -d -m 0755 "$ROOTFS/usr/share/doc/intermasq-lab"
cp "$SCRIPT_DIR/MANUAL.md" "$ROOTFS/usr/share/doc/intermasq-lab/MANUAL.md"

curl -fL --retry 3 "$INTERMASQ_BINARY_URL" -o "$ROOTFS/usr/local/lib/intermasq"
printf '%s  %s\n' "$INTERMASQ_BINARY_SHA256" "$ROOTFS/usr/local/lib/intermasq" | sha256sum -c -
chmod 0755 "$ROOTFS/usr/local/lib/intermasq"

cat > "$ROOTFS/etc/network/interfaces" <<'EOF'
auto lo
iface lo inet loopback
EOF

printf '%s\n' 'intermasq-lab' > "$ROOTFS/etc/hostname"
cat > "$ROOTFS/etc/motd" <<'EOF'
Intermasq Lab

The lab starts automatically after networking is ready.
The console prints the detected network interface and panel URLs.

Panels: 8082 office, 8083 lab, 8084 demo
Admin: admin / intermasq-lab
User:  operator / operator-lab

VM NIC must be virtio-net (QEMU/KVM), VMXNET3 (VMware) or
paravirtualized (VirtualBox). The kernel does not include e1000.
EOF

printf '%s\n' 'root:intermasq-lab' | chroot "$ROOTFS" chpasswd
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin yes/' "$ROOTFS/etc/ssh/sshd_config"
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' "$ROOTFS/etc/ssh/sshd_config"
chroot "$ROOTFS" rc-update add networking boot
chroot "$ROOTFS" rc-update add sshd default
chroot "$ROOTFS" rc-update add intermasq-lab default

cat > "$INITRAMFS/init" <<'EOF'
#!/bin/sh

export PATH=/sbin:/usr/sbin:/bin:/usr/bin

mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev 2>/dev/null || mount -t tmpfs tmpfs /dev
mkdir -p /dev/pts
mount -t devpts devpts /dev/pts
mount -t tmpfs tmpfs /run

# Load network drivers for all major hypervisors before OpenRC starts.
# linux-virt has many of these built-in; modprobe just skips those.
for mod in virtio_pci virtio_net virtio_blk virtio_scsi \
           vmxnet3 hv_netvsc hv_vmbus \
           e1000 e1000e igb ixgbe 8139too 8139cp pcnet32 ne2k_pci; do
	modprobe "$mod" 2>/dev/null
done

# Coldplug: detect remaining PCI/USB devices and create /dev nodes
mdev -s 2>/dev/null

exec /sbin/init
EOF
chmod 0755 "$INITRAMFS/init"

tar -C "$ROOTFS" -cf - . | tar -C "$INITRAMFS" -xf -
cp "$ROOTFS/boot/vmlinuz-virt" "$ISO_ROOT/boot/vmlinuz"
(cd "$INITRAMFS" && find . -print | cpio -o -H newc 2>/dev/null | gzip -9 > "$ISO_ROOT/boot/initramfs.img")

cp /usr/share/syslinux/isolinux.bin "$ISO_ROOT/isolinux/isolinux.bin"
cp /usr/share/syslinux/ldlinux.c32 "$ISO_ROOT/isolinux/ldlinux.c32"
cat > "$ISO_ROOT/isolinux/isolinux.cfg" <<'EOF'
DEFAULT intermasq
PROMPT 0
TIMEOUT 30

LABEL intermasq
  KERNEL /boot/vmlinuz
  APPEND initrd=/boot/initramfs.img console=tty0 console=ttyS0,115200n8
EOF

ISO_PATH="$OUTPUT_DIR/intermasq-lab-${INTERMASQ_RELEASE#v}-${ALPINE_ARCH}.iso"
xorriso -as mkisofs \
	-o "$ISO_PATH" \
	-V INTERMASQLAB \
	-b isolinux/isolinux.bin \
	-c isolinux/boot.cat \
	-no-emul-boot \
	-boot-load-size 4 \
	-boot-info-table \
	-isohybrid-mbr /usr/share/syslinux/isohdpfx.bin \
	"$ISO_ROOT"

sha256sum "$ISO_PATH" > "$ISO_PATH.sha256"
printf 'created: %s\n' "$ISO_PATH"
