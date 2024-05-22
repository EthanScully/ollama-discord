FROM --platform=$BUILDPLATFORM golang:latest AS build
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG CGO_ENABLED=0
WORKDIR /build/
COPY . /build/
RUN GOOS=$(echo $TARGETPLATFORM | cut -d'/' -f1) GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f2)	go build -ldflags="-s -w" -o exec
FROM scratch
COPY --from=build /build/exec /bin/ollama-discord
COPY --from=build /etc/ssl/ /etc/ssl/
ENTRYPOINT ["ollama-discord"]
