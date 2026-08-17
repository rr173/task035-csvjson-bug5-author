FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm AS build
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=local \
    GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn
RUN go build -mod=vendor -o /out/csvjson .

FROM docker.m.daocloud.io/library/alpine:3.20
COPY --from=build /out/csvjson /usr/local/bin/csvjson
ENTRYPOINT ["csvjson"]
