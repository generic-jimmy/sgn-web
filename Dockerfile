# ================================================================
#  SGN Web Interface — Multi-stage Dockerfile
#  Compatible with: Railway · Render · Fly.io
#  Listens on $PORT (default 8080)
# ================================================================

# ────────────────────────────────────────────────────────────────
# Stage 1 — Build keystone engine (static lib) + SGN binary
# ────────────────────────────────────────────────────────────────
FROM golang:1.22-bookworm AS sgn-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        cmake \
        g++-multilib \
        gcc-multilib \
        git \
        libcapstone-dev \
        python3 \
        time \
    && rm -rf /var/lib/apt/lists/*

# Clone & build keystone as a fully static library
RUN git clone --depth=1 https://github.com/EgeBalci/keystone /ks
RUN mkdir /ks/build
WORKDIR /ks/build
RUN ../make-lib.sh
RUN cmake -DCMAKE_BUILD_TYPE=Release \
          -DBUILD_SHARED_LIBS=OFF \
          -DLLVM_TARGETS_TO_BUILD="AArch64;X86" \
          -G "Unix Makefiles" ..
RUN make -j$(nproc)
RUN make install && ldconfig

# Clone & build SGN as a fully static binary (no libc deps at runtime)
RUN git clone --depth=1 https://github.com/EgeBalci/sgn /sgn-src
WORKDIR /sgn-src
RUN go build \
        -o /out/sgn \
        -ldflags '-w -s -extldflags -static' \
        -trimpath \
        main.go

# ────────────────────────────────────────────────────────────────
# Stage 2 — Build the Go web server (CGO disabled, fully static)
# ────────────────────────────────────────────────────────────────
FROM golang:1.22-bookworm AS web-builder

WORKDIR /app
COPY main.go go.mod ./
COPY static/ ./static/

RUN CGO_ENABLED=0 GOOS=linux go build \
        -o /out/sgn-web \
        -ldflags '-w -s' \
        -trimpath \
        .

# ────────────────────────────────────────────────────────────────
# Stage 3 — Final minimal image (~10 MB)
# Both binaries are statically linked — no runtime deps needed
# ────────────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && mkdir -p /tmp \
    && chmod 1777 /tmp

COPY --from=sgn-builder /out/sgn     /usr/local/bin/sgn
COPY --from=web-builder /out/sgn-web /usr/local/bin/sgn-web

# Default port — all three platforms override this with $PORT at runtime
EXPOSE 8080
ENV PORT=8080

# Optional: raise the encoding timeout for ASCII brute-force mode
# ENV SGN_TIMEOUT=15m

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:${PORT}/health || exit 1

CMD ["/usr/local/bin/sgn-web"]
