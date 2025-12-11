package main

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/brotli"
)

// 添加全局变量用于调试
var debugMode = false

func main() {
	// 检查是否启用调试模式
	debugMode = false
	for _, arg := range os.Args {
		if arg == "--debug" {
			debugMode = true
		}
	}
	
	// 检查是否请求帮助
	for _, arg := range os.Args {
		if arg == "--help" || arg == "-h" {
			printHelp()
			return
		}
	}
	
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	isLocal := false
	isSeries := false
	isLocalSeries := false
	startChapterID := ""
	input := ""
	id := ""

	// 解析命令行参数（跳过--debug参数）
	args := []string{}
	for _, arg := range os.Args[1:] {
		if arg != "--debug" {
			args = append(args, arg)
		}
	}
	
	// 解析参数
	i := 0
	for i < len(args) {
		if args[i] == "--local" && i+1 < len(args) {
			isLocal = true
			input = args[i+1]
			id = "local_" + input
			i += 2
		} else if args[i] == "--series" && i+1 < len(args) {
			isSeries = true
			input = args[i+1]
			id = input
			i += 2
		} else if args[i] == "--local-series" && i+1 < len(args) {
			isLocalSeries = true
			input = args[i+1]
			id = "local_series_" + input
			i += 2
		} else if args[i] == "--start" && i+1 < len(args) {
			startChapterID = args[i+1]
			i += 2
		} else if i == 0 {
			// 第一个参数默认为章节ID
			input = args[i]
			id = input
			i++
		} else {
			i++
		}
	}

	if isLocalSeries {
		// 从本地文件下载整个漫画系列
		downloadLocalSeries(input)
		return
	}

	if isSeries {
		// 下载整个漫画系列，支持从指定章节开始
		downloadSeries(input, startChapterID)
		return
	}

	var doc *goquery.Document
	var err error

	if isLocal {
		// 从本地文件解析
		fmt.Printf("正在从本地文件 %s 解析图片链接...\n", input)
		doc, err = parseLocalFile(input)
		if err != nil {
			fmt.Printf("解析本地文件失败: %v\n", err)
			return
		}
	} else {
		// 从网络下载
		var url string
		if strings.Contains(id, "92hm.life") {
			url = input // 如果输入完整URL，则直接使用
		} else {
			// 默认使用新的网站格式
			url = "https://www.92hm.life/chapter/" + id
		}

		fmt.Printf("正在下载章节 %s 的图片...\n", id)

		// 获取页面内容（带重试机制）
		doc, err = fetchPageWithRetry(url, 3)
		if err != nil {
			fmt.Printf("获取页面失败: %v\n", err)
			return
		}
	}

	// 提取图片链接
	imageUrls := extractImageUrls(doc)
	if len(imageUrls) == 0 {
		fmt.Println("未找到任何图片链接，请检查选择器是否正确")
		return
	}
	
	fmt.Printf("找到 %d 张图片\n", len(imageUrls))

	// 为单章节创建目录
	chapterTitle := extractChapterTitle(doc)
	if chapterTitle == "" {
		chapterTitle = "chapter_" + id
	}
	
	// 创建保存图片的目录
	dirName := chapterTitle
	err = os.MkdirAll(dirName, 0755)
	if err != nil {
		fmt.Printf("创建目录失败: %v\n", err)
		return
	}

	// 下载图片
	for i, imgUrl := range imageUrls {
		// 使用4位数字编号，例如 0001.jpg, 0002.jpg 等
		filename := fmt.Sprintf("%s/%04d.jpg", dirName, i+1)
		
		// 无论本地还是网络模式都尝试下载图片
		err := downloadImageWithRetry(imgUrl, filename, 3)
		if err != nil {
			fmt.Printf("下载图片 %d 失败: %v\n", i+1, err)
			continue
		}
		fmt.Printf("已下载图片 %d/%d: %s\n", i+1, len(imageUrls), filename)
	}

	fmt.Printf("\n章节《%s》下载完成! 图片保存在 %s 目录中\n", chapterTitle, dirName)
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Println("漫画下载器使用说明:")
	fmt.Println("  从网页下载单章节: ./comicbox <章节ID>")
	fmt.Println("  例如: ./comicbox 16124")
	fmt.Println("")
	fmt.Println("  从网页下载整个漫画: ./comicbox --series <漫画ID>")
	fmt.Println("  例如: ./comicbox --series 418")
	fmt.Println("")
	fmt.Println("  从指定章节开始下载整个漫画: ./comicbox --series <漫画ID> --start <起始章节ID>")
	fmt.Println("  例如: ./comicbox --series 418 --start 16124")
	fmt.Println("")
	fmt.Println("  从本地文件解析并下载: ./comicbox --local <本地HTML文件路径>")
	fmt.Println("  例如: ./comicbox --local hm_page.html")
	fmt.Println("")
	fmt.Println("  从本地文件解析并批量下载整个漫画: ./comicbox --local-series <本地目录HTML文件路径>")
	fmt.Println("  例如: ./comicbox --local-series comic_index.html")
	fmt.Println("")
	fmt.Println("  启用调试模式: 在任何命令前加上 --debug 参数")
	fmt.Println("  例如: ./comicbox --debug 16124")
	fmt.Println("")
	fmt.Println("下载完成后，可以使用以下方式阅读漫画:")
	fmt.Println("  1. 直接使用支持漫画格式的阅读器打开图片目录")
	fmt.Println("  2. 使用 pack 工具将章节打包为 CBZ 格式:")
	fmt.Println("     ./pack chapter_16124          # 打包单个章节")
	fmt.Println("     ./pack chapter_*              # 批量打包所有章节")
	fmt.Println("     CBZ文件可以使用ComicGlass、CDisplayEx等专业漫画阅读器打开")
	fmt.Println("")
	fmt.Println("注意: 章节ID为URL中的数字部分，如 https://www.92hm.life/chapter/16124 中的 16124")
	fmt.Println("     漫画ID为URL中的数字部分，如 https://www.92hm.life/book/418 中的 418")
}

// downloadLocalSeries 从本地目录文件下载整个漫画系列
func downloadLocalSeries(filePath string) {
	fmt.Printf("正在从本地文件 %s 下载漫画系列...\n", filePath)
	
	// 解析本地目录文件
	doc, err := parseLocalFile(filePath)
	if err != nil {
		fmt.Printf("解析本地目录文件失败: %v\n", err)
		return
	}
	
	// 提取章节链接
	chapters := extractChapterLinks(doc)
	if len(chapters) == 0 {
		fmt.Println("未找到任何章节链接")
		return
	}
	
	// 获取漫画标题
	comicTitle := extractComicTitle(doc)
	if comicTitle == "" {
		comicTitle = "local_comic"
	}
	
	// 创建漫画主目录
	err = os.MkdirAll(comicTitle, 0755)
	if err != nil {
		fmt.Printf("创建漫画主目录失败: %v\n", err)
		return
	}
	
	fmt.Printf("漫画标题: %s\n", comicTitle)
	fmt.Printf("找到 %d 个章节\n", len(chapters))
	
	// 为了演示目的，我们只下载第一个章节
	// 实际使用时，这里会遍历所有章节
	if len(chapters) > 0 {
		chapter := chapters[0] // 只下载第一个章节作为演示
		// 使用更具描述性的章节目录名
		chapterDirName := fmt.Sprintf("%03d_%s", 1, sanitizeFileName(chapter.title))
		
		fmt.Printf("\n正在下载章节: %s (%s)\n", chapter.title, chapter.id)
		
		// 对于本地演示，我们使用之前保存的hm_page.html作为示例
		doc, err := parseLocalFile("hm_page.html")
		if err != nil {
			fmt.Printf("解析章节页面失败: %v\n", err)
			return
		}
		
		// 提取图片链接
		imageUrls := extractImageUrls(doc)
		if len(imageUrls) == 0 {
			fmt.Println("未找到任何图片链接")
			return
		}
		
		fmt.Printf("找到 %d 张图片\n", len(imageUrls))
		
		// 创建保存图片的目录（在漫画主目录下）
		dirName := filepath.Join(comicTitle, chapterDirName)
		err = os.MkdirAll(dirName, 0755)
		if err != nil {
			fmt.Printf("创建目录失败: %v\n", err)
			return
		}
		
		// 下载图片
		for j, imgUrl := range imageUrls {
			// 使用4位数字编号，例如 0001.jpg, 0002.jpg 等
			filename := fmt.Sprintf("%s/%04d.jpg", dirName, j+1)
			
			err := downloadImageWithRetry(imgUrl, filename, 3)
			if err != nil {
				fmt.Printf("下载图片 %d 失败: %v\n", j+1, err)
				continue
			}
			fmt.Printf("已下载图片 %d/%d: %s\n", j+1, len(imageUrls), filename)
		}
		
		fmt.Printf("章节 %s 下载完成\n", chapter.title)
	}
	
	fmt.Printf("\n漫画《%s》下载演示完成! 所有章节保存在 %s 目录中\n", comicTitle, comicTitle)
}

// downloadSeries 下载整个漫画系列
func downloadSeries(seriesID string, startChapterID string) {
	fmt.Printf("正在下载漫画系列 %s...\n", seriesID)
	if startChapterID != "" {
		fmt.Printf("从章节 %s 开始下载\n", startChapterID)
	}
	
	// 构造目录页面URL
	tocURL := "https://www.92hm.life/book/" + seriesID
	
	// 获取目录页面
	doc, err := fetchPageWithRetry(tocURL, 3)
	if err != nil {
		fmt.Printf("获取目录页面失败: %v\n", err)
		return
	}
	
	// 提取章节链接
	chapters := extractChapterLinks(doc)
	if len(chapters) == 0 {
		fmt.Println("未找到任何章节链接")
		return
	}
	
	// 获取漫画标题
	comicTitle := extractComicTitle(doc)
	if comicTitle == "" {
		comicTitle = "comic_" + seriesID
	}
	
	// 创建漫画主目录
	err = os.MkdirAll(comicTitle, 0755)
	if err != nil {
		fmt.Printf("创建漫画主目录失败: %v\n", err)
		return
	}
	
	fmt.Printf("漫画标题: %s\n", comicTitle)
	fmt.Printf("找到 %d 个章节\n", len(chapters))
	
	// 如果指定了起始章节，则从该章节开始下载
	startIndex := 0
	if startChapterID != "" {
		found := false
		for i, chapter := range chapters {
			if chapter.id == startChapterID {
				startIndex = i
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("警告: 未找到起始章节 %s，将从头开始下载\n", startChapterID)
		} else {
			fmt.Printf("从章节 [%d/%d] 开始下载\n", startIndex+1, len(chapters))
		}
	}
	
	// 按顺序下载每个章节（从startIndex开始）
	for i := startIndex; i < len(chapters); i++ {
		chapter := chapters[i]
		// 使用更具描述性的章节目录名
		chapterDirName := fmt.Sprintf("%03d_%s", i+1, sanitizeFileName(chapter.title))
		
		fmt.Printf("\n正在下载章节 [%d/%d]: %s (%s)\n", i+1, len(chapters), chapter.title, chapter.id)
		
		// 构造章节URL
		chapterURL := "https://www.92hm.life/chapter/" + chapter.id
		
		// 获取章节页面
		doc, err := fetchPageWithRetry(chapterURL, 3)
		if err != nil {
			fmt.Printf("获取章节页面失败: %v\n", err)
			continue
		}
		
		// 提取图片链接
		imageUrls := extractImageUrls(doc)
		if len(imageUrls) == 0 {
			fmt.Println("未找到任何图片链接")
			continue
		}
		
		fmt.Printf("找到 %d 张图片\n", len(imageUrls))
		
		// 创建保存图片的目录（在漫画主目录下）
		dirName := filepath.Join(comicTitle, chapterDirName)
		err = os.MkdirAll(dirName, 0755)
		if err != nil {
			fmt.Printf("创建目录失败: %v\n", err)
			continue
		}
		
		// 下载图片
		for j, imgUrl := range imageUrls {
			// 使用4位数字编号，例如 0001.jpg, 0002.jpg 等
			filename := fmt.Sprintf("%s/%04d.jpg", dirName, j+1)
			
			err := downloadImageWithRetry(imgUrl, filename, 3)
			if err != nil {
				fmt.Printf("下载图片 %d 失败: %v\n", j+1, err)
				continue
			}
			fmt.Printf("已下载图片 %d/%d: %s\n", j+1, len(imageUrls), filename)
		}
		
		fmt.Printf("章节 %s 下载完成\n", chapter.title)
	}
	
	fmt.Printf("\n漫画《%s》下载完成! 所有章节保存在 %s 目录中\n", comicTitle, comicTitle)
}

// ChapterInfo 章节信息
type ChapterInfo struct {
	id    string
	title string
}

// extractChapterLinks 从目录页面提取章节链接
func extractChapterLinks(doc *goquery.Document) []ChapterInfo {
	var chapters []ChapterInfo
	
	// 查找章节链接
	doc.Find("a[href*='/chapter/']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && strings.Contains(href, "/chapter/") {
			// 提取章节ID
			parts := strings.Split(href, "/")
			if len(parts) >= 3 {
				chapterID := parts[len(parts)-1]
				// 检查是否为纯数字
				if _, err := strconv.Atoi(chapterID); err == nil {
					title := strings.TrimSpace(s.Text())
					if title == "" {
						title = "Chapter " + chapterID
					}
					
					// 避免重复添加
					found := false
					for _, c := range chapters {
						if c.id == chapterID {
							found = true
							break
						}
					}
					
					if !found {
						chapters = append(chapters, ChapterInfo{id: chapterID, title: title})
					}
				}
			}
		}
	})
	
	// 如果没有找到链接，尝试其他选择器
	if len(chapters) == 0 {
		doc.Find(".chapter-item a").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if exists && strings.Contains(href, "/chapter/") {
				parts := strings.Split(href, "/")
				if len(parts) >= 3 {
					chapterID := parts[len(parts)-1]
					if _, err := strconv.Atoi(chapterID); err == nil {
						title := strings.TrimSpace(s.Text())
						if title == "" {
							title = "Chapter " + chapterID
						}
						
						found := false
						for _, c := range chapters {
							if c.id == chapterID {
								found = true
								break
							}
						}
						
						if !found {
							chapters = append(chapters, ChapterInfo{id: chapterID, title: title})
						}
					}
				}
			}
		})
	}
	
	return chapters
}

// parseLocalFile 从本地HTML文件解析内容
func parseLocalFile(filePath string) (*goquery.Document, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	doc, err := goquery.NewDocumentFromReader(file)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

// fetchPageWithRetry 获取并解析网页内容，支持重试
func fetchPageWithRetry(url string, maxRetries int) (*goquery.Document, error) {
	var err error
	for i := 0; i < maxRetries; i++ {
		fmt.Printf("正在获取页面... (尝试 %d/%3d)\n", i+1, maxRetries)
		
		doc, err := fetchPage(url)
		if err == nil {
			// 检查是否获取到了有效内容
			title := doc.Find("title").Text()
			if strings.TrimSpace(title) != "" && !strings.Contains(title, "错误") {
				return doc, nil
			}
			// 如果标题为空或包含错误，可能页面内容不完整
			fmt.Println("获取到的页面内容可能不完整")
		}
		
		fmt.Printf("获取页面失败: %v\n", err)
		if i < maxRetries-1 {
			fmt.Println("等待5秒后重试...")
			time.Sleep(5 * time.Second)
		}
	}
	
	return nil, fmt.Errorf("在 %d 次尝试后仍然无法获取页面: %v", maxRetries, err)
}

// fetchPage 获取并解析网页内容
func fetchPage(url string) (*goquery.Document, error) {
	if debugMode {
		fmt.Printf("DEBUG: 正在请求URL: %s\n", url)
	}
	
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 更完整地模拟浏览器请求
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Referer", "https://www.92hm.life/")

	if debugMode {
		fmt.Printf("DEBUG: 请求头:\n")
		for key, values := range req.Header {
			for _, value := range values {
				fmt.Printf("  %s: %s\n", key, value)
			}
		}
	}

	// 创建带代理的客户端
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   60 * time.Second,
				KeepAlive: 60 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 限制重定向次数
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			if debugMode {
				fmt.Printf("DEBUG: 重定向到: %s\n", req.URL.String())
			}
			return nil
		},
	}
	
	if debugMode {
		fmt.Printf("DEBUG: 发送请求...\n")
	}
	
	resp, err := client.Do(req)
	if err != nil {
		if debugMode {
			fmt.Printf("DEBUG: 请求失败: %v\n", err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if debugMode {
		fmt.Printf("DEBUG: 响应状态码: %d\n", resp.StatusCode)
		fmt.Printf("DEBUG: 响应头:\n")
		for key, values := range resp.Header {
			for _, value := range values {
				fmt.Printf("  %s: %s\n", key, value)
			}
		}
	}

	// 检查状态码
	if resp.StatusCode != 200 {
		// 尝试读取错误响应体以提供更多调试信息
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) // 限制读取大小
		if debugMode {
			fmt.Printf("DEBUG: 错误响应体: %s\n", string(body))
		}
		return nil, fmt.Errorf("状态码错误: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 检查内容编码并相应处理
	var reader io.Reader = resp.Body
	contentEncoding := resp.Header.Get("Content-Encoding")
	if contentEncoding == "gzip" {
		if debugMode {
			fmt.Printf("DEBUG: 内容已gzip压缩，正在解压...\n")
		}
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			if debugMode {
				fmt.Printf("DEBUG: 创建gzip解压器失败: %v\n", err)
			}
			return nil, fmt.Errorf("创建gzip解压器失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	} else if contentEncoding == "br" {
		if debugMode {
			fmt.Printf("DEBUG: 内容已Brotli压缩，正在解压...\n")
		}
		reader = brotli.NewReader(resp.Body)
	}

	// 读取内容用于调试
	var content []byte
	if debugMode {
		content, err = io.ReadAll(reader)
		if err != nil {
			fmt.Printf("DEBUG: 读取响应体失败: %v\n", err)
			return nil, err
		}
		fmt.Printf("DEBUG: 响应体大小: %d 字节\n", len(content))
		reader = strings.NewReader(string(content))
	}

	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		if debugMode {
			fmt.Printf("DEBUG: 解析文档失败: %v\n", err)
		}
		return nil, err
	}

	// 检查页面标题以确认是否获取到有效内容
	title := doc.Find("title").Text()
	if debugMode {
		fmt.Printf("DEBUG: 页面标题: %s\n", title)
	}
	
	// 如果标题为空，可能是内容不完整
	if strings.TrimSpace(title) == "" {
		if debugMode {
			htmlContent, _ := doc.Html()
			fmt.Printf("DEBUG: 页面HTML内容长度: %d\n", len(htmlContent))
			if len(htmlContent) < 15000 { // 正常页面通常更大
				fmt.Printf("DEBUG: 页面内容可能不完整\n")
			}
		}
		return nil, fmt.Errorf("页面内容可能不完整")
	}

	return doc, nil
}

// extractImageUrls 从页面中提取所有图片链接
func extractImageUrls(doc *goquery.Document) []string {
	var urls []string

	// 打印页面标题以帮助调试
	title := doc.Find("title").Text()
	fmt.Printf("页面标题: %s\n", title)

	// 显示页面大小帮助调试
	content, _ := doc.Html()
	fmt.Printf("页面HTML长度: %d 字符\n", len(content))

	// 专门针对92hm.life网站的选择器
	foundCount := 0
	doc.Find("img.lazy").Each(func(i int, s *goquery.Selection) {
		imgSrc, exists := s.Attr("data-original")
		if exists && imgSrc != "" {
			imgSrc = strings.TrimSpace(imgSrc)
			
			// 处理相对链接
			if strings.HasPrefix(imgSrc, "//") {
				imgSrc = "https:" + imgSrc
			} else if strings.HasPrefix(imgSrc, "/") {
				imgSrc = "https://www.92hm.life" + imgSrc
			}
			
			urls = append(urls, imgSrc)
			foundCount++
			if foundCount <= 5 { // 只打印前5个
				fmt.Printf("找到图片 [%d]: %s\n", i+1, imgSrc)
			}
		}
	})
	
	if foundCount > 5 {
		fmt.Printf("还有 %d 张图片...\n", foundCount-5)
	}

	// 如果上面的方法没找到，尝试通用方法
	if len(urls) == 0 {
		doc.Find("img").Each(func(i int, s *goquery.Selection) {
			imgSrc, exists := s.Attr("data-original")
			if !exists {
				imgSrc, exists = s.Attr("data-src")
			}
			if !exists {
				imgSrc, exists = s.Attr("src")
			}
			
			if exists && imgSrc != "" {
				imgSrc = strings.TrimSpace(imgSrc)
				
				// 检查是否为漫画图片
				if strings.Contains(imgSrc, "upload") || strings.Contains(imgSrc, "book") || 
				   strings.Contains(imgSrc, "imgBridge") || strings.Contains(imgSrc, "imgs") ||
				   strings.HasSuffix(imgSrc, ".jpg") || strings.HasSuffix(imgSrc, ".png") || 
				   strings.HasSuffix(imgSrc, ".jpeg") || strings.Contains(imgSrc, "comic") {
				    
					// 处理相对链接
					if strings.HasPrefix(imgSrc, "//") {
						imgSrc = "https:" + imgSrc
					} else if strings.HasPrefix(imgSrc, "/") {
						imgSrc = "https://www.92hm.life" + imgSrc
					}
					
					urls = append(urls, imgSrc)
				}
			}
		})
	}

	// 最后的备选方案
	if len(urls) == 0 {
		doc.Find("div.cropped").Each(func(i int, s *goquery.Selection) {
			imgSrc, exists := s.Attr("data-src")
			if !exists {
				imgSrc, exists = s.Attr("src")
			}
			
			if exists && imgSrc != "" {
				imgSrc = strings.TrimSpace(imgSrc)
				
				// 处理相对链接
				if strings.HasPrefix(imgSrc, "//") {
					imgSrc = "https:" + imgSrc
				} else if strings.HasPrefix(imgSrc, "/") {
					imgSrc = "https://www.92hm.life" + imgSrc
				}
				
				urls = append(urls, imgSrc)
			}
		})
	}

	return urls
}

// downloadImageWithRetry 下载单个图片，支持重试
func downloadImageWithRetry(url, filename string, maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = downloadImage(url, filename)
		if err == nil {
			return nil
		}
		
		if i < maxRetries-1 {
			fmt.Printf("图片下载失败，%d秒后重试... (%d/%d)\n", 2, i+1, maxRetries)
			time.Sleep(time.Duration(2) * time.Second)
		}
	}
	
	return fmt.Errorf("在 %d 次尝试后仍然无法下载图片: %v", maxRetries, err)
}

// downloadImage 下载单个图片
func downloadImage(imageURL, filename string) error {
	// 解析URL以检查其有效性
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		return fmt.Errorf("无效的URL: %v", err)
	}

	// 创建文件
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// 创建带上下文的请求
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
	if err != nil {
		return err
	}

	// 设置用户代理
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Referer", "https://www.92hm.life/")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	// 创建带代理的客户端
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   60 * time.Second,
				KeepAlive: 60 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 60 * time.Second,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("图片下载失败，状态码: %d", resp.StatusCode)
	}

	// 检查内容是否被gzip压缩
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("创建gzip解压器失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	// 将图片写入文件
	_, err = io.Copy(file, reader)
	return err
}

// extractComicTitle 从目录页面提取漫画标题
func extractComicTitle(doc *goquery.Document) string {
	// 首先尝试查找面包屑导航中的漫画名称
	title := doc.Find(".comic-name").First().Text()
	if title == "" {
		title = doc.Find(".crumbs a").Eq(1).Text()
	}
	if title == "" {
		title = doc.Find("h1").First().Text()
	}
	if title == "" {
		title = doc.Find(".comic-title").First().Text()
	}
	if title == "" {
		title = doc.Find("title").First().Text()
		// 清理标题中的额外信息
		if idx := strings.Index(title, "-"); idx > 0 {
			title = strings.TrimSpace(title[:idx])
		}
	}
	
	// 清理标题
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "\n", "")
	title = strings.ReplaceAll(title, "\t", "")
	
	// 如果标题仍然为空，返回默认值
	if title == "" {
		return ""
	}
	
	return sanitizeFileName(title)
}

// extractChapterTitle 从章节页面提取章节标题
func extractChapterTitle(doc *goquery.Document) string {
	// 尝试多种选择器获取标题
	title := doc.Find("h1").First().Text()
	if title == "" {
		title = doc.Find(".chapter-title").First().Text()
	}
	if title == "" {
		title = doc.Find("title").First().Text()
		// 清理标题中的额外信息
		if idx := strings.Index(title, "-"); idx > 0 {
			title = strings.TrimSpace(title[:idx])
		}
	}
	
	// 清理标题
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "\n", "")
	title = strings.ReplaceAll(title, "\t", "")
	
	return sanitizeFileName(title)
}

// sanitizeFileName 清理文件名中的非法字符
func sanitizeFileName(filename string) string {
	// 替换非法字符
	illegalChars := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"}
	for _, char := range illegalChars {
		filename = strings.ReplaceAll(filename, char, "_")
	}
	
	// 限制长度
	if len(filename) > 100 {
		filename = filename[:100]
	}
	
	return strings.TrimSpace(filename)
}