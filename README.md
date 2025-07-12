# Jacky

一个用Go语言重写的Jekyll静态网站生成器。

## 功能特性

- ✅ Markdown文件处理
- ✅ 前置数据（Front Matter）解析
- ✅ 布局模板系统
- ✅ 文章和页面管理
- ✅ 永久链接配置
- ✅ 数据文件支持
- ✅ 配置管理
- 🔄 文件监听（开发中）
- 🔄 本地服务器（开发中）
- 🔄 插件系统（计划中）

## 安装

确保已安装Go 1.21或更高版本：

```bash
# 克隆项目
git clone https://github.com/jacky-c/jacky.git
cd jacky

# 安装依赖
go mod tidy

# 编译
go build -o jacky main.go
```

## 使用方法

### 基本命令

```bash
# 构建站点
./jacky

# 指定源目录和目标目录
./jacky -source ./my-site -destination ./output

# 指定配置文件
./jacky -config ./custom_config.yml

# 详细输出
./jacky -verbose

# 静默模式
./jacky -quiet
```

### 项目结构

```
my-site/
├── _config.yml          # 配置文件
├── _layouts/            # 布局模板
│   ├── default.html
│   └── post.html
├── _posts/              # 文章目录
│   ├── 2024-01-01-hello-world.md
│   └── 2024-01-02-second-post.md
├── _data/               # 数据文件
│   └── site.yml
├── about.md             # 页面文件
├── contact.md
└── index.md
```

### 文章格式

文章文件名格式：`YYYY-MM-DD-title.md`

```markdown
---
title: "我的第一篇文章"
layout: post
date: 2024-01-01
categories: [技术, Go]
tags: [jekyll, go, 静态网站]
---

这里是文章内容...

## 二级标题

更多内容...
```

### 布局模板

布局文件使用Go的HTML模板语法：

```html
<!DOCTYPE html>
<html>
<head>
    <title>{{.page.Title}} - {{.site.Title}}</title>
</head>
<body>
    <header>
        <h1>{{.site.Title}}</h1>
    </header>
    
    <main>
        {{.content}}
    </main>
    
    <footer>
        <p>&copy; {{.site.Author}}</p>
    </footer>
</body>
</html>
```

### 配置文件

`_config.yml` 示例：

```yaml
# 站点信息
title: "我的网站"
description: "网站描述"
author: "作者名"
url: "http://localhost:4000"

# 目录配置
source: "."
destination: "_site"
layouts_dir: "_layouts"
posts_dir: "_posts"

# 永久链接格式
permalink: "date"  # 可选: date, pretty, none

# 服务器配置
port: 4000
host: "127.0.0.1"
```

## 与Jekyll的差异

### 相似之处

- 相同的项目结构
- 相同的前置数据格式
- 相同的配置文件格式
- 相同的永久链接配置

### 主要差异

1. **模板引擎**: 使用Go的HTML模板而不是Liquid
2. **Markdown处理器**: 使用Goldmark而不是Kramdown
3. **性能**: Go版本通常更快
4. **部署**: 编译为单个二进制文件，无需Ruby环境

### 模板语法对比

| Jekyll (Liquid) | Jacky (Go Template) |
|----------------|------------------------|
| `{{ page.title }}` | `{{.page.Title}}` |
| `{{ site.title }}` | `{{.site.Title}}` |
| `{{ content }}` | `{{.content}}` |
| `{% if page.title %}` | `{{if .page.Title}}` |
| `{% for post in site.posts %}` | `{{range .site.Posts}}` |

## 开发计划

- [ ] 文件监听功能
- [ ] 本地开发服务器
- [ ] 插件系统
- [ ] 更多模板函数
- [ ] 分页功能
- [ ] 标签和分类页面
- [ ] RSS/Atom feed
- [ ] 搜索功能

## 贡献

欢迎提交Issue和Pull Request！

## 许可证

MIT License 