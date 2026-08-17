# task035-csvjson

这是一个纯 Go 的 CSV/JSON 双向转换与校验 HTTP 服务。它提供 CSV→JSON、JSON→CSV 和健康检查接口，支持 RFC 4180 引用规则、单元格标量类型推断、严格字段数校验以及进程内自检；不依赖数据库、网络或其他外部服务。

## 标准命令

在 `env/` 目录执行：

```bash
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local go vet ./...
GOTOOLCHAIN=local go run . --smoke-test
GOTOOLCHAIN=local go run . server --addr :8080
```

`--smoke-test` 会启动进程内 HTTP 服务并完成健康检查、转换、校验和往返测试后退出；服务器模式默认监听 `:8080`。

## Benzhi 容器

`build_benzhi_docker.sh` 使用 `benzhi.Dockerfile` 构建评测镜像，参数依次是镜像名和平台，默认值为 `my-project` 与 `linux/amd64`。例如：

```bash
bash build_benzhi_docker.sh csvjson-benzhi linux/amd64
docker run --rm -it csvjson-benzhi:latest
```

容器启动后进入 shell；构建阶段执行 `go build ./...`，不访问外部业务服务。
