package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// Definisikan Konstanta threshold ukuran file untuk partisi
	thresholdSizeLarge    = 5 * 1024 * 1024 // 5 MB (super besar)
	thresholdSizeSuperBig = 3 * 1024 * 1024 // 3 MB (besar)
	thresholdSizeMedium   = 500 * 1024      // 500 KB (medium)
	thresholdSizeSmall    = 10 * 1024       // 10 KB (kecil)
	bufferSize            = 8192
)

func main() {
	var inputURL, outputDir string
	var keepPartition, removePartition bool

	// Gunakan flag untuk mengambil argumen dari baris perintah
	flag.StringVar(&inputURL, "url", "", "URL untuk diunduh")
	flag.StringVar(&outputDir, "output", "output", "Direktori output untuk partisi dan file yang digabungkan")
	flag.BoolVar(&keepPartition, "keep-partition", false, "Biarkan file partisi setelah digabungkan")
	flag.BoolVar(&removePartition, "remove-partition", false, "Hapus direktori partisi setelah digabungkan")

	flag.Parse()

	// Periksa apakah argumen URL diberikan
	if inputURL == "" {
		fmt.Println("Penggunaan: go run main.go -url <URL> [--keep-partition|--remove-partition]")
		return
	}

	if err := downloadAndSplit(inputURL, outputDir, keepPartition, removePartition); err != nil {
		log.Fatal("Error:", err)
	}
}

func downloadAndSplit(url, outputDir string, keepPartition, removePartition bool) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("gagal memulai pengunduhan: %v", err)
	}
	defer resp.Body.Close()

	baseFileName := getBaseFileName(url)
	outputFilePath := filepath.Join(outputDir, "final", baseFileName)

	if resp.ContentLength < thresholdSizeSmall {
		fmt.Println("File kecil. Tidak perlu dipartisi.")
		return downloadToFile(url, outputFilePath)
	}

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("gagal membuat direktori output: %v", err)
	}

	numPartitions := 1
	fileSize := resp.ContentLength

	switch {
	case fileSize > thresholdSizeSuperBig:
		numPartitions = 10
	case fileSize > thresholdSizeLarge:
		numPartitions = 5
	case fileSize > thresholdSizeMedium:
		numPartitions = 2
	}

	fmt.Printf("Mengunduh %d partisi...\n", numPartitions)

	downloadSize := fileSize / int64(numPartitions)
	allDone := make(chan bool, numPartitions)
	partitionDir := filepath.Join(outputDir, "partisi")
	finalDir := filepath.Join(outputDir, "final")

	if err := os.MkdirAll(partitionDir, os.ModePerm); err != nil {
		return fmt.Errorf("gagal membuat direktori partisi: %v", err)
	}

	if err := os.MkdirAll(finalDir, os.ModePerm); err != nil {
		return fmt.Errorf("gagal membuat direktori final: %v", err)
	}

	var wg sync.WaitGroup

	for i := 0; i < numPartitions; i++ {
		wg.Add(1)
		partitionFileName := fmt.Sprintf("part%d_%s", i+1, baseFileName)
		partitionPath := filepath.Join(partitionDir, partitionFileName)
		startRange := int64(i) * downloadSize
		endRange := startRange + downloadSize - 1 // Sesuaikan akhiran agar tidak tumpang tindih

		if i == numPartitions-1 {
			endRange = fileSize - 1
		}

		go func(partitionNum int, start, end int64, path string, done chan bool) {
			defer wg.Done()
			if err := downloadRange(url, path, start, end, partitionNum, numPartitions, done); err != nil {
				log.Printf("Error mengunduh partisi %d: %v", partitionNum, err)
			}
		}(i+1, startRange, endRange, partitionPath, allDone)
	}

	go func() {
		wg.Wait()
		close(allDone)
	}()

	for range allDone {
	}

	fmt.Println("Semua partisi diunduh. Menggabungkan...")

	if err := mergePartitions(partitionDir, finalDir, outputFilePath, keepPartition, removePartition); err != nil {
		log.Printf("Error menggabungkan partisi: %v", err)
	} else {
		fmt.Println("Pengunduhan dan penggabungan selesai.")
	}

	return nil
}

func downloadRange(url, outputPath string, start, end int64, partitionNum, totalPartitions int, done chan bool) error {
	client := http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("gagal membuat permintaan HTTP: %v", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gagal melakukan permintaan HTTP: %v", err)
	}
	defer resp.Body.Close()

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("gagal membuat file output: %v", err)
	}
	defer file.Close()

	totalBytes := end - start
	downloadedBytes := int64(0)
	buffer := make([]byte, 1024)

	defer func() {
		fmt.Println()
		done <- true
	}()

	progressBarWidth := 50

	for {
		n, err := resp.Body.Read(buffer)
		if err != nil && err != io.EOF {
			log.Printf("Error membaca isi tubuh respons: %v", err)
			return err
		}
		if n == 0 {
			break
		}

		downloadedBytes += int64(n)
		progress := (float64(downloadedBytes) / float64(totalBytes)) * 100.0
		progressBar := int((float64(downloadedBytes) / float64(totalBytes)) * float64(progressBarWidth))

		// Pemalsuan penundaan untuk mensimulasikan pembaruan progres yang lebih lambat
		time.Sleep(50 * time.Millisecond)

		fmt.Printf("(partisi %d) [%s%s] %.2f%%\r", partitionNum, strings.Repeat("=", progressBar), strings.Repeat(" ", progressBarWidth-progressBar), progress)

		file.Sync()
	}

	return nil
}

func mergePartitions(partitionDir, finalDir, outputFilePath string, keepPartition, removePartition bool) error {
	files, err := filepath.Glob(filepath.Join(partitionDir, "*"))
	if err != nil {
		return fmt.Errorf("gagal melisting file partisi: %v", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("tidak ada file partisi yang ditemukan")
	}

	mergedFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("gagal membuat file yang digabungkan: %v", err)
	}
	defer mergedFile.Close()

	// Urutkan file untuk memastikan urutan yang benar
	sort.Strings(files)

	for i, file := range files {
		if err := mergeFile(mergedFile, file); err != nil {
			return fmt.Errorf("gagal menggabungkan partisi %d: %v", i+1, err)
		}

		// Secara opsional, biarkan file partisi setelah digabungkan
		if keepPartition {
			finalFileName := fmt.Sprintf("part%d_%s", i+1, filepath.Base(file))
			finalPath := filepath.Join(finalDir, finalFileName)
			if err := os.Rename(file, finalPath); err != nil {
				return fmt.Errorf("gagal mengubah nama file partisi: %v", err)
			}
		}
	}

	// Secara opsional, hapus direktori partisi setelah digabungkan
	if removePartition {
		if err := os.RemoveAll(partitionDir); err != nil {
			return fmt.Errorf("gagal menghapus direktori partisi: %v", err)
		}
	}

	fmt.Println("Partisi digabungkan.")
	return nil
}

func mergeFile(dst *os.File, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("gagal membuka file partisi: %v", err)
	}
	defer src.Close()

	switch filepath.Ext(srcPath) {
	case ".zip":
		return mergeZip(dst, src)
	case ".tar", ".tar.gz", ".tgz":
		return mergeTar(dst, src)
	}

	_, err = io.Copy(dst, src)
	return err
}

func mergeZip(dst *os.File, src *os.File) error {
	r, err := zip.OpenReader(src.Name())
	if err != nil {
		return err
	}
	defer r.Close()

	for _, file := range r.File {
		rc, err := file.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		_, err = io.Copy(dst, rc)
		if err != nil {
			return err
		}
	}

	return nil
}

func mergeTar(dst *os.File, src *os.File) error {
	var reader io.Reader
	if filepath.Ext(src.Name()) == ".gz" {
		gr, err := gzip.NewReader(src)
		if err != nil {
			return err
		}
		defer gr.Close()
		reader = gr
	} else {
		reader = src
	}

	tr := tar.NewReader(reader)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg {
			_, err := io.Copy(dst, tr)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func downloadToFile(url, outputPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("gagal memulai pengunduhan: %v", err)
	}
	defer resp.Body.Close()

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("gagal membuat file output: %v", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("gagal menyalin tubuh respons: %v", err)
	}

	fmt.Println("Pengunduhan selesai.")
	return nil
}

func getProgressBar(progress float64) string {
	const width = 50
	numBars := int(progress / (100.0 / width))
	return "[" + strings.Repeat("=", numBars) + strings.Repeat(" ", width-numBars) + "]"
}

func getBaseFileName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "output"
	}

	return filepath.Base(u.Path)
}
