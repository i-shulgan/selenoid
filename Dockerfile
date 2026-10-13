FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG GIT_REVISION=HEAD
ARG BUILD_STAMP=unknown
# Optional features: "s3" (upload videos and logs to S3), "metadata" (save
# session metadata next to logs), e.g. --build-arg BUILD_TAGS="s3 metadata".
ARG BUILD_TAGS=

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags "${BUILD_TAGS}" \
    -ldflags "-s -w -X main.gitRevision=${GIT_REVISION} -X main.buildStamp=${BUILD_STAMP}" \
    -o /out/selenoid .

FROM alpine:3

# mailcap provides MIME types for videos and logs served by Selenoid.
RUN apk add --no-cache ca-certificates tzdata mailcap
COPY --from=build /out/selenoid /usr/bin/selenoid

# Runs as root on purpose: Selenoid talks to /var/run/docker.sock, which is
# usually owned by root:docker with a host-specific group ID.
EXPOSE 4444
ENTRYPOINT ["/usr/bin/selenoid", "-listen", ":4444", "-conf", "/etc/selenoid/browsers.json", "-video-output-dir", "/opt/selenoid/video/"]
