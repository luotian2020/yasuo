package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	InputDir       string `json:"InputDir"`
	OutputDir      string `json:"OutputDir"`
	InitialQuality int    `json:"InitialQuality"`
}

// 从 JPEG 文件中提取 APP1(EXIF) 段
func extractExif(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil
	}

	offset := 2
	for offset+4 < len(data) {
		if data[offset] != 0xFF {
			break
		}
		marker := data[offset+1]
		size := int(data[offset+2])<<8 | int(data[offset+3])
		if marker == 0xE1 { // APP1
			return data[offset+4 : offset+2+size]
		}
		offset += 2 + size
	}
	return nil
}

// 修正方向
func fixOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return flipHorizontal(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipVertical(img)
	case 5:
		return flipHorizontal(rotate270(img))
	case 6:
		return rotate90(img)
	case 7:
		return flipHorizontal(rotate90(img))
	case 8:
		return rotate270(img)
	default:
		return img
	}
}

func rotate90(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dst.Set(b.Max.Y-y-1, x, img.At(x, y))
		}
	}
	return dst
}

func rotate180(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dst.Set(b.Max.X-x-1, b.Max.Y-y-1, img.At(x, y))
		}
	}
	return dst
}

func rotate270(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dst.Set(y, b.Max.X-x-1, img.At(x, y))
		}
	}
	return dst
}

func flipHorizontal(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dst.Set(b.Max.X-x-1, y, img.At(x, y))
		}
	}
	return dst
}

func flipVertical(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			dst.Set(x, b.Max.Y-y-1, img.At(x, y))
		}
	}
	return dst
}

func main() {
	// 读取 config.json
	data, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic("读取 config.json 失败: " + err.Error())
	}

	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		panic("解析 config.json 失败: " + err.Error())
	}

	os.MkdirAll(cfg.OutputDir, os.ModePerm)

	fmt.Println("开始压缩并保留 EXIF...")

	filepath.Walk(cfg.InputDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		lower := strings.ToLower(info.Name())
		if !strings.HasSuffix(lower, ".jpg") && !strings.HasSuffix(lower, ".jpeg") {
			return nil
		}

		// 读取原始文件字节
		origBytes, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Println("读取文件失败:", path)
			return nil
		}

		// 提取 EXIF
		exifBytes := extractExif(origBytes)

		// 解码图片
		imgFile, err := os.Open(path)
		if err != nil {
			fmt.Println("打开文件失败:", path)
			return nil
		}
		img, _, err := image.Decode(imgFile)
		imgFile.Close()
		if err != nil {
			fmt.Println("解码失败:", path)
			return nil
		}

		// 修正方向
		orientation := 1
		if len(exifBytes) >= 2 {
			// 简单尝试解析 Orientation 字段
			orientation = int(exifBytes[len(exifBytes)-1])
		}
		img = fixOrientation(img, orientation)

		// 压缩到内存
		var buf bytes.Buffer
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: cfg.InitialQuality})
		if err != nil {
			fmt.Println("压缩失败:", path)
			return nil
		}

		// 写入输出文件
		outPath := filepath.Join(cfg.OutputDir, info.Name())
		outFile, err := os.Create(outPath)
		if err != nil {
			fmt.Println("创建输出文件失败:", outPath)
			return nil
		}
		defer outFile.Close()

		if exifBytes != nil {
			outFile.Write([]byte{0xFF, 0xD8})           // SOI
			outFile.Write([]byte{0xFF, 0xE1})           // APP1
			length := uint16(len(exifBytes) + 2)
			outFile.Write([]byte{byte(length >> 8), byte(length & 0xFF)})
			outFile.Write(exifBytes)
			outFile.Write(buf.Bytes()[2:]) // 跳过原 JPEG SOI
		} else {
			outFile.Write(buf.Bytes())
		}

		fmt.Println("压缩成功:", outPath)
		return nil
	})

	fmt.Println("完成")
	fmt.Println("按回车退出...")
	fmt.Scanln()
}
