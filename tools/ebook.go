package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使用方法:")
		fmt.Println("  打包漫画为电子书: ebook <漫画目录>")
		fmt.Println("  例如: ebook '秘密教学'")
		return
	}

	comicDir := os.Args[1]
	
	// 检查漫画目录是否存在
	if _, err := os.Stat(comicDir); os.IsNotExist(err) {
		fmt.Printf("错误: 漫画目录 '%s' 不存在\n", comicDir)
		return
	}

	// 创建电子书
	err := createEbook(comicDir)
	if err != nil {
		fmt.Printf("创建电子书失败: %v\n", err)
		return
	}
	
	fmt.Printf("成功创建电子书: %s.cbz\n", comicDir)
}

// createEbook 将漫画目录打包成电子书
func createEbook(comicDir string) error {
	// 创建输出文件
	outputFile := comicDir + ".cbz"
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer file.Close()

	// 创建zip写入器
	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	// 获取漫画信息
	comicInfo, err := getComicInfo(comicDir)
	if err != nil {
		return fmt.Errorf("获取漫画信息失败: %v", err)
	}

	// 添加漫画信息文件
	err = addComicInfoToZip(zipWriter, comicInfo)
	if err != nil {
		return fmt.Errorf("添加漫画信息失败: %v", err)
	}

	// 添加目录HTML文件
	err = addTOCFileToZip(zipWriter, comicInfo)
	if err != nil {
		return fmt.Errorf("添加目录文件失败: %v", err)
	}

	// 添加所有章节图片
	err = addChaptersToZip(zipWriter, comicDir, comicInfo)
	if err != nil {
		return fmt.Errorf("添加章节图片失败: %v", err)
	}

	return nil
}

// ComicInfo 漫画信息结构
type ComicInfo struct {
	Title    string     `json:"title"`
	Chapters []Chapter  `json:"chapters"`
}

// Chapter 章节信息结构
type Chapter struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	DirName   string `json:"dir_name"`
	ImageCount int   `json:"image_count"`
	StartPage int   `json:"start_page"`
}

// getComicInfo 获取漫画信息
func getComicInfo(comicDir string) (ComicInfo, error) {
	var comicInfo ComicInfo
	comicInfo.Title = filepath.Base(comicDir)

	// 获取所有章节目录
	entries, err := os.ReadDir(comicDir)
	if err != nil {
		return comicInfo, err
	}

	pageCounter := 1
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		chapterDir := filepath.Join(comicDir, entry.Name())
		chapterName := entry.Name()
		
		// 获取章节中的图片数量
		imageCount, err := countImages(chapterDir)
		if err != nil {
			continue
		}

		// 提取章节ID和标题
		var chapterID, chapterTitle string
		parts := strings.SplitN(chapterName, "_", 2)
		if len(parts) == 2 {
			chapterID = strings.TrimLeft(parts[0], "0") // 移除前导零
			chapterTitle = parts[1]
		} else {
			chapterTitle = chapterName
			chapterID = chapterName
		}

		chapter := Chapter{
			ID:         chapterID,
			Title:      chapterTitle,
			DirName:    chapterName,
			ImageCount: imageCount,
			StartPage:  pageCounter,
		}

		comicInfo.Chapters = append(comicInfo.Chapters, chapter)
		pageCounter += imageCount
	}

	// 按章节ID排序
	sort.Slice(comicInfo.Chapters, func(i, j int) bool {
		return comicInfo.Chapters[i].ID < comicInfo.Chapters[j].ID
	})

	return comicInfo, nil
}

// countImages 计算目录中的图片数量
func countImages(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") ||
		   strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".gif") {
			count++
		}
	}

	return count, nil
}

// addComicInfoToZip 添加漫画信息到zip
func addComicInfoToZip(zipWriter *zip.Writer, comicInfo ComicInfo) error {
	// 创建comic.json文件
	jsonData, err := json.MarshalIndent(comicInfo, "", "  ")
	if err != nil {
		return err
	}

	// 添加到zip
	writer, err := zipWriter.Create("comic.json")
	if err != nil {
		return err
	}

	_, err = writer.Write(jsonData)
	return err
}

// addTOCFileToZip 添加目录HTML文件到zip
func addTOCFileToZip(zipWriter *zip.Writer, comicInfo ComicInfo) error {
	tocTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>{{.Title}} - 目录</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        h1 { color: #333; }
        ul { list-style-type: none; padding: 0; }
        li { margin: 10px 0; padding: 10px; border: 1px solid #ddd; border-radius: 5px; }
        a { text-decoration: none; color: #007bff; }
        a:hover { text-decoration: underline; }
        .chapter-info { color: #666; font-size: 0.9em; }
    </style>
</head>
<body>
    <h1>{{.Title}}</h1>
    <h2>目录</h2>
    <ul>
        {{range .Chapters}}
        <li>
            <a href="{{.DirName}}/0001.jpg">{{.Title}}</a>
            <div class="chapter-info">{{.ImageCount}} 页</div>
        </li>
        {{end}}
    </ul>
</body>
</html>
`

	tmpl, err := template.New("toc").Parse(tocTemplate)
	if err != nil {
		return err
	}

	writer, err := zipWriter.Create("toc.html")
	if err != nil {
		return err
	}

	return tmpl.Execute(writer, comicInfo)
}

// addChaptersToZip 添加所有章节到zip
func addChaptersToZip(zipWriter *zip.Writer, comicDir string, comicInfo ComicInfo) error {
	for _, chapter := range comicInfo.Chapters {
		chapterDir := filepath.Join(comicDir, chapter.DirName)
		
		// 获取章节中的所有图片
		images, err := getImages(chapterDir)
		if err != nil {
			return err
		}

		// 按顺序添加图片到zip
		for _, image := range images {
			imagePath := filepath.Join(chapterDir, image.Name())
			zipPath := filepath.Join(chapter.DirName, image.Name())
			
			err := addFileToZip(zipWriter, imagePath, zipPath)
			if err != nil {
				return fmt.Errorf("添加图片失败 %s: %v", imagePath, err)
			}
		}
	}

	return nil
}

// getImages 获取目录中的所有图片文件
func getImages(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var images []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") ||
		   strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".gif") {
			images = append(images, entry)
		}
	}

	// 按文件名排序
	sort.Slice(images, func(i, j int) bool {
		return images[i].Name() < images[j].Name()
	})

	return images, nil
}

// addFileToZip 将文件添加到zip归档
func addFileToZip(zipWriter *zip.Writer, filePath, zipPath string) error {
	// 打开要添加的文件
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 获取文件信息
	info, err := file.Stat()
	if err != nil {
		return err
	}

	// 创建zip文件头
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath

	// 创建zip文件写入器
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	// 复制文件内容
	_, err = io.Copy(writer, file)
	return err
}