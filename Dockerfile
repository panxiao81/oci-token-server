FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/oci-token-server .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/oci-token-server /oci-token-server
EXPOSE 8080
ENTRYPOINT ["/oci-token-server"]
