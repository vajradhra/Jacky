package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/emirpasic/gods/utils"
	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/yanyiwu/gojieba"
)

// ==================== 配置管理 ====================

// Config 表示Jekyll配置
type Config struct {
	// 目录配置
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	CacheDir    string `yaml:"cache_dir"`
	Root        string `yaml:"root"`
	Plugins     string `yaml:"plugins"`
	CodeDir     string `yaml:"code_dir"`
	CategoryDir string `yaml:"category_dir"`

	// 目录结构
	LayoutsDir  string `yaml:"layouts_dir"`
	DataDir     string `yaml:"data_dir"`
	IncludesDir string `yaml:"includes_dir"`
	PostsDir    string `yaml:"posts_dir"`

	// 内容处理
	MarkdownExt      string `yaml:"markdown_ext"`
	Permalink        string `yaml:"permalink"`
	Paginate         int    `yaml:"paginate"`
	PaginatePath     string `yaml:"paginate_path"`
	ExcerptSeparator string `yaml:"excerpt_separator"`
	RecentPosts      int    `yaml:"recent_posts"`
	ExcerptLink      string `yaml:"excerpt_link"`
	Titlecase        bool   `yaml:"titlecase"`

	// 服务器配置
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
	BaseURL string `yaml:"baseurl"`

	// 其他配置
	Title       string                 `yaml:"title"`
	Subtitle    string                 `yaml:"subtitle"`
	Description string                 `yaml:"description"`
	Author      string                 `yaml:"author"`
	URL         string                 `yaml:"url"`
	Data        map[string]interface{} `yaml:"data"`

	// 侧边栏、社交、评论等扩展字段
	DefaultAsides             []string `yaml:"default_asides"`
	GithubUser                string   `yaml:"github_user"`
	GithubRepoCount           int      `yaml:"github_repo_count"`
	TwitterUser               string   `yaml:"twitter_user"`
	DisqusShortName           string   `yaml:"disqus_short_name"`
	GoogleAnalyticsTrackingID string   `yaml:"google_analytics_tracking_id"`
	// ...可继续扩展

	// 内部使用
	configPath string
}

// Defaults 返回默认配置
func Defaults() *Config {
	return &Config{
		Source:      ".",
		Destination: "_site",
		CacheDir:    ".jekyll-cache",
		LayoutsDir:  "_layouts",
		DataDir:     "_data",
		IncludesDir: "_includes",
		PostsDir:    "_posts",
		MarkdownExt: "markdown,mkdown,mkdn,mkd,md",
		Permalink:   "date",
		Port:        4000,
		Host:        "127.0.0.1",
		Title:       "Octopress 文档",
		Description: "Octopress 静态博客框架文档",
		Author:      "Octopress",
		URL:         "http://localhost:4000",
		Data:        make(map[string]interface{}),
	}
}

// Load 加载配置文件
func Load(configPath, source, destination string) (*Config, error) {
	config := Defaults()

	// 设置命令行参数
	if source != "." {
		config.Source = source
	}
	if destination != "_site" {
		config.Destination = destination
	}

	// 尝试加载配置文件
	if configPath != "" {
		config.configPath = configPath
		if err := config.loadFromFile(); err != nil {
			return nil, fmt.Errorf("加载配置文件失败: %w", err)
		}
	}

	// 验证配置
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return config, nil
}

// loadFromFile 从文件加载配置
func (c *Config) loadFromFile() error {
	// 尝试多个配置文件扩展名
	extensions := []string{".yml", ".yaml", ".toml"}
	configFile := ""

	for _, ext := range extensions {
		path := filepath.Join(c.Source, "_config"+ext)
		if _, err := os.Stat(path); err == nil {
			configFile = path
			break
		}
	}

	if configFile == "" {
		// 使用指定的配置文件路径
		if c.configPath != "" {
			configFile = c.configPath
		} else {
			return nil // 没有配置文件，使用默认值
		}
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取配置文件 %s 失败: %w", configFile, err)
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	return nil
}

// validate 验证配置
func (c *Config) validate() error {
	// 检查源目录是否存在
	if _, err := os.Stat(c.Source); os.IsNotExist(err) {
		return fmt.Errorf("源目录不存在: %s", c.Source)
	}

	// 确保目标目录不是源目录的子目录（但允许_site目录）
	absSource, _ := filepath.Abs(c.Source)
	absDest, _ := filepath.Abs(c.Destination)

	if strings.HasPrefix(absDest, absSource) && absSource != absDest {
		// 检查是否是_site目录
		destBase := filepath.Base(absDest)
		if destBase != "_site" {
			return fmt.Errorf("目标目录不能是源目录的子目录")
		}
	}

	return nil
}

// GetMarkdownExtensions 获取Markdown文件扩展名列表
func (c *Config) GetMarkdownExtensions() []string {
	exts := strings.Split(c.MarkdownExt, ",")
	for i, ext := range exts {
		exts[i] = strings.TrimSpace(ext)
	}
	return exts
}

// IsMarkdownFile 检查文件是否为Markdown文件
func (c *Config) IsMarkdownFile(filename string) bool {
	ext := filepath.Ext(filename)
	if ext == "" {
		return false
	}

	ext = strings.ToLower(ext[1:]) // 去掉点号
	extensions := c.GetMarkdownExtensions()

	for _, validExt := range extensions {
		if ext == validExt {
			return true
		}
	}
	return false
}

// ==================== 前置数据解析 ====================

// Parse 解析前置数据
func Parse(content string) (map[string]interface{}, string, error) {
	// 检查内容是否为空
	if strings.TrimSpace(content) == "" {
		return make(map[string]interface{}), "", nil
	}

	lines := strings.Split(content, "\n")

	// 查找前置数据开始和结束位置
	start := -1
	end := -1

	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if start == -1 {
				start = i
			} else {
				end = i
				break
			}
		}
	}

	// 如果没有找到前置数据
	if start == -1 || end == -1 {
		return make(map[string]interface{}), content, nil
	}

	// 检查前置数据范围是否有效
	if start >= end || start >= len(lines) || end >= len(lines) {
		log.Printf("警告: 前置数据范围无效，跳过前置数据解析")
		return make(map[string]interface{}), content, nil
	}

	// 提取前置数据
	frontMatterLines := lines[start+1 : end]
	frontMatterContent := strings.Join(frontMatterLines, "\n")

	// 解析YAML
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontMatterContent), &data); err != nil {
		log.Printf("警告: YAML解析失败，使用默认前置数据: %v", err)
		// 返回默认数据而不是错误，避免程序崩溃
		data = make(map[string]interface{})
	}

	// 提取正文内容
	bodyLines := lines[end+1:]
	bodyContent := strings.Join(bodyLines, "\n")

	return data, bodyContent, nil
}

// ==================== Markdown转换器 ====================

// MarkdownConverter 表示Markdown转换器
type MarkdownConverter struct {
	config *Config
	md     goldmark.Markdown
}

// NewConverter 创建新的Markdown转换器
func NewConverter(cfg *Config) *MarkdownConverter {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.DefinitionList,
			extension.Footnote,
			extension.Linkify,
			extension.Strikethrough,
			extension.Table,
			extension.TaskList,
			extension.Typographer,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(), // 允许原始HTML，不转义HTML实体
		),
	)

	return &MarkdownConverter{
		config: cfg,
		md:     md,
	}
}

// Convert 转换Markdown内容为HTML
func (c *MarkdownConverter) Convert(content string) (string, error) {
	// 检查内容是否为空
	if strings.TrimSpace(content) == "" {
		return "", nil
	}

	// 预处理内容，处理常见的Markdown格式问题
	content = c.preprocessMarkdown(content)

	var buf bytes.Buffer
	if err := c.md.Convert([]byte(content), &buf); err != nil {
		// 如果转换失败，返回原始内容并记录错误
		log.Printf("Markdown转换失败，使用原始内容: %v", err)
		return template.HTMLEscapeString(content), nil
	}

	result := buf.String()

	// 后处理，确保HTML安全
	result = c.postprocessHTML(result)

	return result, nil
}

// preprocessMarkdown 预处理Markdown内容
func (c *MarkdownConverter) preprocessMarkdown(content string) string {
	// 处理空行
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// 确保代码块正确闭合
	lines := strings.Split(content, "\n")
	var processedLines []string
	inCodeBlock := false

	for _, line := range lines {
		// 检查代码块开始
		if strings.HasPrefix(line, "```") && !inCodeBlock {
			inCodeBlock = true
			processedLines = append(processedLines, line)
			continue
		}

		// 检查代码块结束
		if strings.HasPrefix(line, "```") && inCodeBlock {
			inCodeBlock = false
			processedLines = append(processedLines, line)
			continue
		}

		// 在代码块内，保持原样
		if inCodeBlock {
			processedLines = append(processedLines, line)
			continue
		}

		// 处理标题行
		if strings.HasPrefix(line, "#") {
			// 确保标题后有空格
			if !strings.Contains(line, " ") {
				line = strings.Replace(line, "#", "# ", 1)
			}
		}

		// 处理列表
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
			// 确保列表项格式正确
			if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") && !strings.HasPrefix(line, "+ ") {
				line = "- " + strings.TrimSpace(line)
			}
		}

		// 处理链接
		if strings.Contains(line, "[") && strings.Contains(line, "]") && strings.Contains(line, "(") && strings.Contains(line, ")") {
			// 确保链接格式正确
			line = c.fixMarkdownLinks(line)
		}

		processedLines = append(processedLines, line)
	}

	// 如果代码块没有正确闭合，添加结束标记
	if inCodeBlock {
		processedLines = append(processedLines, "```")
	}

	return strings.Join(processedLines, "\n")
}

// fixMarkdownLinks 修复Markdown链接格式
func (c *MarkdownConverter) fixMarkdownLinks(line string) string {
	// 简单的链接修复逻辑
	// 这里可以添加更复杂的链接修复规则
	return line
}

// postprocessHTML 后处理HTML内容
func (c *MarkdownConverter) postprocessHTML(html string) string {
	// 移除潜在的XSS攻击向量
	html = strings.ReplaceAll(html, "<script", "&lt;script")
	html = strings.ReplaceAll(html, "</script>", "&lt;/script&gt;")
	html = strings.ReplaceAll(html, "javascript:", "")

	// 确保图片标签有alt属性
	html = c.fixImageTags(html)

	return html
}

// fixImageTags 修复图片标签
func (c *MarkdownConverter) fixImageTags(html string) string {
	// 简单的图片标签修复
	// 这里可以添加更复杂的图片标签处理逻辑
	return html
}

// ValidateMarkdown 验证Markdown格式
func ValidateMarkdown(content string) (bool, []string) {
	var errors []string

	// 检查基本格式
	if strings.TrimSpace(content) == "" {
		errors = append(errors, "内容为空")
		return false, errors
	}

	lines := strings.Split(content, "\n")

	// 检查代码块是否配对
	inCodeBlock := false
	inFrontMatter := false
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// 检查是否进入或退出前置数据块
		if trimmedLine == "---" {
			inFrontMatter = !inFrontMatter
			continue
		}

		// 如果在前置数据块内，跳过格式检查
		if inFrontMatter {
			continue
		}

		if strings.HasPrefix(trimmedLine, "```") {
			if inCodeBlock {
				inCodeBlock = false
			} else {
				inCodeBlock = true
			}
		}

		// 检查标题格式
		if strings.HasPrefix(line, "#") {
			if !strings.Contains(line, " ") {
				errors = append(errors, fmt.Sprintf("第%d行: 标题格式错误，应在#后加空格", i+1))
			}
		}

		// 检查列表格式
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "+") {
			if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") && !strings.HasPrefix(line, "+ ") {
				errors = append(errors, fmt.Sprintf("第%d行: 列表格式错误，应在符号后加空格", i+1))
			}
		}
	}

	// 检查代码块是否闭合
	if inCodeBlock {
		errors = append(errors, "代码块未正确闭合")
	}

	return len(errors) == 0, errors
}

// ==================== 页面管理 ====================

// Page 表示一个页面
type Page struct {
	Path             string
	Title            string
	Layout           string
	Content          string
	RenderedContent  string
	FrontMatter      map[string]interface{}
	URL              string
	Date             time.Time
	Description      string
	Excerpt          string
	ExcerptSeparator string
}

// NewPage 创建新的页面
func NewPage(path string, cfg *Config) (*Page, error) {
	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	// 解析前置数据
	fm, body, err := Parse(string(content))
	if err != nil {
		log.Printf("警告: 解析前置数据失败，使用默认值: %v", err)
		fm = make(map[string]interface{})
	}

	// 创建页面对象
	page := &Page{
		Path:             path,
		Content:          body,
		FrontMatter:      fm,
		ExcerptSeparator: "\n\n",
	}

	// 从前置数据中提取信息
	if err := page.extractFrontMatter(); err != nil {
		log.Printf("警告: 提取前置数据失败，使用默认值: %v", err)
		// 设置默认值
		page.Title = filepath.Base(path)
		page.Layout = "default"
		page.Date = time.Now()
	}

	// 生成URL
	page.generateURL(cfg)

	// 生成摘要
	page.generateExcerpt()

	return page, nil
}

// extractFrontMatter 从前置数据中提取信息
func (p *Page) extractFrontMatter() error {
	// 标题
	if title, ok := p.FrontMatter["title"].(string); ok {
		p.Title = title
	} else {
		// 如果没有标题，使用文件名
		p.Title = strings.TrimSuffix(filepath.Base(p.Path), filepath.Ext(p.Path))
	}

	// 布局
	if layout, ok := p.FrontMatter["layout"].(string); ok {
		p.Layout = layout
	}

	// 日期
	if date, ok := p.FrontMatter["date"].(time.Time); ok {
		p.Date = date
	} else if dateStr, ok := p.FrontMatter["date"].(string); ok {
		// 尝试解析日期字符串
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			p.Date = parsed
		}
	} else {
		// 使用文件修改时间
		if info, err := os.Stat(p.Path); err == nil {
			p.Date = info.ModTime()
		}
	}

	// 描述
	if description, ok := p.FrontMatter["description"].(string); ok {
		p.Description = description
	}

	// 摘要分隔符
	if separator, ok := p.FrontMatter["excerpt_separator"].(string); ok {
		p.ExcerptSeparator = separator
	}

	return nil
}

// generateURL 生成页面URL
func (p *Page) generateURL(cfg *Config) {
	// 计算相对路径
	relPath, _ := filepath.Rel(cfg.Source, p.Path)

	// 移除扩展名
	relPath = strings.TrimSuffix(relPath, filepath.Ext(relPath))

	// 转换为URL格式
	p.URL = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

	// 添加.html扩展名
	p.URL += ".html"
}

// generateExcerpt 生成摘要
func (p *Page) generateExcerpt() {
	if p.ExcerptSeparator == "" {
		return
	}

	parts := strings.Split(p.Content, p.ExcerptSeparator)
	if len(parts) > 0 {
		p.Excerpt = strings.TrimSpace(parts[0])
	}
}

// ==================== 文章管理 ====================

// Post 表示一篇文章
type Post struct {
	Path             string
	Title            string
	Layout           string
	Content          string
	RenderedContent  string
	FrontMatter      map[string]interface{}
	URL              string
	Date             time.Time
	Description      string
	Excerpt          string
	ExcerptSeparator string
	Slug             string
	Permalink        string
}

// 文件名格式正则表达式: YYYY-MM-DD-title.md
var filenameRegex = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})-(.+)\.(.+)$`)

// NewPost 创建新的文章
func NewPost(path string, cfg *Config) (*Post, error) {
	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	// 解析前置数据
	fm, body, err := Parse(string(content))
	if err != nil {
		log.Printf("警告: 解析前置数据失败，使用默认值: %v", err)
		fm = make(map[string]interface{})
	}

	// 创建文章对象
	post := &Post{
		Path:             path,
		Content:          body,
		FrontMatter:      fm,
		ExcerptSeparator: "\n\n",
	}

	// 从前置数据中提取信息
	if err := post.extractFrontMatter(); err != nil {
		log.Printf("警告: 提取前置数据失败，使用默认值: %v", err)
		// 设置默认值
		post.Title = filepath.Base(path)
		post.Layout = "post"
		post.Date = time.Now()
	}

	// 从文件名中提取信息
	post.extractFromFilename()

	// 生成URL
	post.generateURL(cfg)

	// 生成摘要
	post.generateExcerpt()

	return post, nil
}

// extractFrontMatter 从前置数据中提取信息
func (p *Post) extractFrontMatter() error {
	// 标题
	if title, ok := p.FrontMatter["title"].(string); ok {
		p.Title = title
	}

	// 布局
	if layout, ok := p.FrontMatter["layout"].(string); ok {
		p.Layout = layout
	}

	// 日期
	if date, ok := p.FrontMatter["date"].(time.Time); ok {
		p.Date = date
	} else if dateStr, ok := p.FrontMatter["date"].(string); ok {
		// 尝试解析日期字符串
		if parsed, err := time.Parse("2006-01-02 15:04:05", dateStr); err == nil {
			p.Date = parsed
		} else if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			p.Date = parsed
		}
	}

	// 摘要分隔符
	if separator, ok := p.FrontMatter["excerpt_separator"].(string); ok {
		p.ExcerptSeparator = separator
	}

	// 永久链接
	if permalink, ok := p.FrontMatter["permalink"].(string); ok {
		p.Permalink = permalink
	}

	return nil
}

// extractFromFilename 从文件名中提取信息
func (p *Post) extractFromFilename() {
	filename := filepath.Base(p.Path)
	matches := filenameRegex.FindStringSubmatch(filename)

	if len(matches) >= 5 {
		// 提取日期
		if p.Date.IsZero() {
			if year, month, day := matches[1], matches[2], matches[3]; year != "" && month != "" && day != "" {
				if date, err := time.Parse("2006-01-02", fmt.Sprintf("%s-%s-%s", year, month, day)); err == nil {
					p.Date = date
				}
			}
		}

		// 提取标题和slug
		titlePart := matches[4]
		// 自动去除 slug 前缀的日期（如 2025-07-12-測試rss和sitemap功能 -> 測試rss和sitemap功能）
		titlePart = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-`).ReplaceAllString(titlePart, "")
		if p.Title == "" {
			p.Title = strings.ReplaceAll(titlePart, "-", " ")
		}
		// 生成slug
		p.Slug = titlePart
	}
}

// generateURL 生成文章URL
func (p *Post) generateURL(cfg *Config) {
	// 如果有自定义永久链接，使用它
	if p.Permalink != "" {
		p.URL = p.Permalink
		if !strings.HasSuffix(p.URL, ".html") {
			p.URL += ".html"
		}
		// 转换为绝对URL
		p.URL = p.makeAbsoluteURL(p.URL, cfg)
		return
	}

	// 根据配置生成URL
	switch cfg.Permalink {
	case "date":
		p.generateDateURL(cfg)
	case "pretty":
		p.generatePrettyURL(cfg)
	case "none":
		p.generateNoneURL(cfg)
	default:
		p.generateDateURL(cfg) // 默认使用日期格式
	}
}

// makeAbsoluteURL 将相对URL转换为绝对URL
func (p *Post) makeAbsoluteURL(relativeURL string, cfg *Config) string {
	// 如果已经是完整的URL，直接返回
	if strings.HasPrefix(relativeURL, "http://") || strings.HasPrefix(relativeURL, "https://") {
		return relativeURL
	}

	// 确保路径以 / 开头
	if !strings.HasPrefix(relativeURL, "/") {
		relativeURL = "/" + relativeURL
	}

	// 组合基础URL和路径
	baseURL := cfg.URL
	if strings.HasSuffix(baseURL, "/") {
		baseURL = baseURL[:len(baseURL)-1] // 移除末尾的 /
	}

	return baseURL + relativeURL
}

// generateDateURL 生成日期格式的URL
func (p *Post) generateDateURL(cfg *Config) {
	var relativeURL string
	if p.Date.IsZero() {
		relativeURL = "/" + p.Slug + ".html"
	} else {
		year := p.Date.Format("2006")
		month := p.Date.Format("01")
		day := p.Date.Format("02")
		// 使用二叉树路径格式：/年/月/日/slug.html
		relativeURL = fmt.Sprintf("/%s/%s/%s/%s.html", year, month, day, p.Slug)
	}

	p.URL = p.makeAbsoluteURL(relativeURL, cfg)
}

// generatePrettyURL 生成美观格式的URL
func (p *Post) generatePrettyURL(cfg *Config) {
	var relativeURL string
	if p.Date.IsZero() {
		relativeURL = "/" + p.Slug + "/index.html"
	} else {
		year := p.Date.Format("2006")
		month := p.Date.Format("01")
		day := p.Date.Format("02")
		// 使用二叉树路径格式：/年/月/日/slug/index.html
		relativeURL = fmt.Sprintf("/%s/%s/%s/%s/index.html", year, month, day, p.Slug)
	}

	p.URL = p.makeAbsoluteURL(relativeURL, cfg)
}

// generateNoneURL 生成无扩展名的URL
func (p *Post) generateNoneURL(cfg *Config) {
	var relativeURL string
	if p.Date.IsZero() {
		relativeURL = "/" + p.Slug + "/index.html"
	} else {
		year := p.Date.Format("2006")
		month := p.Date.Format("01")
		day := p.Date.Format("02")
		// 使用二叉树路径格式：/年/月/日/slug/index.html
		relativeURL = fmt.Sprintf("/%s/%s/%s/%s/index.html", year, month, day, p.Slug)
	}

	p.URL = p.makeAbsoluteURL(relativeURL, cfg)
}

// extractRelativeURL 从绝对URL中提取相对路径
func (p *Post) extractRelativeURL() string {
	// 如果URL不是绝对URL，直接返回
	if !strings.HasPrefix(p.URL, "http://") && !strings.HasPrefix(p.URL, "https://") {
		return p.URL
	}

	// 解析URL
	parsedURL, err := url.Parse(p.URL)
	if err != nil {
		// 如果解析失败，尝试简单处理
		if strings.Contains(p.URL, "://") {
			parts := strings.SplitN(p.URL, "://", 2)
			if len(parts) > 1 {
				path := strings.SplitN(parts[1], "/", 2)
				if len(path) > 1 {
					return "/" + path[1]
				}
			}
		}
		return p.URL
	}

	return parsedURL.Path
}

// generateExcerpt 生成摘要
func (p *Post) generateExcerpt() {
	if p.ExcerptSeparator == "" {
		return
	}

	parts := strings.Split(p.Content, p.ExcerptSeparator)
	if len(parts) > 0 {
		p.Excerpt = strings.TrimSpace(parts[0])
	}
}

// ==================== 模板引擎 ====================

// Layout 表示一个布局模板
type Layout struct {
	Name    string
	Content string
	tmpl    *template.Template
}

// Engine 表示模板引擎
type Engine struct {
	config *Config
	site   *Site // 添加对Site的引用
}

// NewEngine 创建新的模板引擎
func NewEngine(cfg *Config, site *Site) *Engine {
	return &Engine{
		config: cfg,
		site:   site,
	}
}

// NewLayout 创建新的布局
func NewLayout(name, content string) *Layout {
	return &Layout{
		Name:    name,
		Content: content,
	}
}

// initMasterTemplate 初始化主模板
func (s *Site) initMasterTemplate() error {
	// 创建一个新的模板实例，并定义所有函数
	s.MasterTemplate = template.New("main").Funcs(template.FuncMap{
		"include":         s.Template.include,
		"date":            s.Template.date,
		"escape":          s.Template.escape,
		"strip":           s.Template.strip,
		"truncate":        s.Template.truncate,
		"safe":            s.Template.safe,
		"url_path_escape": func(s string) string { return url.PathEscape(s) },
		"add":             func(a, b int) int { return a + b },
		"sub":             func(a, b int) int { return a - b },
		"first":           s.Template.first,
		"mul":             func(a, b int) int { return a * b },
		"join":            s.Template.join,
	})

	// 解析所有布局文件和包含文件
	for name, layout := range s.Layouts {
		// Each layout/include is a named template within the master template set
		_, err := s.MasterTemplate.Parse(fmt.Sprintf("{{define \"%s\"}}%s{{end}}", name, layout.Content))
		if err != nil {
			return fmt.Errorf("解析模板 %s 失败: %w", name, err)
		}
	}
	log.Println("所有布局和包含文件已解析到主模板")
	return nil
}

// Render 渲染模板
func (e *Engine) Render(layout *Layout, data map[string]interface{}) (string, error) {
	// 渲染模板
	var buf bytes.Buffer
	if err := e.site.MasterTemplate.ExecuteTemplate(&buf, layout.Name, data); err != nil {
		return "", fmt.Errorf("渲染模板 %s 失败: %w", layout.Name, err)
	}

	return buf.String(), nil
}

// include 包含文件
func (e *Engine) include(filename string) (template.HTML, error) {
	// 渲染包含文件
	var buf bytes.Buffer
	if err := e.site.MasterTemplate.ExecuteTemplate(&buf, filename, e.site.CurrentData); err != nil {
		return "", fmt.Errorf("渲染包含文件 %s 失败: %w", filename, err)
	}

	return template.HTML(buf.String()), nil
}

// date 格式化日期
func (e *Engine) date(format string, date interface{}) string {
	if d, ok := date.(time.Time); ok {
		return d.Format(format)
	}
	return ""
}

// escape HTML转义
func (e *Engine) escape(s string) template.HTML {
	return template.HTML(template.HTMLEscapeString(s))
}

// strip 去除空白字符
func (e *Engine) strip(s string) string {
	return strings.TrimSpace(s)
}

// truncate 截断字符串
func (e *Engine) truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length] + "..."
}

// safe 返回不转义的HTML内容
func (e *Engine) safe(s string) template.HTML {
	return template.HTML(s)
}

// first 返回切片或数组的前n个元素
func (e *Engine) first(n int, data interface{}) interface{} {
	val := reflect.ValueOf(data)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil
	}
	if n < 0 || n > val.Len() {
		n = val.Len()
	}
	return val.Slice(0, n).Interface()
}

// join 将字符串切片用分隔符连接
func (e *Engine) join(sep string, data interface{}) string {
	val := reflect.ValueOf(data)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return ""
	}

	var parts []string
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i)
		if item.Kind() == reflect.String {
			parts = append(parts, item.String())
		}
	}

	return strings.Join(parts, sep)
}

// ==================== 二叉树URL搜索 ====================

// URLNode 表示URL二叉树节点
type URLNode struct {
	Path     string
	Post     *Post
	Children map[string]*URLNode
}

// URLTree 表示URL二叉树
type URLTree struct {
	Root *URLNode
}

// NewURLTree 创建新的URL二叉树
func NewURLTree() *URLTree {
	return &URLTree{
		Root: &URLNode{
			Path:     "/",
			Children: make(map[string]*URLNode),
		},
	}
}

// Insert 插入URL到二叉树
func (t *URLTree) Insert(path string, post *Post) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	current := t.Root

	for i, part := range parts {
		if part == "" {
			continue
		}

		if current.Children == nil {
			current.Children = make(map[string]*URLNode)
		}

		if _, exists := current.Children[part]; !exists {
			current.Children[part] = &URLNode{
				Path:     part,
				Children: make(map[string]*URLNode),
			}
		}

		current = current.Children[part]

		// 如果是最后一个部分，设置文章
		if i == len(parts)-1 {
			current.Post = post
		}
	}
}

// Search 搜索URL
func (t *URLTree) Search(path string) *Post {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	current := t.Root

	for _, part := range parts {
		if part == "" {
			continue
		}

		if current.Children == nil {
			return nil
		}

		if child, exists := current.Children[part]; exists {
			current = child
		} else {
			return nil
		}
	}

	return current.Post
}

// SearchPrefix 前缀搜索
func (t *URLTree) SearchPrefix(prefix string) []*Post {
	var results []*Post
	parts := strings.Split(strings.Trim(prefix, "/"), "/")
	current := t.Root

	// 导航到前缀节点
	for _, part := range parts {
		if part == "" {
			continue
		}

		if current.Children == nil {
			return results
		}

		if child, exists := current.Children[part]; exists {
			current = child
		} else {
			return results
		}
	}

	// 收集所有子节点的文章
	t.collectPosts(current, &results)
	return results
}

// collectPosts 收集节点及其子节点的所有文章
func (t *URLTree) collectPosts(node *URLNode, results *[]*Post) {
	if node.Post != nil {
		*results = append(*results, node.Post)
	}

	for _, child := range node.Children {
		t.collectPosts(child, results)
	}
}

// ==================== 站点管理 ====================

// Site 表示Jekyll站点
type Site struct {
	Config         *Config
	Pages          []*Page
	Posts          []*Post
	Layouts        map[string]*Layout
	Data           map[string]interface{}
	Converter      *MarkdownConverter
	Template       *Engine
	MasterTemplate *template.Template     // 存储所有解析后的模板
	CurrentData    map[string]interface{} // 用于传递当前渲染数据给include

	mu sync.RWMutex
	// 新增分页和归档结构体
	PagedPosts [][]*Post
	Archives   map[string][]*Post // "2024-07" => []*Post
	JiebaTags  []string           // 高频词标签云
	RouteTree  *treemap.Map       // key: 路由path, value: 主路径
	URLTree    *URLTree           // URL二叉树，用于搜索

	// RSS 和 Sitemap 相关
	RSSFeed    string // RSS feed 内容
	SitemapXML string // Sitemap XML 内容
}

// New 创建新的站点实例
func New(cfg *Config) *Site {
	s := &Site{
		Config:    cfg,
		Pages:     make([]*Page, 0),
		Posts:     make([]*Post, 0),
		Layouts:   make(map[string]*Layout),
		Data:      make(map[string]interface{}),
		Converter: NewConverter(cfg),
		URLTree:   NewURLTree(),
	}
	s.Template = NewEngine(cfg, s) // Pass the site instance to NewEngine
	return s
}

// Build 构建站点
func (s *Site) Build() error {
	log.Println("开始构建站点...")

	// 1. 读取数据文件
	if err := s.loadData(); err != nil {
		return fmt.Errorf("加载数据失败: %w", err)
	}

	// 2. 读取布局文件
	if err := s.loadLayouts(); err != nil {
		return fmt.Errorf("加载布局失败: %w", err)
	}

	// 2.5. 读取包含文件
	if err := s.loadIncludes(); err != nil {
		return fmt.Errorf("加载包含文件失败: %w", err)
	}

	// 3. 读取页面文件
	if err := s.loadPages(); err != nil {
		return fmt.Errorf("加载页面失败: %w", err)
	}

	// 4. 读取文章文件
	if err := s.loadPosts(); err != nil {
		return fmt.Errorf("加载文章失败: %w", err)
	}

	// 4.5. 初始化MasterTemplate
	if err := s.initMasterTemplate(); err != nil {
		return fmt.Errorf("初始化主模板失败: %w", err)
	}

	// 5. 处理分页、归档、标签、分类数据
	s.processCollections()

	// 6. 构建Jieba标签云、路由树和URL二叉树
	s.buildJiebaTags()
	s.buildRouteTree()
	s.buildURLTree()

	// 7. 渲染所有内容
	if err := s.render(); err != nil {
		return fmt.Errorf("渲染失败: %w", err)
	}

	// 7. 写入输出目录
	if err := s.write(); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}

	// 8. 生成 RSS Feed
	if err := s.generateRSSFeed(); err != nil {
		return fmt.Errorf("生成RSS Feed失败: %w", err)
	}

	// 9. 生成 Sitemap
	if err := s.generateSitemap(); err != nil {
		return fmt.Errorf("生成Sitemap失败: %w", err)
	}

	// 10. 复制静态文件
	if err := s.copyStaticFiles(); err != nil {
		return fmt.Errorf("复制静态文件失败: %w", err)
	}
	s.buildJiebaTags()
	s.buildRouteTree()

	log.Printf("构建完成: %d 个页面, %d 篇文章", len(s.Pages), len(s.Posts))
	return nil
}

// loadData 加载数据文件
func (s *Site) loadData() error {
	dataDir := filepath.Join(s.Config.Source, s.Config.DataDir)
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return nil // 数据目录不存在，跳过
	}

	return filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 只处理YAML文件
		ext := filepath.Ext(path)
		if ext != ".yml" && ext != ".yaml" {
			return nil
		}

		// 读取并解析数据文件
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取数据文件 %s 失败: %w", path, err)
		}

		var fileData interface{}
		if err := yaml.Unmarshal(data, &fileData); err != nil {
			return fmt.Errorf("解析数据文件 %s 失败: %w", path, err)
		}

		// 使用文件名（不含扩展名）作为键
		key := strings.TrimSuffix(filepath.Base(path), ext)
		s.Data[key] = fileData

		log.Printf("加载数据文件: %s", key)
		return nil
	})
}

// loadLayouts 加载布局文件
func (s *Site) loadLayouts() error {
	// 只从文件系统加载布局（不再加载嵌入式布局）
	layoutsDir := filepath.Join(s.Config.Source, s.Config.LayoutsDir)
	if _, err := os.Stat(layoutsDir); os.IsNotExist(err) {
		return fmt.Errorf("布局目录不存在: %s，请手动补充布局文件", layoutsDir)
	}

	return filepath.Walk(layoutsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 只处理HTML文件
		ext := filepath.Ext(path)
		if ext != ".html" && ext != ".htm" {
			return nil
		}

		// 读取布局文件
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取布局文件 %s 失败: %w", path, err)
		}

		// 创建布局对象
		layoutName := strings.TrimSuffix(filepath.Base(path), ext)
		layout := NewLayout(layoutName, string(content))

		s.mu.Lock()
		s.Layouts[layoutName] = layout
		s.mu.Unlock()

		log.Printf("加载文件布局: %s", layoutName)
		return nil
	})
}

// loadIncludes 加载包含文件
func (s *Site) loadIncludes() error {
	includesDir := filepath.Join(s.Config.Source, s.Config.IncludesDir)
	if _, err := os.Stat(includesDir); os.IsNotExist(err) {
		return nil // 包含目录不存在，跳过
	}

	// 正则表达式匹配 {{ define "name" }} 和 {{ end }}
	defineStartRegex := regexp.MustCompile(`(?sU)^\s*\{\{\s*define\s*"([^"]+)"\s*\}\}\s*`)
	defineEndRegex := regexp.MustCompile(`(?sU)\s*\{\{\s*end\s*\}\}\s*`)

	return filepath.Walk(includesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 只处理HTML文件
		ext := filepath.Ext(path)
		if ext != ".html" && ext != ".htm" {
			return nil
		}

		// 读取包含文件
		contentBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取包含文件 %s 失败: %w", path, err)
		}
		content := string(contentBytes)

		// 移除 {{ define "name" }} 和 {{ end }}
		content = defineStartRegex.ReplaceAllString(content, "")
		content = defineEndRegex.ReplaceAllString(content, "")

		// 创建布局对象，这里复用Layout结构体，但表示的是include
		includeName := strings.TrimSuffix(filepath.Base(path), ext)
		includeLayout := NewLayout(includeName, content)

		s.mu.Lock()
		s.Layouts[includeName] = includeLayout // 存储在Layouts中，方便统一管理
		s.mu.Unlock()

		log.Printf("加载包含: %s", includeName)
		return nil
	})
}

// loadPages 加载页面文件
func (s *Site) loadPages() error {
	return filepath.Walk(s.Config.Source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 跳过特殊目录
		relPath, _ := filepath.Rel(s.Config.Source, path)
		if strings.HasPrefix(relPath, "_") || strings.HasPrefix(relPath, ".") {
			return nil
		}

		// 只处理Markdown文件
		if !s.Config.IsMarkdownFile(path) {
			return nil
		}

		// 创建页面对象
		p, err := NewPage(path, s.Config)
		if err != nil {
			log.Printf("警告: 创建页面失败 %s: %v", path, err)
			return nil // 跳过有问题的文件，继续处理其他文件
		}

		// 验证Markdown格式
		if valid, errors := ValidateMarkdown(p.Content); !valid {
			log.Printf("警告: 页面 %s Markdown格式有问题: %v", filepath.Base(path), errors)
			// 继续处理，不中断程序
		}

		s.mu.Lock()
		s.Pages = append(s.Pages, p)
		s.mu.Unlock()

		log.Printf("加载页面: %s", relPath)
		return nil
	})
}

// loadPosts 加载文章文件
func (s *Site) loadPosts() error {
	postsDir := filepath.Join(s.Config.Source, s.Config.PostsDir)
	if _, err := os.Stat(postsDir); os.IsNotExist(err) {
		return nil // 文章目录不存在，跳过
	}

	return filepath.Walk(postsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// 只处理Markdown文件
		if !s.Config.IsMarkdownFile(path) {
			return nil
		}

		// 创建文章对象
		p, err := NewPost(path, s.Config)
		if err != nil {
			log.Printf("警告: 创建文章失败 %s: %v", path, err)
			return nil // 跳过有问题的文件，继续处理其他文件
		}

		// 验证Markdown格式
		if valid, errors := ValidateMarkdown(p.Content); !valid {
			log.Printf("警告: 文章 %s Markdown格式有问题: %v", filepath.Base(path), errors)
			// 继续处理，不中断程序
		}

		s.mu.Lock()
		s.Posts = append(s.Posts, p)
		s.mu.Unlock()

		log.Printf("加载文章: %s", filepath.Base(path))
		return nil
	})
}

// processCollections 处理分页、归档、标签、分类数据
func (s *Site) processCollections() {
	// 按日期排序文章
	sort.Slice(s.Posts, func(i, j int) bool {
		return s.Posts[i].Date.After(s.Posts[j].Date)
	})

	// 处理分页
	s.processPagination()

	// 处理归档
	s.processArchives()
}

// processPagination 处理分页
func (s *Site) processPagination() {
	if s.Config.Paginate <= 0 {
		return
	}

	s.PagedPosts = make([][]*Post, 0)
	for i := 0; i < len(s.Posts); i += s.Config.Paginate {
		end := i + s.Config.Paginate
		if end > len(s.Posts) {
			end = len(s.Posts)
		}
		s.PagedPosts = append(s.PagedPosts, s.Posts[i:end])
	}
}

// processArchives 处理归档
func (s *Site) processArchives() {
	s.Archives = make(map[string][]*Post)
	for _, post := range s.Posts {
		key := post.Date.Format("2006-01")
		s.Archives[key] = append(s.Archives[key], post)
	}
}

// render 渲染所有内容
func (s *Site) render() error {
	// 渲染页面
	for _, p := range s.Pages {
		if err := s.renderPage(p); err != nil {
			return fmt.Errorf("渲染页面失败 %s: %w", p.Path, err)
		}
	}

	// 渲染文章
	for _, p := range s.Posts {
		if err := s.renderPost(p); err != nil {
			return fmt.Errorf("渲染文章失败 %s: %w", p.Path, err)
		}
	}

	// 渲染分页页面
	if err := s.renderPagination(); err != nil {
		return fmt.Errorf("渲染分页失败: %w", err)
	}

	// 渲染归档页面
	if err := s.renderArchives(); err != nil {
		return fmt.Errorf("渲染归档失败: %w", err)
	}

	return nil
}

// renderPagination 渲染分页页面
func (s *Site) renderPagination() error {
	if len(s.PagedPosts) == 0 {
		return nil
	}

	for i, posts := range s.PagedPosts {
		pageNum := i + 1

		// 创建分页页面数据
		data := map[string]interface{}{
			"layout": "index",
			"title":  s.Config.Title,
			"posts":  posts,
			"page": map[string]interface{}{
				"number": pageNum,
				"total":  len(s.PagedPosts),
			},
			"site": map[string]interface{}{
				"title":       s.Config.Title,
				"subtitle":    s.Config.Subtitle,
				"description": s.Config.Description,
				"author":      s.Config.Author,
				"url":         s.Config.URL,
				"posts":       s.Posts,
				"pages":       s.Pages,
				"data":        s.Data,
				"archives":    s.Archives,
				"tags":        s.JiebaTags,
			},
		}

		// 设置CurrentData
		s.CurrentData = data

		// 渲染分页页面
		layout, exists := s.Layouts["index"]
		if !exists {
			return fmt.Errorf("布局 index 不存在")
		}

		content, err := s.Template.Render(layout, data)
		if err != nil {
			return fmt.Errorf("渲染分页页面失败: %w", err)
		}

		// 写入分页页面
		var outputPath string
		if pageNum == 1 {
			outputPath = filepath.Join(s.Config.Destination, "index.html")
		} else {
			outputPath = filepath.Join(s.Config.Destination, "page", fmt.Sprintf("%d", pageNum), "index.html")
		}
		outputDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}

		if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("写入分页页面失败: %w", err)
		}

		log.Printf("写入分页页面: 第 %d 页", pageNum)
	}

	return nil
}

// renderArchives 渲染归档页面
func (s *Site) renderArchives() error {
	// 如果没有归档，插入友好提示
	archives := s.Archives
	if len(archives) == 0 {
		archives = map[string][]*Post{"": {}}
	}
	data := map[string]interface{}{
		"layout":   "archive",
		"title":    "归档",
		"archives": archives,
		"site": map[string]interface{}{
			"title":       s.Config.Title,
			"subtitle":    s.Config.Subtitle,
			"description": s.Config.Description,
			"author":      s.Config.Author,
			"url":         s.Config.URL,
			"posts":       s.Posts,
			"pages":       s.Pages,
			"data":        s.Data,
			"archives":    archives,
			"tags":        s.JiebaTags,
		},
	}
	// 设置CurrentData
	s.CurrentData = data
	layout, exists := s.Layouts["archive"]
	if !exists {
		return fmt.Errorf("布局 archive 不存在")
	}
	content, err := s.Template.Render(layout, data)
	if err != nil {
		return fmt.Errorf("渲染归档页面失败: %w", err)
	}
	outputPath := filepath.Join(s.Config.Destination, "archives", "index.html")
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入归档页面失败: %w", err)
	}
	log.Printf("写入归档页面: /archives/")
	return nil
}

// write 写入输出目录
func (s *Site) write() error {
	// 创建输出目录
	if err := os.MkdirAll(s.Config.Destination, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 写入页面
	for _, p := range s.Pages {
		if err := s.writePage(p); err != nil {
			return fmt.Errorf("写入页面失败 %s: %w", p.Path, err)
		}
	}

	// 写入文章
	for _, p := range s.Posts {
		if err := s.writePost(p); err != nil {
			return fmt.Errorf("写入文章失败 %s: %w", p.Path, err)
		}
	}

	return nil
}

// writePage 写入单个页面
func (s *Site) writePage(p *Page) error {
	// 计算输出路径
	relPath, _ := filepath.Rel(s.Config.Source, p.Path)
	outputPath := filepath.Join(s.Config.Destination, strings.TrimSuffix(relPath, filepath.Ext(relPath))+".html")

	// 创建输出目录
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(outputPath, []byte(p.RenderedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	log.Printf("写入页面: %s", outputPath)
	return nil
}

// writePost 写入单个文章
func (s *Site) writePost(p *Post) error {
	// 从绝对URL中提取相对路径用于文件写入
	relativeURL := p.extractRelativeURL()

	// 1. 原有路径
	outputPath := filepath.Join(s.Config.Destination, relativeURL)
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}
	if err := os.WriteFile(outputPath, []byte(p.RenderedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	log.Printf("写入文章: %s", outputPath)

	// 2. 归档路径副本 /archives/年/月/日/slug.html
	archivePath := filepath.Join(s.Config.Destination, "archives", p.Date.Format("2006"), p.Date.Format("01"), p.Date.Format("02"), filepath.Base(relativeURL))
	archiveDir := filepath.Dir(archivePath)
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("创建归档输出目录失败: %w", err)
	}
	if err := os.WriteFile(archivePath, []byte(p.RenderedContent), 0644); err != nil {
		return fmt.Errorf("写入归档副本失败: %w", err)
	}
	log.Printf("写入归档副本: %s", archivePath)

	// 已移除分类和标签路径副本，使用jieba分词作为智能分类

	return nil
}

// copyStaticFiles 复制静态文件
func (s *Site) copyStaticFiles() error {
	// 复制 stylesheets 目录
	stylesheetsSrc := filepath.Join(s.Config.Source, "stylesheets")
	stylesheetsDest := filepath.Join(s.Config.Destination, "stylesheets")

	if _, err := os.Stat(stylesheetsSrc); err == nil {
		if err := s.copyDirectory(stylesheetsSrc, stylesheetsDest); err != nil {
			return fmt.Errorf("复制 stylesheets 失败: %w", err)
		}
		log.Printf("复制 stylesheets 目录")
	} else {
		return fmt.Errorf("未找到 stylesheets 目录: %s，请手动补充样式文件", stylesheetsSrc)
	}

	// 复制其他静态文件目录
	staticDirs := []string{"images", "js", "fonts", "assets"}
	for _, dir := range staticDirs {
		src := filepath.Join(s.Config.Source, dir)
		dest := filepath.Join(s.Config.Destination, dir)

		if _, err := os.Stat(src); err == nil {
			if err := s.copyDirectory(src, dest); err != nil {
				return fmt.Errorf("复制 %s 失败: %w", dir, err)
			}
			log.Printf("复制 %s 目录", dir)
		}
	}

	return nil
}

// copyDirectory 复制目录
func (s *Site) copyDirectory(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 计算相对路径
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dest, relPath)

		if info.IsDir() {
			// 创建目录
			return os.MkdirAll(destPath, 0755)
		} else {
			// 复制文件
			return s.copyFile(path, destPath)
		}
	})
}

// copyFile 复制文件
func (s *Site) copyFile(src, dest string) error {
	// 创建目标目录
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// 读取源文件
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// 写入目标文件
	return os.WriteFile(dest, data, 0644)
}

// Watch 监听文件变化
func (s *Site) Watch() error {
	// TODO: 实现文件监听功能
	log.Println("文件监听功能尚未实现")
	return nil
}

// Serve 启动本地服务器
func (s *Site) Serve(host string, port int) error {
	if err := s.Build(); err != nil {
		return fmt.Errorf("构建站点失败: %w", err)
	}

	// 使用 Gin 框架
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 静态文件服务
	r.Static("/stylesheets", filepath.Join(s.Config.Destination, "stylesheets"))
	r.Static("/images", filepath.Join(s.Config.Destination, "images"))
	r.Static("/js", filepath.Join(s.Config.Destination, "js"))
	r.Static("/fonts", filepath.Join(s.Config.Destination, "fonts"))
	r.Static("/assets", filepath.Join(s.Config.Destination, "assets"))

	// 搜索API
	r.GET("/api/search", func(c *gin.Context) {
		// 添加CORS头
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		query := c.Query("q")
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "查询参数不能为空"})
			return
		}

		// 使用URL二叉树进行前缀搜索
		results := s.URLTree.SearchPrefix(query)

		var response []gin.H
		for _, post := range results {
			response = append(response, gin.H{
				"title": post.Title,
				"url":   post.URL,
				"date":  post.Date.Format("2006-01-02"),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"query":   query,
			"results": response,
			"count":   len(response),
		})
	})

	// 兜底路由处理
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// 使用URL二叉树搜索
		if post := s.URLTree.Search(path); post != nil {
			// 找到文章，提供对应的HTML文件
			relativeURL := post.extractRelativeURL()
			filePath := filepath.Join(s.Config.Destination, relativeURL)
			if _, err := os.Stat(filePath); err == nil {
				c.Header("Content-Type", "text/html; charset=utf-8")
				c.File(filePath)
				return
			}
		}

		// 尝试直接提供文件
		filePath := filepath.Join(s.Config.Destination, path)
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath)
			return
		}

		// 尝试添加 .html 扩展名
		if !strings.HasSuffix(path, ".html") {
			htmlPath := filepath.Join(s.Config.Destination, path+".html")
			if _, err := os.Stat(htmlPath); err == nil {
				c.Header("Content-Type", "text/html; charset=utf-8")
				c.File(htmlPath)
				return
			}
		}

		// 尝试 index.html
		indexPath := filepath.Join(s.Config.Destination, path, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.File(indexPath)
			return
		}

		// 404 处理
		c.Status(http.StatusNotFound)
		c.String(http.StatusNotFound, "404 - 页面未找到")
	})

	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("启动服务器: http://%s", addr)
	log.Printf("服务目录: %s", s.Config.Destination)
	log.Println("按 Ctrl+C 停止服务器")

	return r.Run(addr)
}

// ensureProjectStructure 自动初始化项目结构和核心文件
func ensureProjectStructure() (fixed bool, report []string) {
	// 只检查目录，不再自动写入嵌入式样式和布局
	dirs := []string{"_layouts", "_posts", "_data", "_includes", "stylesheets"}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.Mkdir(dir, 0755)
			report = append(report, "已创建目录: "+dir)
			fixed = true
		}
	}
	return fixed, report
}

// doctorCheck 检查项目结构和文件格式
func doctorCheck(cfg *Config) (fixed bool, report []string) {
	report = append(report, "=== 开始检查项目结构 ===")

	// 1. 检查必要目录
	dirs := []string{"_layouts", "_posts", "_data", "_includes", "stylesheets"}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.Mkdir(dir, 0755)
			report = append(report, "✓ 已创建目录: "+dir)
			fixed = true
		} else {
			report = append(report, "✓ 目录存在: "+dir)
		}
	}

	// 2. 检查布局文件
	report = append(report, "\n=== 检查布局文件 ===")
	layoutsDir := filepath.Join(cfg.Source, cfg.LayoutsDir)
	if err := checkLayouts(layoutsDir, &report); err != nil {
		report = append(report, "✗ 布局检查失败: "+err.Error())
	} else {
		report = append(report, "✓ 布局文件检查完成")
	}

	// 3. 检查 Markdown 文件
	report = append(report, "\n=== 检查 Markdown 文件 ===")
	if err := checkMarkdownFiles(cfg, &report); err != nil {
		report = append(report, "✗ Markdown 检查失败: "+err.Error())
	} else {
		report = append(report, "✓ Markdown 文件检查完成")
	}

	// 4. 检查样式文件
	report = append(report, "\n=== 检查样式文件 ===")
	if err := checkStylesheets(cfg, &report); err != nil {
		report = append(report, "✗ 样式文件检查失败: "+err.Error())
	} else {
		report = append(report, "✓ 样式文件检查完成")
	}

	// 5. 检查配置文件
	report = append(report, "\n=== 检查配置文件 ===")
	if err := checkConfigFile(cfg, &report); err != nil {
		report = append(report, "✗ 配置文件检查失败: "+err.Error())
	} else {
		report = append(report, "✓ 配置文件检查完成")
	}

	report = append(report, "\n=== 检查完成 ===")
	return fixed, report
}

// checkLayouts 检查布局文件
func checkLayouts(layoutsDir string, report *[]string) error {
	if _, err := os.Stat(layoutsDir); os.IsNotExist(err) {
		*report = append(*report, "✗ 布局目录不存在: "+layoutsDir)
		return fmt.Errorf("布局目录不存在")
	}

	layouts := []string{"default.html", "post.html", "index.html", "archive.html", "tag.html", "category.html"}
	foundLayouts := 0

	for _, layout := range layouts {
		layoutPath := filepath.Join(layoutsDir, layout)
		if _, err := os.Stat(layoutPath); err == nil {
			*report = append(*report, "✓ 布局文件存在: "+layout)
			foundLayouts++

			// 检查布局文件内容
			if err := checkLayoutContent(layoutPath, report); err != nil {
				*report = append(*report, "  ⚠ 布局内容有问题: "+err.Error())
			}
		} else {
			*report = append(*report, "✗ 缺少布局文件: "+layout)
		}
	}

	if foundLayouts == 0 {
		*report = append(*report, "⚠ 警告: 没有找到任何布局文件")
	}

	return nil
}

// checkLayoutContent 检查布局文件内容
func checkLayoutContent(layoutPath string, report *[]string) error {
	content, err := os.ReadFile(layoutPath)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// 检查基本结构
	if !strings.Contains(contentStr, "<!DOCTYPE html>") {
		*report = append(*report, "    ⚠ 缺少 DOCTYPE 声明")
	}

	if !strings.Contains(contentStr, "<html") {
		*report = append(*report, "    ⚠ 缺少 <html> 标签")
	}

	if !strings.Contains(contentStr, "<head>") {
		*report = append(*report, "    ⚠ 缺少 <head> 标签")
	}

	if !strings.Contains(contentStr, "<body>") {
		*report = append(*report, "    ⚠ 缺少 <body> 标签")
	}

	// 检查模板变量
	if !strings.Contains(contentStr, "{{") {
		*report = append(*report, "    ⚠ 没有找到模板变量")
	}

	// 检查内容占位符
	if !strings.Contains(contentStr, "{{.content}}") && !strings.Contains(contentStr, "{{ content }}") {
		*report = append(*report, "    ⚠ 没有找到内容占位符 {{.content}}")
	}

	return nil
}

// checkMarkdownFiles 检查 Markdown 文件
func checkMarkdownFiles(cfg *Config, report *[]string) error {
	// 检查页面文件
	if err := checkMarkdownInDir(cfg.Source, cfg, report, "页面"); err != nil {
		return err
	}

	// 检查文章文件
	postsDir := filepath.Join(cfg.Source, cfg.PostsDir)
	if _, err := os.Stat(postsDir); err == nil {
		if err := checkMarkdownInDir(postsDir, cfg, report, "文章"); err != nil {
			return err
		}
	} else {
		*report = append(*report, "⚠ 文章目录不存在: "+postsDir)
	}

	return nil
}

// checkMarkdownInDir 检查目录中的 Markdown 文件
func checkMarkdownInDir(dir string, cfg *Config, report *[]string, fileType string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// 跳过特殊目录
			relPath, _ := filepath.Rel(dir, path)
			if strings.HasPrefix(relPath, "_") || strings.HasPrefix(relPath, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// 只检查 Markdown 文件
		if !cfg.IsMarkdownFile(path) {
			return nil
		}

		// 检查文件内容
		if err := checkMarkdownContent(path, report, fileType); err != nil {
			*report = append(*report, "✗ "+fileType+"文件检查失败 "+filepath.Base(path)+": "+err.Error())
		} else {
			*report = append(*report, "✓ "+fileType+"文件正常: "+filepath.Base(path))
		}

		return nil
	})
}

// checkMarkdownContent 检查 Markdown 文件内容
func checkMarkdownContent(filePath string, report *[]string, fileType string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// 检查前置数据
	if !strings.Contains(contentStr, "---") {
		*report = append(*report, "    ⚠ 缺少前置数据分隔符 ---")
		return fmt.Errorf("缺少前置数据")
	}

	// 解析前置数据
	fm, _, err := Parse(contentStr)
	if err != nil {
		return fmt.Errorf("前置数据解析失败: %w", err)
	}

	// 检查必要字段
	if title, ok := fm["title"].(string); ok && title != "" {
		*report = append(*report, "    ✓ 标题: "+title)
	} else {
		*report = append(*report, "    ⚠ 缺少标题")
	}

	if layout, ok := fm["layout"].(string); ok && layout != "" {
		*report = append(*report, "    ✓ 布局: "+layout)
	} else {
		*report = append(*report, "    ⚠ 缺少布局设置")
	}

	// 检查文章特有字段
	if fileType == "文章" {
		if date, ok := fm["date"].(time.Time); ok && !date.IsZero() {
			*report = append(*report, "    ✓ 日期: "+date.Format("2006-01-02"))
		} else if dateStr, ok := fm["date"].(string); ok && dateStr != "" {
			*report = append(*report, "    ✓ 日期: "+dateStr)
		} else {
			*report = append(*report, "    ⚠ 缺少日期")
		}

		// 检查文件名格式（仅对文章）
		filename := filepath.Base(filePath)
		if !filenameRegex.MatchString(filename) {
			*report = append(*report, "    ⚠ 文件名格式不正确，应为: YYYY-MM-DD-title.md")
		}
	}

	return nil
}

// checkStylesheets 检查样式文件
func checkStylesheets(cfg *Config, report *[]string) error {
	stylesheetsDir := filepath.Join(cfg.Source, "stylesheets")
	if _, err := os.Stat(stylesheetsDir); os.IsNotExist(err) {
		*report = append(*report, "✗ 样式目录不存在: "+stylesheetsDir)
		return fmt.Errorf("样式目录不存在")
	}

	// 检查主要样式文件
	mainStyles := []string{"site.css", "site.scss"}
	foundStyles := 0

	for _, style := range mainStyles {
		stylePath := filepath.Join(stylesheetsDir, style)
		if _, err := os.Stat(stylePath); err == nil {
			*report = append(*report, "✓ 样式文件存在: "+style)
			foundStyles++
		} else {
			*report = append(*report, "✗ 缺少样式文件: "+style)
		}
	}

	if foundStyles == 0 {
		*report = append(*report, "⚠ 警告: 没有找到任何样式文件")
	}

	return nil
}

// checkConfigFile 检查配置文件
func checkConfigFile(cfg *Config, report *[]string) error {
	configExtensions := []string{".yml", ".yaml", ".toml"}
	configFound := false

	for _, ext := range configExtensions {
		configPath := filepath.Join(cfg.Source, "_config"+ext)
		if _, err := os.Stat(configPath); err == nil {
			*report = append(*report, "✓ 配置文件存在: _config"+ext)
			configFound = true

			// 检查配置文件内容
			if err := checkConfigContent(configPath, report); err != nil {
				*report = append(*report, "  ⚠ 配置文件内容有问题: "+err.Error())
			}
			break
		}
	}

	if !configFound {
		*report = append(*report, "⚠ 没有找到配置文件，将使用默认配置")
	}

	return nil
}

// checkConfigContent 检查配置文件内容
func checkConfigContent(configPath string, report *[]string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// 检查基本配置项
	requiredFields := []string{"title", "description", "author", "url"}
	for _, field := range requiredFields {
		if !strings.Contains(contentStr, field+":") {
			*report = append(*report, "    ⚠ 缺少配置项: "+field)
		} else {
			*report = append(*report, "    ✓ 配置项存在: "+field)
		}
	}

	return nil
}

func createNewPost(title string, cfg *Config) error {
	// 生成文件名
	dir := "_posts"
	if cfg.PostsDir != "" {
		dir = cfg.PostsDir
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	date := time.Now()
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	filename := filepath.Join(dir, date.Format("2006-01-02")+"-"+slug+".md")
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("文件已存在: %s", filename)
	}

	frontMatter := "---\n" +
		"layout: post\n" +
		"title: \"" + title + "\"\n" +
		"date: " + date.Format("2006-01-02 15:04:05 -0700") + "\n" +
		"categories: []\n" +
		"tags: []\n" +
		"comments: true\n" +
		"---\n\n这里是正文内容。\n"

	return os.WriteFile(filename, []byte(frontMatter), 0644)
}

func createNewPage(name string, cfg *Config) error {
	// 生成文件名

	dir := name
	if strings.ContainsAny(name, "/\\") {
		dir = filepath.Dir(name)
		name = filepath.Base(name)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := filepath.Join(dir, "index.md")
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("文件已存在: %s", filename)
	}
	date := time.Now()
	frontMatter := "---\n" +
		"layout: page\n" +
		"title: \"" + name + "\"\n" +
		"date: " + date.Format("2006-01-02 15:04:05 -0700") + "\n" +
		"comments: true\n" +
		"sharing: true\n" +
		"footer: true\n" +
		"---\n\n这里是页面内容。\n"
	return os.WriteFile(filename, []byte(frontMatter), 0644)
}

// ==================== 文件监听器 ====================

// FileWatcher 增强的文件监听器，支持SHA256哈希值比较
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

// NewFileWatcher 创建新的文件监听器
func NewFileWatcher(site *Site, cfg *Config) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("创建文件监听器失败: %w", err)
	}

	fw := &FileWatcher{
		watcher:    watcher,
		site:       site,
		config:     cfg,
		fileHashes: make(map[string]string),
		debounce:   500 * time.Millisecond, // 500ms防抖
		rebuildCh:  make(chan struct{}, 1),
	}

	// 初始化文件哈希值
	if err := fw.initializeFileHashes(); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("初始化文件哈希值失败: %w", err)
	}

	return fw, nil
}

// initializeFileHashes 初始化所有监听文件的哈希值
func (fw *FileWatcher) initializeFileHashes() error {
	dirs := []string{".", "_posts", "_layouts", "_includes", "_data"}

	for _, dir := range dirs {
		if err := fw.watcher.Add(dir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("添加监听目录失败 %s: %w", dir, err)
		}

		// 计算目录下所有文件的哈希值
		if err := fw.calculateDirHashes(dir); err != nil {
			return fmt.Errorf("计算目录哈希值失败 %s: %w", dir, err)
		}
	}

	return nil
}

// calculateDirHashes 计算目录下所有文件的哈希值
func (fw *FileWatcher) calculateDirHashes(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和隐藏文件
		if info.IsDir() || strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		// 跳过_site目录
		if strings.Contains(path, "_site") {
			return nil
		}

		hash, err := fw.calculateFileHash(path)
		if err != nil {
			return fmt.Errorf("计算文件哈希值失败 %s: %w", path, err)
		}

		fw.mu.Lock()
		fw.fileHashes[path] = hash
		fw.mu.Unlock()

		return nil
	})
}

// calculateFileHash 计算单个文件的SHA256哈希值
func (fw *FileWatcher) calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hasFileChanged 检查文件是否发生变化
func (fw *FileWatcher) hasFileChanged(filePath string) (bool, error) {
	newHash, err := fw.calculateFileHash(filePath)
	if err != nil {
		return false, err
	}

	fw.mu.RLock()
	oldHash, exists := fw.fileHashes[filePath]
	fw.mu.RUnlock()

	if !exists {
		// 新文件
		fw.mu.Lock()
		fw.fileHashes[filePath] = newHash
		fw.mu.Unlock()
		return true, nil
	}

	if oldHash != newHash {
		// 文件内容发生变化
		fw.mu.Lock()
		fw.fileHashes[filePath] = newHash
		fw.mu.Unlock()
		return true, nil
	}

	return false, nil
}

// removeFileHash 移除文件的哈希值记录（文件被删除时）
func (fw *FileWatcher) removeFileHash(filePath string) {
	fw.mu.Lock()
	delete(fw.fileHashes, filePath)
	fw.mu.Unlock()
}

// debouncedRebuild 防抖重建
func (fw *FileWatcher) debouncedRebuild() {
	if fw.timer != nil {
		fw.timer.Stop()
	}

	fw.timer = time.AfterFunc(fw.debounce, func() {
		select {
		case fw.rebuildCh <- struct{}{}:
		default:
		}
	})
}

// Start 开始监听文件变化
func (fw *FileWatcher) Start() error {
	fmt.Println("[watch] 正在监听文件变化，按 Ctrl+C 退出...")

	// 启动重建协程
	go fw.rebuildWorker()

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return nil
			}
			fw.handleFileEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Printf("[watch] 错误: %v\n", err)
		}
	}
}

// handleFileEvent 处理文件事件
func (fw *FileWatcher) handleFileEvent(event fsnotify.Event) {
	// 跳过隐藏文件和_site目录
	if strings.HasPrefix(filepath.Base(event.Name), ".") ||
		strings.Contains(event.Name, "_site") {
		return
	}

	switch {
	case event.Op&fsnotify.Write == fsnotify.Write:
		// 文件写入
		if changed, err := fw.hasFileChanged(event.Name); err != nil {
			fmt.Printf("[watch] 检查文件变化失败 %s: %v\n", event.Name, err)
		} else if changed {
			fmt.Printf("[watch] 检测到文件变化: %s\n", event.Name)
			fw.debouncedRebuild()
		}

	case event.Op&fsnotify.Create == fsnotify.Create:
		// 文件创建
		if !strings.HasPrefix(filepath.Base(event.Name), ".") {
			fmt.Printf("[watch] 检测到新文件: %s\n", event.Name)
			fw.debouncedRebuild()
		}

	case event.Op&fsnotify.Remove == fsnotify.Remove:
		// 文件删除
		fmt.Printf("[watch] 检测到文件删除: %s\n", event.Name)
		fw.removeFileHash(event.Name)
		fw.debouncedRebuild()

	case event.Op&fsnotify.Rename == fsnotify.Rename:
		// 文件重命名
		fmt.Printf("[watch] 检测到文件重命名: %s\n", event.Name)
		fw.removeFileHash(event.Name)
		fw.debouncedRebuild()
	}
}

// rebuildWorker 重建工作协程
func (fw *FileWatcher) rebuildWorker() {
	for range fw.rebuildCh {
		fmt.Println("[watch] 开始自动重建...")
		start := time.Now()

		if err := fw.site.Build(); err != nil {
			fmt.Printf("[watch] 自动重建失败: %v\n", err)
		} else {
			duration := time.Since(start)
			fmt.Printf("[watch] 自动重建完成！耗时: %v\n", duration)
		}
	}
}

// Close 关闭文件监听器
func (fw *FileWatcher) Close() error {
	if fw.timer != nil {
		fw.timer.Stop()
	}
	close(fw.rebuildCh)
	return fw.watcher.Close()
}

// ==================== 主程序 ====================

func main() {
	// 解析命令行参数，模仿 Jekyll 的使用方式
	var (
		flagServe        = flag.Bool("serve", false, "启动本地服务器")
		flagWatch        = flag.Bool("watch", false, "监听文件变化自动重建")
		flagPort         = flag.Int("port", 4000, "服务器端口")
		flagHost         = flag.String("host", "127.0.0.1", "服务器主机")
		flagBaseURL      = flag.String("baseurl", "", "站点基础URL")
		flagConfig       = flag.String("config", "_config.yml", "配置文件路径")
		flagSource       = flag.String("source", ".", "源目录路径")
		flagDestination  = flag.String("destination", "_site", "输出目录路径")
		flagVerbose      = flag.Bool("verbose", false, "详细输出")
		flagQuiet        = flag.Bool("quiet", false, "静默模式")
		flagNewPost      = flag.String("new_post", "", "新建文章（标题）")
		flagNewPage      = flag.String("new_page", "", "新建页面（名称）")
		flagDoctor       = flag.Bool("doctor", false, "自动修复项目结构和缺失文件")
		flagTestMarkdown = flag.Bool("test-markdown", false, "测试Markdown格式健壮性")
		flagHelp         = flag.Bool("help", false, "显示帮助信息")
		flagVersion      = flag.Bool("version", false, "显示版本信息")
	)
	flag.Parse()

	// 显示帮助信息
	if *flagHelp {
		showHelp()
		return
	}

	// 显示版本信息
	if *flagVersion {
		showVersion()
		return
	}

	// 设置日志级别
	if *flagQuiet {
		log.SetOutput(os.Stderr)
	} else if *flagVerbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// 加载配置
	cfg, err := Load(*flagConfig, *flagSource, *flagDestination)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 应用命令行参数到配置
	cfg.Port = *flagPort
	cfg.Host = *flagHost
	if *flagBaseURL != "" {
		cfg.BaseURL = *flagBaseURL
	}

	// 处理新建文章
	if *flagNewPost != "" {
		err := createNewPost(*flagNewPost, cfg)
		if err != nil {
			log.Fatalf("新建文章失败: %v", err)
		}
		fmt.Printf("新建文章成功: %s\n", *flagNewPost)
		return
	}

	// 处理新建页面
	if *flagNewPage != "" {
		err := createNewPage(*flagNewPage, cfg)
		if err != nil {
			log.Fatalf("新建页面失败: %v", err)
		}
		fmt.Printf("新建页面成功: %s\n", *flagNewPage)
		return
	}

	// 处理项目结构修复
	if *flagDoctor {
		fixed, report := doctorCheck(cfg)
		for _, r := range report {
			fmt.Println(r)
		}
		if fixed {
			fmt.Println("\n项目结构已修复，请重新构建。")
		} else {
			fmt.Println("\n项目结构检查完成。")
		}
		return
	}

	// 处理Markdown格式测试
	if *flagTestMarkdown {
		testMarkdownRobustness(cfg)
		return
	}

	// 创建站点实例
	site := New(cfg)

	// 处理监听模式
	if *flagWatch {
		fileWatcher, err := NewFileWatcher(site, cfg)
		if err != nil {
			log.Fatalf("创建文件监听器失败: %v", err)
		}
		defer fileWatcher.Close()

		if err := fileWatcher.Start(); err != nil {
			log.Fatalf("监听失败: %v", err)
		}
		return
	}

	// 处理服务器模式
	if *flagServe {
		if err := site.Serve(cfg.Host, cfg.Port); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
		return
	}

	// 默认构建模式
	// 构建前先 doctor 检查
	fixed, report := doctorCheck(cfg)
	for _, r := range report {
		fmt.Println(r)
	}
	if fixed {
		fmt.Println("\n项目结构已修复，请重新构建。")
		return
	}
	// 检查通过才继续构建
	if err := site.Build(); err != nil {
		log.Fatalf("构建失败: %v", err)
	}
	fmt.Println("构建完成！")
}

// showHelp 显示帮助信息
func showHelp() {
	fmt.Println(`Jekyll-like Static Site Generator

用法:
  main [命令] [选项]

命令:
  build             构建站点 (默认)
  serve             启动本地服务器
  watch             监听文件变化自动重建
  new_post          新建文章
  new_page          新建页面
  doctor            自动修复项目结构
  test-markdown     测试Markdown格式健壮性

选项:
  --config PATH     配置文件路径 (默认: _config.yml)
  --source PATH     源目录路径 (默认: .)
  --destination PATH 输出目录路径 (默认: _site)
  --port PORT       服务器端口 (默认: 4000)
  --host HOST       服务器主机 (默认: 127.0.0.1)
  --baseurl URL     站点基础URL
  --draft           构建草稿文章
  --future          构建未来日期的文章
  --limit N         限制构建的文章数量
  --safe            安全模式（禁用插件）
  --verbose         详细输出
  --quiet           静默模式
  --help            显示帮助信息
  --version         显示版本信息

示例:
  main                    # 构建站点
  main serve              # 启动服务器
  main serve --port 8080  # 在端口8080启动服务器
  main watch              # 监听文件变化
  main new_post "我的文章"  # 新建文章
  main new_page "关于"     # 新建页面
  main doctor             # 修复项目结构

更多信息请访问: https://github.com/your-repo/jekyll-go`)
}

// showVersion 显示版本信息
func showVersion() {
	fmt.Println("Jekyll-like Static Site Generator v1.0.0")
	fmt.Println("Go version: go1.21+")
	fmt.Println("License: MIT")
}

// testMarkdownRobustness 测试Markdown格式的健壮性
func testMarkdownRobustness(cfg *Config) {
	fmt.Println("=== 测试Markdown格式健壮性 ===")

	// 测试用例
	testCases := []struct {
		name    string
		content string
		expect  bool
	}{
		{
			name: "正常Markdown",
			content: `---
title: "测试文章"
date: 2024-01-01
layout: post
---

# 标题1

这是一段正常的内容。

## 标题2

- 列表项1
- 列表项2

` + "```go\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n```" + `
`,
			expect: true,
		},
		{
			name: "缺少前置数据",
			content: `# 标题

这是没有前置数据的内容。

- 列表项
`,
			expect: true,
		},
		{
			name: "格式错误的标题",
			content: `---
title: "测试"
---

#标题1
##标题2

正常内容
`,
			expect: false,
		},
		{
			name: "格式错误的列表",
			content: `---
title: "测试"
---

-列表项1
*列表项2
+列表项3
`,
			expect: false,
		},
		{
			name: "未闭合的代码块",
			content: `---
title: "测试"
---

` + "```go\nfunc main() {\n    fmt.Println(\"Hello\")\n// 缺少结束标记" + `
`,
			expect: false,
		},
		{
			name:    "空内容",
			content: "",
			expect:  false,
		},
		{
			name:    "只有空白字符",
			content: "   \n\t\n  ",
			expect:  false,
		},
		{
			name: "YAML格式错误",
			content: `---
title: "测试"
date: 2024-01-01
layout: post
invalid: yaml: format
---

正常内容
`,
			expect: true, // YAML错误应该被捕获但不导致崩溃
		},
	}

	// 验证Markdown格式
	for i, testCase := range testCases {
		valid, errors := ValidateMarkdown(testCase.content)

		fmt.Printf("测试 %d (%s): ", i+1, testCase.name)
		if valid == testCase.expect {
			fmt.Printf("✓ 通过")
		} else {
			fmt.Printf("✗ 失败")
		}

		if !valid && len(errors) > 0 {
			fmt.Printf(" (错误: %s)", strings.Join(errors, ", "))
		}
		fmt.Println()
	}

	// 测试Markdown转换
	fmt.Println("\n=== 测试Markdown转换 ===")
	converter := NewConverter(cfg)

	for i, testCase := range testCases {
		fmt.Printf("转换测试 %d (%s): ", i+1, testCase.name)

		html, err := converter.Convert(testCase.content)
		if err != nil {
			fmt.Printf("✗ 转换失败 - %v\n", err)
		} else {
			fmt.Printf("✓ 转换成功 (长度: %d)\n", len(html))
		}
	}

	fmt.Println("\n=== Markdown健壮性测试完成 ===")
}

func (s *Site) buildJiebaTags() {
	x := gojieba.NewJieba()
	defer x.Free()
	freq := map[string]int{}
	for _, post := range s.Posts {
		words := x.CutForSearch(post.Title, true)
		for _, w := range words {
			if len([]rune(w)) < 2 {
				continue
			}
			freq[w]++
		}
	}
	type kv struct {
		K string
		V int
	}
	var arr []kv
	for k, v := range freq {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].V > arr[j].V })
	s.JiebaTags = nil
	for i, kv := range arr {
		if i >= 20 {
			break
		}
		s.JiebaTags = append(s.JiebaTags, kv.K)
	}
	if len(s.JiebaTags) == 0 {
		s.JiebaTags = append(s.JiebaTags, "无标签")
	}
}

func (s *Site) buildRouteTree() {
	s.RouteTree = treemap.NewWith(utils.StringComparator)
	for _, post := range s.Posts {
		relativeURL := post.extractRelativeURL()
		s.RouteTree.Put(relativeURL, relativeURL)
		archivePath := fmt.Sprintf("/archives/%04d/%02d/%02d/%s", post.Date.Year(), post.Date.Month(), post.Date.Day(), filepath.Base(relativeURL))
		s.RouteTree.Put(archivePath, relativeURL)
		// 已移除分类和标签路径，使用jieba分词作为智能分类
	}
}

// buildURLTree 构建URL二叉树
func (s *Site) buildURLTree() {
	for _, post := range s.Posts {
		relativeURL := post.extractRelativeURL()
		s.URLTree.Insert(relativeURL, post)

		// 也插入归档路径
		archivePath := fmt.Sprintf("/archives/%04d/%02d/%02d/%s", post.Date.Year(), post.Date.Month(), post.Date.Day(), filepath.Base(relativeURL))
		s.URLTree.Insert(archivePath, post)

		// 已移除分类和标签路径，使用jieba分词作为智能分类
	}

	log.Printf("URL二叉树构建完成，包含 %d 个文章节点", len(s.Posts))
}

// generateRSSFeed 生成 RSS Feed
func (s *Site) generateRSSFeed() error {
	// 按日期排序文章（最新的在前）
	sortedPosts := make([]*Post, len(s.Posts))
	copy(sortedPosts, s.Posts)
	sort.Slice(sortedPosts, func(i, j int) bool {
		return sortedPosts[i].Date.After(sortedPosts[j].Date)
	})

	// 只取最新的20篇文章
	if len(sortedPosts) > 20 {
		sortedPosts = sortedPosts[:20]
	}

	// 生成 RSS XML
	rssTemplate := `<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>{{ .Title }}</title>
    <link>{{ .URL }}</link>
    <description>{{ .Description }}</description>
    <language>zh-CN</language>
    <lastBuildDate>{{ .LastBuildDate }}</lastBuildDate>
    <atom:link href="{{ .URL }}/feed.xml" rel="self" type="application/rss+xml" />
    {{ range .Posts }}
    <item>
      <title>{{ .Title }}</title>
      <link>{{ $.URL }}/{{ .URL }}</link>
      <guid>{{ $.URL }}/{{ .URL }}</guid>
      <pubDate>{{ .Date.Format "Mon, 02 Jan 2006 15:04:05 -0700" }}</pubDate>
      <description><![CDATA[{{ .Excerpt }}]]></description>
    </item>
    {{ end }}
  </channel>
</rss>`

	// 准备数据
	data := map[string]interface{}{
		"Title":         s.Config.Title,
		"URL":           s.Config.URL,
		"Description":   s.Config.Description,
		"LastBuildDate": time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700"),
		"Posts":         sortedPosts,
	}

	// 渲染 RSS
	tmpl, err := template.New("rss").Parse(rssTemplate)
	if err != nil {
		return fmt.Errorf("解析RSS模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("渲染RSS失败: %w", err)
	}

	// 手动添加 XML 声明
	s.RSSFeed = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + buf.String()

	// 写入 RSS 文件
	rssPath := filepath.Join(s.Config.Destination, "feed.xml")
	if err := os.WriteFile(rssPath, []byte(s.RSSFeed), 0644); err != nil {
		return fmt.Errorf("写入RSS文件失败: %w", err)
	}

	log.Printf("生成RSS Feed: %s", rssPath)
	return nil
}

// generateSitemap 生成 Sitemap
func (s *Site) generateSitemap() error {
	// 收集所有URL
	var urls []string

	// 添加首页
	urls = append(urls, "/")

	// 添加页面
	for _, page := range s.Pages {
		relPath, _ := filepath.Rel(s.Config.Source, page.Path)
		url := strings.TrimSuffix(relPath, filepath.Ext(relPath)) + ".html"
		urls = append(urls, "/"+strings.ReplaceAll(url, string(filepath.Separator), "/"))
	}

	// 添加文章
	for _, post := range s.Posts {
		relativeURL := post.extractRelativeURL()
		// 确保URL不以斜杠开头，避免重复斜杠
		if strings.HasPrefix(relativeURL, "/") {
			relativeURL = relativeURL[1:]
		}
		urls = append(urls, "/"+relativeURL)
	}

	// 添加归档页面
	urls = append(urls, "/archives/")

	// 添加标签页面
	urls = append(urls, "/tags/")

	// 添加分类页面
	urls = append(urls, "/categories/")

	// 生成 Sitemap XML
	sitemapTemplate := `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  {{ range .URLs }}
  <url>
    <loc>{{ $.BaseURL }}{{ . }}</loc>
    <lastmod>{{ $.LastMod }}</lastmod>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>
  {{ end }}
</urlset>`

	// 准备数据
	data := map[string]interface{}{
		"BaseURL": s.Config.URL,
		"LastMod": time.Now().Format("2006-01-02"),
		"URLs":    urls,
	}

	// 渲染 Sitemap
	tmpl, err := template.New("sitemap").Parse(sitemapTemplate)
	if err != nil {
		return fmt.Errorf("解析Sitemap模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("渲染Sitemap失败: %w", err)
	}

	// 手动添加 XML 声明
	s.SitemapXML = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + buf.String()

	// 写入 Sitemap 文件
	sitemapPath := filepath.Join(s.Config.Destination, "sitemap.xml")
	if err := os.WriteFile(sitemapPath, []byte(s.SitemapXML), 0644); err != nil {
		return fmt.Errorf("写入Sitemap文件失败: %w", err)
	}

	log.Printf("生成Sitemap: %s", sitemapPath)
	return nil
}

// renderPage 渲染单个页面
func (s *Site) renderPage(p *Page) error {
	// 转换Markdown内容
	htmlContent, _ := s.Converter.Convert(p.Content)
	p.RenderedContent = htmlContent

	// 应用布局
	layoutName := p.Layout
	if layoutName == "" {
		layoutName = "default"
	}
	layout, exists := s.Layouts[layoutName]
	if !exists {
		return fmt.Errorf("布局不存在: %s", layoutName)
	}
	data := map[string]interface{}{
		"page": p,
		"site": map[string]interface{}{
			"title":       s.Config.Title,
			"subtitle":    s.Config.Subtitle,
			"description": s.Config.Description,
			"author":      s.Config.Author,
			"url":         s.Config.URL,
			"posts":       s.Posts,
			"pages":       s.Pages,
			"data":        s.Data,
			"archives":    s.Archives,
			"JiebaTags":   s.JiebaTags,
		},
		"content": template.HTML(p.RenderedContent),
	}

	html, err := s.Template.Render(layout, data)
	if err != nil {
		return fmt.Errorf("应用布局失败: %w", err)
	}
	p.RenderedContent = html
	return nil
}

// renderPost 渲染单个文章
func (s *Site) renderPost(p *Post) error {
	// 自动去除正文开头的一级标题，避免和页面主标题重复
	lines := strings.Split(p.Content, "\n")
	newLines := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !removed && (strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "#\t")) {
			removed = true
			continue // 跳过第一个一级标题
		}
		newLines = append(newLines, line)
	}
	contentNoH1 := strings.Join(newLines, "\n")

	// 转换Markdown内容
	htmlContent, err := s.Converter.Convert(contentNoH1)
	if err != nil {
		return fmt.Errorf("转换Markdown失败: %w", err)
	}
	p.RenderedContent = htmlContent

	// 强制使用 post.html 布局
	layoutName := "post"
	if p.Layout != "" && p.Layout != "default" {
		layoutName = p.Layout
	}
	layout, exists := s.Layouts[layoutName]
	if !exists {
		return fmt.Errorf("布局不存在: %s", layoutName)
	}

	// 准备渲染数据
	data := map[string]interface{}{
		"post":    p,
		"content": template.HTML(p.RenderedContent),
		"site": map[string]interface{}{
			"title":       s.Config.Title,
			"subtitle":    s.Config.Subtitle,
			"description": s.Config.Description,
			"author":      s.Config.Author,
			"url":         s.Config.URL,
			"posts":       s.Posts,
			"pages":       s.Pages,
			"data":        s.Data,
			"archives":    s.Archives,
			"JiebaTags":   s.JiebaTags,
		},
	}

	html, err := s.Template.Render(layout, data)
	if err != nil {
		return fmt.Errorf("应用布局失败: %w", err)
	}
	p.RenderedContent = html
	return nil
}
