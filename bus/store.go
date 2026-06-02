package bus

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxDirSize is the rolling storage quota for ./files (overridable via env).
var maxDirSize int64 = 2 * 1024 * 1024 * 1024 // 2 GB

// Message represents the JSON structure of a file/text bubble in the timeline.
type Message struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Filename string `json:"filename,omitempty"`
	Content  string `json:"content,omitempty"`
	Size     string `json:"size,omitempty"`
	Time     string `json:"time"`
	Pinned   bool   `json:"pinned"`
	Device   string `json:"device"`
}

// ActionPayload is the JSON structure for pin/unpin/delete operations.
type ActionPayload struct {
	ID     string `json:"id"`
	Action string `json:"action"` // "pin", "unpin", "delete"
}

// getChannelDir extracts the channel query param and returns channel name and safe target directory.
func getChannelDir(r *http.Request) (string, string) {
	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	if channel != "" {
		channel = strings.ReplaceAll(channel, "/", "")
		channel = strings.ReplaceAll(channel, "\\", "")
		channel = strings.ReplaceAll(channel, "..", "")
		return channel, filepath.Join(filesDir, channel)
	}
	return "", filesDir
}

// getSafeFilePath validates and returns a clean, safe path to a file inside files/.
func getSafeFilePath(id string) (string, error) {
	// Prevent path traversal
	if strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid file ID")
	}

	safePath := filepath.Clean(filepath.Join(filesDir, id))
	absFilesDir, err := filepath.Abs(filesDir)
	if err != nil {
		return "", err
	}
	absSafePath, err := filepath.Abs(safePath)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absSafePath, absFilesDir) {
		return "", fmt.Errorf("path traversal detected")
	}

	return safePath, nil
}

// parseFilename parses metadata out of the actual filesystem filename.
func parseFilename(filename string, targetDir string) (*Message, error) {
	// Standard: [pinned_]?[timestamp]_[device]_[original_name] or legacy: [pinned_]?[timestamp]_[original_name]
	msg := &Message{
		ID:     filename,
		Device: "pc", // default
	}

	workingName := filename
	if strings.HasPrefix(workingName, "pinned_") {
		msg.Pinned = true
		workingName = strings.TrimPrefix(workingName, "pinned_")
	}

	// Split by first underscore to get timestamp
	tsStr, rest, ok := strings.Cut(workingName, "_")
	if !ok {
		return nil, fmt.Errorf("invalid filename pattern")
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp")
	}

	msg.Time = time.Unix(ts, 0).Format("2006-01-02 15:04:05")

	// Check if there is a device tag
	origName := rest
	if possibleDevice, after, ok := strings.Cut(rest, "_"); ok {
		if possibleDevice == "pc" || possibleDevice == "mobile" || possibleDevice == "ai" || possibleDevice == "web" {
			msg.Device = possibleDevice
			origName = after
		}
	}

	if origName == "text.txt" {
		msg.Type = "text"
		filePath := filepath.Join(targetDir, filename)
		info, err := os.Stat(filePath)
		if err != nil {
			msg.Content = "[Error: failed to read text message info]"
		} else {
			fileSize := info.Size()
			const limit = 10 * 1024 // 10 KB
			if fileSize <= limit {
				contentBytes, err := os.ReadFile(filePath)
				if err != nil {
					msg.Content = "[Error: failed to read text message]"
				} else {
					msg.Content = string(contentBytes)
				}
			} else {
				f, err := os.Open(filePath)
				if err != nil {
					msg.Content = "[Error: failed to open text message]"
				} else {
					defer f.Close()
					buf := make([]byte, limit)
					n, err := f.Read(buf)
					if err != nil && err != io.EOF {
						msg.Content = "[Error: failed to read partial text message]"
					} else {
						msg.Content = string(buf[:n]) + "\n... [内容过长已截断，请下载完整文件]"
					}
				}
			}
		}
	} else {
		msg.Type = "file"
		msg.Filename = origName
		// Get size
		filePath := filepath.Join(targetDir, filename)
		info, err := os.Stat(filePath)
		if err == nil {
			msg.Size = formatSize(info.Size())
		} else {
			msg.Size = "Unknown Size"
		}
	}

	return msg, nil
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func getTimestampFromID(id string) int64 {
	name := filepath.Base(id)
	name = strings.TrimPrefix(name, "pinned_")
	tsStr, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0
	}
	ts, _ := strconv.ParseInt(tsStr, 10, 64)
	return ts
}

// cleanupOldFiles traverses filesDir and removes oldest unpinned files if total exceeds limit.
func cleanupOldFiles() {
	type fileInfo struct {
		path string
		name string
		size int64
		ts   int64
	}

	var totalSize int64
	unpinnedFiles := make([]fileInfo, 0)

	err := filepath.Walk(filesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		totalSize += info.Size()
		name := filepath.Base(path)
		if !strings.HasPrefix(name, "pinned_") {
			ts := getTimestampFromID(name)
			unpinnedFiles = append(unpinnedFiles, fileInfo{
				path: path,
				name: name,
				size: info.Size(),
				ts:   ts,
			})
		}
		return nil
	})

	if err != nil {
		log.Printf("Cleanup warning: failed to walk files directory: %v", err)
	}

	log.Printf("Current Storage Size: %s / Max quota: %s", formatSize(totalSize), formatSize(maxDirSize))

	if totalSize <= maxDirSize {
		return
	}

	// Sort unpinned files by timestamp ascending (oldest first)
	sort.Slice(unpinnedFiles, func(i, j int) bool {
		return unpinnedFiles[i].ts < unpinnedFiles[j].ts
	})

	for _, f := range unpinnedFiles {
		if totalSize <= maxDirSize {
			break
		}

		if err := os.Remove(f.path); err == nil {
			totalSize -= f.size
			log.Printf("[SYSTEM] CLEANUP: removed oldest unpinned file %s (size: %s)", f.name, formatSize(f.size))
		} else {
			log.Printf("[SYSTEM] CLEANUP WARNING: failed to delete file %s: %v", f.name, err)
		}
	}
}
