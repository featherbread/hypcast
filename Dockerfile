# syntax = docker.io/docker/dockerfile:1.21

# The Alpine and Go base images must use the same release of Alpine.
ARG ALPINE_BASE=docker.io/library/alpine:3.23
ARG GOLANG_BASE=docker.io/library/golang:1.26-alpine3.23
# The Node.js image does not require any particular OS.
ARG NODEJS_BASE=docker.io/library/node:24-alpine
# See https://gstreamer.freedesktop.org/news/.
ARG GSTREAMER_VERSION=1.26.10

# Mirror of https://gitlab.freedesktop.org/gstreamer/gstreamer.git.
# Since this is formally marked as a "public mirror" in the GitHub web UI, my
# understanding is that GitHub (the entity I trust to host and build Hypcast
# itself) does the mirroring. Cloning from Git and doing a Meson build from the
# checkout isn't as good as building real ".apk" files from a source tarball.
# But until I put in the work to make this GStreamer build more than a pile of
# shameful hacks, I'd rather use Microsoft's bandwidth and resources than those
# of the freedesktop.org maintainers.
ARG GSTREAMER_REPO=https://github.com/GStreamer/gstreamer.git


# Let's get the client build out of the way, since it's much simpler than
# everything that follows.
FROM --platform=$BUILDPLATFORM $NODEJS_BASE AS client-build
ENV BUILD_PATH=/dist
RUN \
  --mount=type=bind,source=client,target=/mnt/client,rw \
  --mount=type=cache,id=hypcast.node_modules,target=/mnt/client/node_modules \
  --mount=type=cache,id=hypcast.yarn,target=/usr/local/share/.cache/yarn \
  cd /mnt/client && \
  yarn install --frozen-lockfile && \
  yarn build


# This Dockerfile is designed to produce multi-architecture images without
# emulating the target architecture on the build host (which can be very slow).
# The images are based on Alpine Linux with a custom build of GStreamer, where
# all C components are built with LLVM.
#
# Why a custom GStreamer? Alpine 3.16 and up ship gst-plugins-ugly without the
# mpeg2dec plugin, which is an absolute requirement for Hypcast. As a bonus, we
# can reduce the image size by only including plugins we actually need. We
# still use Alpine's versions of GLib and the underlying codecs.
#
# Why LLVM? Alpine does not ship a full set of gcc-based cross toolchains for
# every build host architecture (e.g. no x86_64 toolchain on aarch64 hosts).
# Even if it did, the consistency of the LLVM-based setup provides greater
# confidence that a build executed on one architecture will work on others.


# The build sysroot layer provides development headers and important support
# files for cross-compilation to the target platform. Other build layers will
# mount it as necessary.
FROM --platform=$BUILDPLATFORM $ALPINE_BASE AS build-sysroot
ARG TARGETARCH TARGETVARIANT
COPY build/hypcast-buildenv.sh /hypcast-buildenv.sh
RUN \
  source /hypcast-buildenv.sh && \
  sysroot_init gcc libc-dev libstdc++-dev glib-dev a52dec-dev libmpeg2-dev opus-dev x264-dev


# The GStreamer build base layer sets up parts of the GStreamer build that are
# common to all target platforms. (TODO: Can we not clone GStreamer from Git?
# Does Cerbero support the level of build customization we're looking for?)
FROM --platform=$BUILDPLATFORM $ALPINE_BASE AS gst-build-base
RUN apk add --no-cache bash git clang lld llvm pkgconf meson flex bison glib-dev
ARG GSTREAMER_REPO GSTREAMER_VERSION
RUN git clone -b $GSTREAMER_VERSION --depth 1 $GSTREAMER_REPO /tmp/gstreamer
WORKDIR /tmp/gstreamer
COPY build/hypcast-buildenv.sh /hypcast-buildenv.sh
COPY build/gstreamer-build.bash .


# The GStreamer build layer cross-compiles GStreamer for a specific target
# platform, with everything installed under the /gstreamer directory. The build
# output includes the shared libraries themselves along with headers and
# pkg-config manifests, so it is both mounted into the server build and copied
# to the final image as necessary.
FROM --platform=$BUILDPLATFORM gst-build-base AS gst-build
ARG TARGETARCH TARGETVARIANT
RUN \
  --mount=type=bind,from=build-sysroot,source=/sysroot,target=/sysroot \
  ./gstreamer-build.bash


# The server build base layer sets up parts of the server build that are common
# to all target platforms.
FROM --platform=$BUILDPLATFORM $GOLANG_BASE AS server-build-base
RUN apk add --no-cache clang lld pkgconf
COPY build/hypcast-buildenv.sh /hypcast-buildenv.sh
RUN \
  --mount=type=bind,target=/mnt/hypcast \
  --mount=type=cache,id=hypcast.go-pkg,target=/go/pkg \
  --mount=type=cache,id=hypcast.go-build,target=/root/.cache/go-build \
  cd /mnt/hypcast && go mod download


# The server build layer compiles the hypcast-server binary using a sysroot
# that combines the Alpine and GStreamer headers and pkg-config manifests. See
# hypcast-buildenv.sh for the setup of important environment variables.
FROM --platform=$BUILDPLATFORM server-build-base AS server-build
ARG TARGETARCH TARGETVARIANT
RUN \
  --mount=type=bind,from=build-sysroot,source=/sysroot,target=/sysroot,rw \
  --mount=type=bind,from=gst-build,source=/gstreamer/usr/local,target=/sysroot/usr/local \
  --mount=type=bind,target=/mnt/hypcast \
  --mount=type=cache,id=hypcast.go-pkg,target=/go/pkg \
  --mount=type=cache,id=hypcast.go-build,target=/root/.cache/go-build \
  cd /mnt/hypcast && \
  source /hypcast-buildenv.sh && \
  go build -v \
    -ldflags="-extld=$CC -s -w" -buildmode=pie \
    -o /hypcast-server \
    ./cmd/hypcast-server


# The target sysroot layer bootstraps the root filesystem for the target image.
# Note that this will be a distroless-style image that does not resemble a
# typical Alpine Linux environment, as we can't run the apk scripts required to
# fully set up packages like Busybox (apk would try to run them using the
# target architecture's shell, which requires emulation).
FROM --platform=$BUILDPLATFORM $ALPINE_BASE AS target-sysroot
ARG TARGETARCH TARGETVARIANT
COPY build/hypcast-buildenv.sh /hypcast-buildenv.sh
RUN \
  source /hypcast-buildenv.sh && \
  sysroot_init tini libstdc++ glib a52dec libmpeg2 opus x264-libs


# The final image simply assembles the results of previous build steps.
FROM scratch AS target

COPY --link --from=target-sysroot /sysroot /
COPY --link --from=gst-build /gstreamer /
COPY --link --from=server-build /hypcast-server /opt/hypcast/bin/hypcast-server
COPY --link --from=client-build /dist /opt/hypcast/share/www

ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
EXPOSE 9200
ENTRYPOINT [ \
  "/sbin/tini", "--", \
  "/opt/hypcast/bin/hypcast-server", \
  "-assets", "/opt/hypcast/share/www" ]
