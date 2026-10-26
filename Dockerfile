# Build frontend UI assets on host platform (fast native execution)
FROM --platform=$BUILDPLATFORM node:20-alpine AS ui-builder
WORKDIR /app/ui
COPY ui/package*.json ./
RUN npm install
COPY ui/ ./
RUN npm run build

# Build Go controller binary with native compiler targeting TARGETARCH
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS go-builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY api/ api/
COPY pkg/ pkg/
COPY cmd/ cmd/
COPY --from=ui-builder /app/ui/dist ui/dist

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o /manager ./cmd/controller

# Final minimal runtime matching target platform
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata helm kubectl
ENV HOME=/tmp
WORKDIR /
COPY --from=go-builder /manager /manager
COPY --from=ui-builder /app/ui/dist /ui/dist

USER 65532:65532
ENTRYPOINT ["/manager"]
