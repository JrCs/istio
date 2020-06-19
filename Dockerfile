# BASE_DISTRIBUTION is used to switch between the old base distribution and distroless base images
ARG BASE_DISTRIBUTION=default

# Version is the base image version from the TLD Makefile.core.mk
ARG BASE_VERSION=1.10-dev.15

# IMG is the build toolchain from common/scripts/setup_env.sh
ARG IMG=gcr.io/istio-testing/build-tools:release-1.10-2021-10-13T21-19-05
FROM ${IMG} as builder
ARG ARCH=amd64
WORKDIR /work

#
# Env setup for Istio build
#
ENV HUB "docker.io/istio"
ENV TAG 1.10.6
ENV VERSION 1.10.6
ENV TARGET_ARCH amd64
ENV TARGET_OS linux
ENV TARGET_OUT /work/out/linux_amd64
ENV TARGET_OUT_LINUX /work/out/linux_amd64

COPY . ./
RUN make --no-print-directory -e -f Makefile.core.mk pilot-discovery

# Label the intermediate build to help locate it when needed. This may be useful for
# feeding a value for "--build-arg IMG=<image>" to use the Go build cache.
# To find the <image>, use:
#
# $ docker images --filter "label=builder" --format '{{.CreatedAt}}\t{{.ID}}' | sort -nr | head -n 1 | cut -f2
#
# The build may also be done in two phases to avoid the need to use the label:
#
# $ docker build --target builder -t builder:latest .
# $ docker build --build-arg IMG=builder:latest -t pilot:latest .
LABEL builder=

#
# The rest is adapted from pilot/docker/Dockerfile.pilot
#

# The following section is used as base image if BASE_DISTRIBUTION=default
FROM docker.io/istio/base:${BASE_VERSION} as default

# The following section is used as base image if BASE_DISTRIBUTION=distroless
FROM docker.io/istio/distroless:${BASE_VERSION} as distroless

# This will build the final image based on either default or distroless from above
# hadolint ignore=DL3006
FROM ${BASE_DISTRIBUTION}

COPY --from=builder /work/out/linux_amd64/pilot-discovery /usr/local/bin/

USER 1337:1337

ENTRYPOINT ["/usr/local/bin/pilot-discovery"]
