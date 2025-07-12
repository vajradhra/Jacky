# 文件监听功能说明

## 概述

新增的SHA256文件监听功能提供了智能的文件变化检测和自动重建能力，大大提高了开发效率。

## 核心特性

### 1. SHA256哈希值比较
- 使用SHA256算法计算文件哈希值
- 只有当文件内容真正发生变化时才触发重建
- 避免因文件时间戳变化但内容未变而导致的无效重建

### 2. 防抖机制
- 500ms防抖延迟，避免频繁重建
- 在短时间内多次文件变化时，只执行最后一次重建
- 减少系统资源消耗

### 3. 智能监听
- 自动监听关键目录：`.`, `_posts`, `_layouts`, `_includes`, `_data`
- 跳过隐藏文件（以`.`开头）
- 跳过输出目录（`_site`）
- 支持文件创建、修改、删除、重命名事件

### 4. 详细日志
- 显示文件变化类型和路径
- 显示重建耗时
- 错误信息详细记录

## 使用方法

### 基本监听模式
```bash
./jacky.exe --watch
```

### 同时启动服务器和监听
```bash
./jacky.exe --serve --watch
```

### 自定义端口和主机
```bash
./jacky.exe --serve --watch --port 8080 --host 0.0.0.0
```

## 技术实现

### FileWatcher结构体
```go
type FileWatcher struct {
    watcher    *fsnotify.Watcher
    site       *Site
    config     *Config
    fileHashes map[string]string // 文件路径 -> SHA256哈希值
    mu         sync.RWMutex
    debounce   time.Duration
    timer      *time.Timer
    rebuildCh  chan struct{}
}
```

### 主要方法

#### `NewFileWatcher(site *Site, cfg *Config) (*FileWatcher, error)`
创建新的文件监听器实例，初始化文件哈希值。

#### `calculateFileHash(filePath string) (string, error)`
计算单个文件的SHA256哈希值。

#### `hasFileChanged(filePath string) (bool, error)`
检查文件是否发生变化，通过比较哈希值判断。

#### `handleFileEvent(event fsnotify.Event)`
处理文件系统事件，根据事件类型执行相应操作。

#### `debouncedRebuild()`
防抖重建，避免频繁触发重建操作。

## 性能优化

### 1. 并发安全
- 使用读写锁保护哈希值映射
- 重建操作在独立协程中执行
- 避免阻塞文件监听主循环

### 2. 内存优化
- 及时清理已删除文件的哈希值记录
- 使用通道进行协程间通信
- 避免内存泄漏

### 3. 文件I/O优化
- 只在必要时计算文件哈希值
- 使用缓冲I/O读取文件
- 跳过不必要的文件类型

## 配置选项

### 防抖时间
默认防抖时间为500ms，可以通过修改`debounce`字段调整：

```go
fw.debounce = 1 * time.Second // 设置为1秒
```

### 监听目录
默认监听目录可以通过修改`initializeFileHashes`方法中的`dirs`切片来调整：

```go
dirs := []string{".", "_posts", "_layouts", "_includes", "_data", "custom_dir"}
```

## 故障排除

### 常见问题

1. **文件变化未检测到**
   - 检查文件是否在监听目录中
   - 确认文件不是隐藏文件
   - 验证文件路径不包含`_site`

2. **重建过于频繁**
   - 增加防抖时间
   - 检查是否有编辑器自动保存功能
   - 确认文件内容确实发生了变化

3. **内存使用过高**
   - 检查是否有大量文件被监听
   - 确认哈希值映射被正确清理
   - 监控文件监听器是否正确关闭

### 调试模式

可以通过修改日志输出来启用更详细的调试信息：

```go
fmt.Printf("[watch] 文件哈希值: %s -> %s\n", filePath, hash)
```

## 扩展功能

### 1. 配置文件监听
可以添加配置文件来支持更灵活的监听设置：

```yaml
watcher:
  debounce: 500ms
  directories:
    - "."
    - "_posts"
    - "_layouts"
  exclude:
    - "*.tmp"
    - ".git/*"
```

### 2. 热重载支持
可以扩展支持模板热重载，无需重启服务器：

```go
// 在handleFileEvent中添加模板重载逻辑
if strings.HasSuffix(event.Name, ".html") {
    fw.reloadTemplate(event.Name)
}
```

### 3. 增量构建
可以实现增量构建，只重建受影响的页面：

```go
// 分析文件依赖关系，只重建相关页面
affectedPages := fw.analyzeDependencies(event.Name)
fw.rebuildPages(affectedPages)
```

## 总结

新的SHA256文件监听功能提供了高效、智能的文件变化检测机制，通过哈希值比较和防抖机制，避免了不必要的重建操作，显著提升了开发体验和系统性能。 