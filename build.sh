#!/bin/bash

echo "正在构建 Jacky..."

# 检查Go是否安装
if ! command -v go &> /dev/null; then
    echo "错误: 未找到Go，请先安装Go 1.21或更高版本"
    exit 1
fi

# 安装依赖
echo "安装依赖..."
go mod tidy

# 构建项目
echo "构建项目..."
go build -o jacky.exe main.go

if [ $? -ne 0 ]; then
    echo "构建失败！"
    exit 1
fi

echo "构建成功！生成的文件: jacky"

# 测试构建
echo ""
echo "测试构建示例站点..."
./jacky.exe -verbose

if [ $? -ne 0 ]; then
    echo "测试失败！"
    exit 1
fi
