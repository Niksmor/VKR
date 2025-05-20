FROM golang:1.20-alpine as builder

RUN apk add git

ARG IMPARKING_GIT_USER=nikita.morozov
ARG IMPARKING_GIT_PASS=Nn22122003

RUN echo "" > ~/.gitconfig
RUN git config --add --global url."https://${IMPARKING_GIT_USER}:${IMPARKING_GIT_PASS}@git.intermb.ru".insteadOf  git://git.intermb.ru
RUN echo "machine git.intermb.ru login ${IMPARKING_GIT_USER} password ${IMPARKING_GIT_PASS}" > ~/.netrc && chmod 600 ~/.netrc

ENV GOPATH=/go
ENV PATH=${PATH}:${GOPATH}/bin

#RUN go install go.uber.org/nilaway/cmd/nilaway@latest

WORKDIR /code

COPY go.mod .
COPY go.sum .

ENV GOPRIVATE=git.intermb.ru

RUN go mod download

COPY . .

#RUN nilaway -exclude-pkgs "google.golang.org,go.opentelemetry.io" -exclude-file-docstrings "Code generated" ./...

# `skaffold debug` sets SKAFFOLD_GO_GCFLAGS to disable compiler optimizations
ARG SKAFFOLD_GO_GCFLAGS
ARG CA_CERT
ARG CL_CERT
ARG CL_KEY

RUN REPO="git.intermb.ru/im/parking/imparkingplatform/observability/license_helpers.git/variables" && \
    FLAGS="${FLAGS} -X '${REPO}.CA_CERT=${CA_CERT}'" && \
    FLAGS="${FLAGS} -X '${REPO}.CL_CERT=${CL_CERT}'" && \
    FLAGS="${FLAGS} -X '${REPO}.CL_KEY=${CL_KEY}'" && \
    GOOS=linux go build -gcflags="${SKAFFOLD_GO_GCFLAGS}" -ldflags="-s -w ${FLAGS}" -a -o /app .

FROM alpine:3
# Define GOTRACEBACK to mark this container as using the Go language runtime
# for `skaffold debug` (https://skaffold.dev/docs/workflows/debug/).
ENV GOTRACEBACK=single

RUN apk add --no-cache tzdata ca-certificates
RUN update-ca-certificates
RUN cp /usr/share/zoneinfo/Etc/UTC /etc/localtime
RUN echo "Etc/UTC" >  /etc/timezone
RUN echo "hosts: files dns" > /etc/nsswitch.conf

COPY --from=builder /app .

ENTRYPOINT ["./app"]
