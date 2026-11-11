package api

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func openDownload(name string) (*os.File, error) {
	switch name {
	case "cloud-agent-linux-amd64", "cloud-agent-linux-arm64", "cloud-agent.exe":
	default:
		return nil, fmt.Errorf("unlisted file")
	}
	path := filepath.Join("downloads", name)
	original, err := os.Lstat(path)
	if err != nil || !original.Mode().IsRegular() || original.Size() > 64<<20 {
		return nil, fmt.Errorf("invalid download file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil || !os.SameFile(original, actual) || actual.Size() > 64<<20 {
		f.Close()
		return nil, fmt.Errorf("download file changed")
	}
	return f, nil
}
func (a *API) downloadAgent(c *gin.Context) {
	if a.downloadSlots != nil {
		select {
		case a.downloadSlots <- struct{}{}:
			defer func() { <-a.downloadSlots }()
		default:
			c.Header("Retry-After", "5")
			fail(c, 503, "下载并发已满，请稍后重试")
			return
		}
	}
	f, err := openDownload(c.Param("file"))
	if err != nil {
		fail(c, 404, "安装包不存在")
		return
	}
	defer f.Close()
	// Fixed release files may be 64 MiB. Keep the short API write timeout for
	// JSON, but allow a bounded five-minute transfer on slower NAT links.
	if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(5 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		fail(c, 503, "无法开始下载安装包")
		return
	}
	info, _ := f.Stat()
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
	c.Header("Content-Type", "application/octet-stream")
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
}
func (a *API) downloadChecksum(c *gin.Context) {
	f, err := openDownload(c.Param("file"))
	if err != nil {
		fail(c, 404, "安装包不存在")
		return
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		fail(c, 500, "读取安装包失败")
		return
	}
	c.Data(200, "text/plain; charset=utf-8", []byte(fmt.Sprintf("%x\n", sum.Sum(nil))))
}
