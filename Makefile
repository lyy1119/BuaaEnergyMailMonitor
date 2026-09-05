# BuaaEnergyMailMonitor —— 构建脚本
#
# 常用用法:
#   make            # 编译（等价于 make build）
#   make build      # 编译出可执行文件 buaaenergy
#   make run        # 编译并运行（可用 ARGS 传参，见下方示例）
#   make fmt        # 格式化源码
#   make vet        # 静态检查
#   make clean      # 删除编译产物
#
# 示例（-to 放末尾，可多个收件人）:
#   make run ARGS="00001 一号电表 50 00002 二号电表 40 -to alert@example.com ops@example.com"

GO     ?= go
BINARY := buaaenergy

.PHONY: all build run fmt vet clean

all: build

# 编译当前目录下的包，产物为 ./buaaenergy
build:
	$(GO) build -o $(BINARY) .

# 编译后运行；未给 ARGS 时程序会打印用法
run: build
	./$(BINARY) $(ARGS)

# 格式化全部源码文件
fmt:
	gofmt -w main.go config.go http.go mail.go

# 静态检查
vet:
	$(GO) vet .

# 清理编译产物
clean:
	rm -f $(BINARY)
