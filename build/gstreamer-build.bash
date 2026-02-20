#!/usr/bin/env bash
set -eu

info () { printf '\033[35m[gstreamer-build] \033[34m%s\033[0m\n' "$*"; }

source /hypcast-buildenv.sh
info "Building for LLVM target $LLVMTARGET"

# See https://mesonbuild.com/Cross-compilation.html
info "Writing Meson --cross-file"
cat > meson-cross.txt <<EOF
[host_machine]
system = 'linux'
cpu_family = '$CARCH'
cpu = '$CARCH'
endian = 'little'

[constants]
llvm_cross_args = ['--target=$LLVMTARGET', '--sysroot', '/sysroot']

[binaries]
c = ['clang'] + llvm_cross_args
cpp = ['clang++'] + llvm_cross_args
c_ld = 'lld'
cpp_ld = 'lld'
pkgconfig = 'pkg-config'
strip = 'llvm-strip'

[properties]
sys_root = '/sysroot'

[built-in options]
pkg_config_path = ['/sysroot/usr/lib/pkgconfig']
EOF

info "Starting Meson setup"
meson setup \
	--buildtype=release \
	--cross-file=meson-cross.txt \
	-Db_staticpic=true \
	-Db_pie=true \
	-Dauto_features=disabled \
	-Dgpl=enabled \
	-Dtools=enabled \
	-Dbase=enabled \
	-Dgst-plugins-base:app=enabled \
	-Dgst-plugins-base:audioconvert=enabled \
	-Dgst-plugins-base:audioresample=enabled \
	-Dgst-plugins-base:opus=enabled \
	-Dgst-plugins-base:videoconvertscale=enabled \
	-Dgst-plugins-base:videorate=enabled \
	-Dgood=enabled \
	-Dgst-plugins-good:deinterlace=enabled \
	-Dbad=enabled \
	-Dgst-plugins-bad:dvb=enabled \
	-Dgst-plugins-bad:mpegtsdemux=enabled \
	-Dugly=enabled \
	-Dgst-plugins-ugly:a52dec=enabled \
	-Dgst-plugins-ugly:x264=enabled \
	-Dlibav=enabled \
	output

info "Starting Meson build"
meson compile -C output

info "Installing GStreamer to /gstreamer"
meson install --destdir /gstreamer --strip -C output

info "GStreamer build complete"
