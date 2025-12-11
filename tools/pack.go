package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使用方法:")
		fmt.Println("  打包单个章节: pack chapter_16124")
		fmt.Println("  批量打包章节: pack chapter_*")
		fmt.Println("  打包并指定输出目录: pack -o /path/to/output chapter_*")
		return
	}

	// 解析命令行参数
	outputDir := "."
	args := os.Args[1:]
	
	if args[0] == "-o" && len(args) >= 3 {
		outputDir = args[1]
		args = args[2:]
	}

	// 处理通配符模式
	pattern := args[0]
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		// 批量处理模式
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Printf("解析模式失败: %v\n", err)
			return
		}
		
		for _, match := range matches {
			if isDirectory(match) {
				err := packChapter(match, outputDir)
				if err != nil {
					fmt.Printf("打包章节 %s 失败: %v\n", match, err)
				} else {
					fmt.Printf("成功打包章节 %s\n", match)
				}
			}
		}
	} else {
		// 单个章节模式
		err := packChapter(pattern, outputDir)
		if err != nil {
			fmt.Printf("打包章节失败: %v\n", err)
			return
		}
		fmt.Printf("成功打包章节 %s\n", pattern)
	}
}

// packChapter 将单个章节打包成CBZ文件
func packChapter(chapterDir, outputDir string) error {
	// 检查章节目录是否存在
	if !isDirectory(chapterDir) {
		return fmt.Errorf("章节目录不存在: %s", chapterDir)
	}

	// 检查输出目录是否存在，如果不存在则创建
	if !isDirectory(outputDir) {
		err := os.MkdirAll(outputDir, 0755)
		if err != nil {
			return fmt.Errorf("创建输出目录失败: %v", err)
		}
	}

	// 获取章节名称
	chapterName := filepath.Base(chapterDir)
	
	// 创建输出文件
	outputFile := filepath.Join(outputDir, chapterName+".cbz")
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer file.Close()

	// 创建zip写入器
	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	// 获取所有图片文件
	files, err := getImageFiles(chapterDir)
	if err != nil {
		return fmt.Errorf("获取图片文件失败: %v", err)
	}

	// 按顺序添加文件到zip
	for _, fileInfo := range files {
		err := addFileToZip(zipWriter, filepath.Join(chapterDir, fileInfo.Name()), fileInfo.Name())
		if err != nil {
			return fmt.Errorf("添加文件到zip失败: %v", err)
		}
	}

	return nil
}

// getImageFiles 获取目录中的所有图片文件并排序
func getImageFiles(dir string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		// 检查是否为图片文件
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") ||
		   strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".gif") {
			files = append(files, info)
		}
	}

	// 按文件名排序
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	return files, nil
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

// isDirectory 检查路径是否为目录
func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}