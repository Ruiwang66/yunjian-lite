package api

import (
	"crypto/sha256"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
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
	f, err := openDownload(c.Param("file"))
	if err != nil {
		fail(c, 404, "安装包不存在")
		return
	}
	defer f.Close()
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
