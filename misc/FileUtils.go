package misc

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func CreateDirIfNotExists(dirPath string) error {
	// 直接创建目录，包括所有父目录
	err := os.MkdirAll(dirPath, 0755) // 0755: rwxr-xr-x
	if err != nil {
		return fmt.Errorf("创建目录失败 '%s': %w", dirPath, err)
	}
	return nil
}

func AppendLogBasic(filename, message string) error {
	// 以追加模式打开文件，如果不存在则创建
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// 添加时间戳
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)

	// 写入文件
	_, err = file.WriteString(logEntry)
	return err
}

func CreateDir(dirPath string) error {
	err := os.MkdirAll(dirPath, 0755) // 0755: rwxr-xr-x
	if err != nil {
		return fmt.Errorf("创建目录失败 '%s': %w", dirPath, err)
	}
	return nil
}

// CopyDir 复制目录
func CopyDir(src, dst string) error {
	// 清理路径
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	// Guard against copying a directory onto itself.
	if absSrc, err1 := filepath.Abs(src); err1 == nil {
		if absDst, err2 := filepath.Abs(dst); err2 == nil && absSrc == absDst {
			return nil
		}
	}

	// 获取源目录信息
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("无法读取源目录: %w", err)
	}

	if !srcInfo.IsDir() {
		return fmt.Errorf("源路径不是目录: %s", src)
	}

	// 创建目标目录
	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 读取源目录内容
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("读取源目录失败: %w", err)
	}

	// 遍历并复制每个条目
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// 递归复制子目录
			err = CopyDir(srcPath, dstPath)
			if err != nil {
				continue
			}
		} else {
			// 复制文件
			err = CopyFile(srcPath, dstPath)
			if err != nil {
				continue
			}
		}
	}

	return nil
}

// CopyFile 复制单个文件
func CopyFile(src, dst string) error {
	// Guard against copying a file onto itself.
	if absSrc, err1 := filepath.Abs(src); err1 == nil {
		if absDst, err2 := filepath.Abs(dst); err2 == nil && absSrc == absDst {
			return nil
		}
	}

	// 打开源文件
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	// 获取源文件信息
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("获取源文件信息失败: %w", err)
	}

	// 创建目标文件
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer dstFile.Close()

	// 复制文件内容
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("复制文件内容失败: %w", err)
	}

	// 保持文件权限
	err = os.Chmod(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("设置文件权限失败: %w", err)
	}

	return nil
}

func ZipDirectory(sourceDir, targetZip string) error {
	// 创建 zip 文件
	zipFile, err := os.Create(targetZip)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	// 创建 zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 遍历目录
	return filepath.Walk(sourceDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 创建 zip 文件中的文件头
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// 设置文件头中的文件名（相对路径）
		relPath, err := filepath.Rel(sourceDir, filePath)
		if err != nil {
			return err
		}

		// 确保目录以 / 结尾
		if info.IsDir() {
			relPath += "/"
		}

		header.Name = filepath.ToSlash(relPath) // 确保使用正斜杠

		// 设置压缩方法
		header.Method = zip.Deflate

		// 如果是目录，只需要创建目录头
		if info.IsDir() {
			_, err = zipWriter.CreateHeader(header)
			return err
		}

		// 如果是文件，创建文件并写入内容
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// 打开源文件
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}

		// 复制文件内容到 zip
		_, err = io.Copy(writer, file)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
