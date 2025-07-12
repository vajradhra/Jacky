# Jacky 项目总结

## 项目概述

Jacky 是用 Go 语言重写的 Jekyll 静态网站生成器。该项目保持了与原始 Jekyll 相同的项目结构和配置格式，但提供了更好的性能和更简单的部署方式。

## 核心功能实现

### ✅ 已完成功能

1. **配置管理** (`internal/config/`)
   - YAML 配置文件解析
   - 默认配置管理
   - 配置验证

2. **前置数据解析** (`internal/frontmatter/`)
   - YAML 格式前置数据解析
   - 正文内容提取

3. **Markdown 转换** (`internal/converter/`)
   - 使用 Goldmark 处理器
   - 支持 GFM、表格、任务列表等扩展

4. **页面管理** (`internal/page/`)
   - 页面文件处理
   - 前置数据提取
   - URL 生成

5. **文章管理** (`internal/post/`)
   - 文章文件处理（支持 YYYY-MM-DD-title.md 格式）
   - 多种永久链接格式支持
   - 分类和标签处理

6. **模板引擎** (`internal/template/`)
   - Go HTML 模板支持
   - 基本模板函数
   - 布局系统

7. **站点构建** (`internal/site/`)
   - 完整的构建流程
   - 文件读取和写入
   - 内容渲染

8. **命令行接口** (`main.go`)
   - 命令行参数解析
   - 基本命令支持

### 🔄 开发中功能

1. **文件监听**
   - 实时文件变化检测
   - 增量构建

2. **本地服务器**
   - HTTP 服务器
   - 静态文件服务

### 📋 计划功能

1. **插件系统**
   - 自定义转换器
   - 自定义生成器
   - 自定义标签

2. **高级功能**
   - 分页
   - 标签和分类页面
   - RSS/Atom feed
   - 搜索功能

## 项目结构

```
jacky/
├── main.go                    # 主程序入口
├── go.mod                     # Go模块文件
├── _config.yml                # 项目配置
├── README.md                  # 项目说明
├── PROJECT_SUMMARY.md         # 项目总结
├── Makefile                   # 构建脚本
├── build.bat                  # Windows构建脚本
├── build.sh                   # Linux/macOS构建脚本
├── example/                   # 示例站点
│   ├── _config.yml
│   ├── _layouts/
│   ├── _posts/
│   └── index.md
└── internal/                  # 内部包
    ├── config/                # 配置管理
    ├── converter/             # Markdown转换
    ├── frontmatter/           # 前置数据解析
    ├── page/                  # 页面管理
    ├── post/                  # 文章管理
    ├── site/                  # 站点管理
    └── template/              # 模板引擎
```

## 技术栈

- **语言**: Go 1.21+
- **Markdown处理器**: Goldmark
- **YAML解析**: gopkg.in/yaml.v3
- **模板引擎**: Go HTML templates
- **构建工具**: Make, Go modules

## 与原始Jekyll的对比

### 相似之处

- 相同的项目结构（`_layouts/`, `_posts/`, `_data/` 等）
- 相同的前置数据格式（YAML + `---` 分隔符）
- 相同的配置文件格式（`_config.yml`）
- 相同的永久链接配置选项

### 主要差异

| 特性 | Jekyll (Ruby) | Jacky |
|------|---------------|-----------|
| 语言 | Ruby | Go |
| 模板引擎 | Liquid | Go HTML templates |
| Markdown处理器 | Kramdown | Goldmark |
| 部署方式 | 需要Ruby环境 | 单个二进制文件 |
| 性能 | 较慢 | 更快 |
| 内存使用 | 较高 | 较低 |
| 跨平台 | 需要Ruby | 原生支持 |

### 模板语法对比

| Jekyll (Liquid) | Jacky (Go Template) |
|----------------|------------------------|
| `{{ page.title }}` | `{{.page.Title}}` |
| `{{ site.title }}` | `{{.site.Title}}` |
| `{{ content }}` | `{{.content}}` |
| `{% if page.title %}` | `{{if .page.Title}}` |
| `{% for post in site.posts %}` | `{{range .site.Posts}}` |

## 构建和测试

### 构建命令

```bash
# 基本构建
make build

# 构建所有平台
make build-all

# 构建示例站点
make example

# 运行测试
make test

# 清理构建文件
make clean
```

### 测试

项目包含基本的单元测试：

```bash
go test ./...
```

## 使用示例

### 基本使用

```bash
# 构建站点
./jacky

# 指定源目录和目标目录
./jacky -source ./my-site -destination ./output

# 详细输出
./jacky -verbose
```

### 项目结构示例

```
my-site/
├── _config.yml
├── _layouts/
│   └── default.html
├── _posts/
│   └── 2024-01-01-hello-world.md
├── _data/
│   └── site.yml
└── index.md
```

### 文章格式示例

```markdown
---
title: "我的文章"
layout: default
date: 2024-01-01
categories: [技术, Go]
tags: [jekyll, go]
---

文章内容...
```

## 性能优势

1. **编译型语言**: Go的编译特性带来更好的性能
2. **并发处理**: 可以轻松实现并发处理多个文件
3. **内存效率**: Go的垃圾回收和内存管理更高效
4. **启动速度**: 编译后的二进制文件启动更快

## 部署优势

1. **单文件部署**: 编译为单个二进制文件
2. **无依赖**: 不需要安装Ruby或其他运行时
3. **跨平台**: 支持Windows、macOS、Linux
4. **容器友好**: 适合Docker容器化部署

## 未来发展方向

1. **完善功能**: 实现文件监听和本地服务器
2. **插件系统**: 支持自定义插件
3. **性能优化**: 进一步优化构建速度
4. **社区建设**: 建立用户社区和文档
5. **生态系统**: 开发相关工具和插件

## 总结

Jacky 成功地将 Jekyll 的核心功能用 Go 语言重新实现，保持了与原始 Jekyll 的兼容性，同时提供了更好的性能和更简单的部署方式。虽然目前功能还不够完整，但已经具备了基本的静态网站生成能力，为后续的功能扩展奠定了良好的基础。

该项目展示了如何使用 Go 语言重写现有的工具，并充分利用 Go 语言的特性来提升性能和简化部署。 