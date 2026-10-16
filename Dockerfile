FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/oci-token-server .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/oci-token-server /oci-token-server
EXPOSE 8080
ENTRYPOINT ["/oci-token-server"]
