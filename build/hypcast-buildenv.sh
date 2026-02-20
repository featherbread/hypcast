#!/bin/sh

export CGO_ENABLED=1 GOOS=linux GOARCH="$TARGETARCH"

case $TARGETARCH in
	amd64) export CARCH=x86_64;;
	arm64) export CARCH=aarch64;;
	*)
		echo "unsupported architecture $TARGETARCH"
		return 1;;
esac

export LLVMTARGET="$CARCH-alpine-linux-musl"

sysroot_init () {
	mkdir -p /sysroot/etc/apk/keys
	cp /etc/apk/repositories /sysroot/etc/apk/
	cp /usr/share/apk/keys/*.pub /sysroot/etc/apk/keys/
	ln -s /etc/apk/cache /sysroot/etc/apk/cache
	apk add -p /sysroot --arch "$CARCH" --initdb --no-scripts "$@"
}

export CC=clang
export CGO_CFLAGS="--target=$LLVMTARGET --sysroot /sysroot"
export CGO_LDFLAGS="--target=$LLVMTARGET --sysroot /sysroot -pie -fuse-ld=lld"
export PKG_CONFIG_SYSROOT_DIR=/sysroot
export PKG_CONFIG_PATH=/sysroot/usr/lib/pkgconfig:/sysroot/usr/local/lib/pkgconfig
