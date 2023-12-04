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
	thresholdSizeLarge    = 5 * 1024 * 1024
	thresholdSizeSuperBig = 3 * 1024 * 1024
	thresholdSizeMedium   = 500 * 1024
	thresholdSizeSmall    = 10 * 1024
	bufferSize            = 8192
)

var mutex sync.Mutex

func main() {
	var inputURL, outputDir string
	var keepPartition, removePartition bool

	flag.StringVar(&inputURL, "url", "", "URL to download")
	flag.StringVar(&outputDir, "output", "output", "Output directory for partitions and merged file")
	flag.BoolVar(&keepPartition, "keep-partition", false, "Keep partition files after merging")
	flag.BoolVar(&removePartition, "remove-partition", false, "Remove partition directory after merging")

	flag.Parse()

	if inputURL == "" {
		fmt.Println("Usage: go run main.go -url <URL> [--keep-partition|--remove-partition] -output <Directory>")
		fmt.Println("Example Usage: go run main.go -url https://example.com/file.zip -output /outputDir")
		return
	}

	if err := downloadAndSplit(inputURL, outputDir, keepPartition, removePartition); err != nil {
		log.Fatal("Error:", err)
	}
}

func downloadAndSplit(url, outputDir string, keepPartition, removePartition bool) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to initiate download: %v", err)
	}
	defer resp.Body.Close()

	baseFileName := getBaseFileName(url)
	outputFilePath := filepath.Join(outputDir, "final", baseFileName)

	if resp.ContentLength < thresholdSizeSmall {
		fmt.Println("Small file. No need to split.")
		return downloadToFile(url, outputFilePath)
	}

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	numPartitions := calculateNumPartitions(resp.ContentLength)

	fmt.Printf("Downloading %d partitions...\n", numPartitions)

	downloadSize := resp.ContentLength / int64(numPartitions)
	allDone := make(chan bool, numPartitions)
	partitionDir := filepath.Join(outputDir, "partitions")
	finalDir := filepath.Join(outputDir, "final")

	if err := os.MkdirAll(partitionDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create partition directory: %v", err)
	}

	if err := os.MkdirAll(finalDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create final directory: %v", err)
	}

	var wg sync.WaitGroup

	for i := 0; i < numPartitions; i++ {
		wg.Add(1)
		partitionFileName := fmt.Sprintf("part%d_%s", i+1, baseFileName)
		partitionPath := filepath.Join(partitionDir, partitionFileName)
		startRange := int64(i) * downloadSize
		endRange := startRange + downloadSize - 1

		if i == numPartitions-1 {
			endRange = resp.ContentLength - 1
		}

		go func(partitionNum int, start, end int64, path string, done chan bool) {
			defer wg.Done()
			if err := downloadRange(url, path, start, end, partitionNum, numPartitions, done); err != nil {
				log.Printf("Error downloading partition %d: %v", partitionNum, err)
			}
		}(i+1, startRange, endRange, partitionPath, allDone)
	}

	go func() {
		wg.Wait()
		close(allDone)
	}()

	for range allDone {
	}

	fmt.Println("All partitions downloaded. Merging...")

	if err := mergePartitions(partitionDir, finalDir, outputFilePath, keepPartition, removePartition); err != nil {
		log.Printf("Error merging partitions: %v", err)
	} else {
		fmt.Println("Download and merge complete.")
	}

	return nil
}

// calculateNumPartitions calculates the number of partitions based on the file size.
func calculateNumPartitions(fileSize int64) int {
	switch {
	case fileSize > thresholdSizeSuperBig:
		return 10
	case fileSize > thresholdSizeLarge:
		return 5
	case fileSize > thresholdSizeMedium:
		return 2
	default:
		return 1
	}
}

func downloadRange(url, outputPath string, start, end int64, partitionNum, totalPartitions int, done chan bool) error {
	client := http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to perform HTTP request: %v", err)
	}
	defer resp.Body.Close()

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer func() {
		file.Close()
		fmt.Println()
		done <- true
	}()

	totalBytes := end - start
	downloadedBytes := int64(0)
	buffer := make([]byte, bufferSize)

	mutex.Lock()
	defer mutex.Unlock()
	progressBarWidth := 50

	for {
		n, err := resp.Body.Read(buffer)
		if err != nil && err != io.EOF {
			log.Printf("Error reading response body: %v", err)
			return err
		}
		if n == 0 {
			break
		}

		downloadedBytes += int64(n)
		progressBar := int((float64(downloadedBytes) / float64(totalBytes)) * float64(progressBarWidth))

		// Fake delay to simulate slower progress update
		time.Sleep(50 * time.Millisecond)

		fmt.Printf("(partition %d) [%s%s] %.2f%%\r", partitionNum, strings.Repeat("=", progressBar), strings.Repeat(" ", progressBarWidth-progressBar), float64(downloadedBytes)/float64(totalBytes)*100.0)

		_, err = file.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("failed to write to output file: %v", err)
		}

		file.Sync()
	}

	return nil
}

func mergePartitions(partitionDir, finalDir, outputFilePath string, keepPartition, removePartition bool) error {
	files, err := filepath.Glob(filepath.Join(partitionDir, "*"))
	if err != nil {
		return fmt.Errorf("failed to list partition files: %v", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no partition files found")
	}

	mergedFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create merged file: %v", err)
	}
	defer mergedFile.Close()

	// Sort files to ensure correct order
	sort.Strings(files)

	var previousBaseName string

	for i, file := range files {
		baseName := getBaseFileName(file)
		ext := filepath.Ext(file)

		if i > 0 && baseName == previousBaseName {
			// Partisi dengan nama yang sama, gabungkan hanya jika tipe file sama
			previousExt := filepath.Ext(files[i-1])
			if ext != previousExt {
				return fmt.Errorf("partitions with the same base name but different file types cannot be merged")
			}

			if err := mergeFile(mergedFile, file); err != nil {
				return fmt.Errorf("failed to merge partition %d: %v", i+1, err)
			}

			// Optionally, keep the partition files after merging
			if keepPartition {
				finalFileName := fmt.Sprintf("part%d_%s", i+1, filepath.Base(file))
				finalPath := filepath.Join(finalDir, finalFileName)
				if err := os.Rename(file, finalPath); err != nil {
					return fmt.Errorf("failed to rename partition file: %v", err)
				}
			}
		} else {
			// Partisi baru dengan base name yang berbeda, langsung tambahkan ke merged file
			if err := mergeFile(mergedFile, file); err != nil {
				return fmt.Errorf("failed to merge partition %d: %v", i+1, err)
			}

			// Optionally, keep the partition files after merging
			if keepPartition {
				finalFileName := fmt.Sprintf("part%d_%s", i+1, filepath.Base(file))
				finalPath := filepath.Join(finalDir, finalFileName)
				if err := os.Rename(file, finalPath); err != nil {
					return fmt.Errorf("failed to rename partition file: %v", err)
				}
			}
		}

		previousBaseName = baseName
	}

	// Optionally, remove the partition directory after merging
	if removePartition {
		if err := os.RemoveAll(partitionDir); err != nil {
			return fmt.Errorf("failed to remove partition directory: %v", err)
		}
	}

	fmt.Println("Partitions merged.")
	return nil
}

func mergeFile(dst *os.File, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open partition file: %v", err)
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
		return fmt.Errorf("failed to initiate download: %v", err)
	}
	defer resp.Body.Close()

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy response body: %v", err)
	}

	fmt.Println("Download complete.")
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
