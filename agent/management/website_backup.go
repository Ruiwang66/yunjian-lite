package management

import (
	"archive/zip"
	"cloudease-lite/internal/privatefs"
	"cloudease-lite/shared"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxBackupBytes int64 = 256 * 1024 * 1024

func (m *Manager) backupSite(ctx context.Context, site SiteConfig, kind string) (shared.BackupResult, error) {
	cfg := m.websites.config
	if secureRoot(cfg.BackupDir) != nil {
		return shared.BackupResult{}, errors.New("备份目录权限不安全")
	}
	if info, e := os.Stat(cfg.BackupDir); e != nil || mPathLinux() && info.Mode().Perm()&0077 != 0 {
		return shared.BackupResult{}, errors.New("备份目录必须为 root 专用 0700")
	}
	entries, err := os.ReadDir(cfg.BackupDir)
	if err != nil {
		return shared.BackupResult{}, err
	}
	if len(entries) >= 200 {
		return shared.BackupResult{}, errors.New("备份目录已达 200 项，请在主机清理旧备份")
	}
	var total int64
	for _, item := range entries {
		if info, e := item.Info(); e == nil {
			total += info.Size()
		}
	}
	if total >= 2*1024*1024*1024 {
		return shared.BackupResult{}, errors.New("备份目录已达 2 GiB 限制，请在主机清理旧备份")
	}
	suffix := ".zip"
	if kind == "database" {
		suffix = ".sqlite"
		if site.MySQLDatabase != "" {
			suffix = ".sql"
		}
	}
	temp, err := os.CreateTemp(cfg.BackupDir, ".partial-*")
	if err != nil {
		return shared.BackupResult{}, err
	}
	temp.Chmod(0600)
	partial := temp.Name()
	temp.Close()
	defer os.Remove(partial)
	switch kind {
	case "files":
		err = archiveWebsite(ctx, site.RootPath, partial)
	case "database":
		if site.SQLitePath != "" {
			input, checkErr := protectedOpen(filepath.Dir(site.SQLitePath), filepath.Base(site.SQLitePath), maxBackupBytes)
			if checkErr != nil {
				return shared.BackupResult{}, errors.New("SQLite 文件无效或权限不足")
			}
			input.Close()
			db, e := gorm.Open(sqlite.Open("file:"+(&url.URL{Path: filepath.ToSlash(site.SQLitePath)}).EscapedPath()+"?mode=ro"), &gorm.Config{})
			if e != nil {
				return shared.BackupResult{}, errors.New("SQLite 无法打开")
			}
			conn, e := db.DB()
			if e != nil {
				return shared.BackupResult{}, e
			}
			_, err = conn.ExecContext(ctx, "VACUUM INTO ?", partial)
			conn.Close()
			if err == nil {
				err = os.Chmod(partial, 0600)
			}
		} else if site.MySQLDatabase != "" {
			err = mysqlBackup(ctx, site, partial)
		} else {
			err = errors.New("此站点未配置 SQLite 或 MySQL 本地数据库备份")
		}
	default:
		err = errors.New("备份类型无效")
	}
	if err != nil {
		return shared.BackupResult{}, err
	}
	if ctx.Err() != nil {
		return shared.BackupResult{}, errors.New("备份超时，未保留不完整备份")
	}
	f, err := os.Open(partial)
	if err != nil {
		return shared.BackupResult{}, err
	}
	info, err := f.Stat()
	if err != nil || info.Size() > maxBackupBytes {
		f.Close()
		return shared.BackupResult{}, errors.New("备份超过 256 MiB 限制")
	}
	if total+info.Size() > 2*1024*1024*1024 {
		f.Close()
		return shared.BackupResult{}, errors.New("新增备份会超过 2 GiB 目录配额")
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	f.Close()
	if err != nil {
		return shared.BackupResult{}, err
	}
	label := site.Domain
	if len(label) > 80 {
		label = label[:80]
	}
	name := label + "-" + kind + "-" + time.Now().UTC().Format("20060102-150405") + "-" + strings.TrimPrefix(filepath.Base(partial), ".partial-") + suffix
	if err = os.Rename(partial, filepath.Join(cfg.BackupDir, name)); err != nil {
		return shared.BackupResult{}, err
	}
	return shared.BackupResult{Name: name, Kind: kind, Size: info.Size(), SHA256: hex.EncodeToString(hash.Sum(nil)), CreatedAt: time.Now().UnixMilli()}, nil
}
func archiveWebsite(ctx context.Context, dir, destination string) error {
	if secureRoot(dir) != nil {
		return errors.New("网站目录权限不安全")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	writer := zip.NewWriter(output)
	var bytes int64
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if name == "." {
			return nil
		}
		if strings.Count(name, "/") > 32 || len(name) > 2048 {
			return errors.New("网站目录层级过深")
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".git" || part == "backup" || part == "backups" || part == "node_modules" {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("网站备份拒绝符号链接，请使用独立真实目录")
		}
		if entry.IsDir() {
			return nil
		}
		count++
		if count > 10000 {
			return errors.New("网站备份最多 10000 个文件")
		}
		parent, base, cleanup, e := pinParent(root, name)
		if e != nil {
			return e
		}
		defer cleanup()
		before, e := parent.Lstat(base)
		if e != nil || !regularSingleLink(before) {
			return errors.New("网站备份拒绝硬链接、设备及特殊文件")
		}
		bytes += before.Size()
		if bytes > maxBackupBytes {
			return errors.New("网站文件超过 256 MiB 限制")
		}
		file, e := openNonblocking(parent, base)
		if e != nil {
			return e
		}
		defer file.Close()
		actual, e := file.Stat()
		if e != nil || !os.SameFile(before, actual) {
			return errors.New("网站文件已变化")
		}
		header, e := zip.FileInfoHeader(actual)
		if e != nil {
			return e
		}
		header.Name = name
		header.Method = zip.Deflate
		item, e := writer.CreateHeader(header)
		if e != nil {
			return e
		}
		n, e := io.Copy(item, io.LimitReader(file, before.Size()+1))
		after, statErr := file.Stat()
		if n != before.Size() || statErr != nil || !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
			return errors.New("备份期间网站文件发生变化，请重试")
		}
		return e
	})
	closeErr := writer.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return output.Sync()
}

type cappedBackup struct {
	file *os.File
	size int64
}

func (b *cappedBackup) Write(data []byte) (int, error) {
	if b.size+int64(len(data)) > maxBackupBytes {
		return 0, errors.New("database backup too large")
	}
	n, e := b.file.Write(data)
	b.size += int64(n)
	return n, e
}
func mysqlBackup(ctx context.Context, site SiteConfig, destination string) error {
	if _, err := privatefs.ReadSecret(site.MySQLDefaultsFile); err != nil {
		return errors.New("MySQL 凭据文件必须为私有普通文件且最多 512 字节")
	}
	var binary string
	for _, name := range []string{"/usr/bin/mysqldump", "/usr/bin/mariadb-dump"} {
		if info, e := os.Stat(name); e == nil && info.Mode().IsRegular() {
			binary = name
			break
		}
	}
	if binary == "" {
		return errors.New("mysqldump / mariadb-dump 未安装")
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd := exec.CommandContext(ctx, binary, "--defaults-file="+site.MySQLDefaultsFile, "--single-transaction", "--skip-lock-tables", "--skip-comments", "--no-tablespaces", "--", site.MySQLDatabase)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "LANG=C"}
	cmd.Stdout = &cappedBackup{file: file}
	cmd.Stderr = &boundedOutput{}
	cmd.WaitDelay = 2 * time.Second
	if cmd.Run() != nil {
		return errors.New("数据库备份失败，请检查主机数据库权限或超时；错误输出不返回凭据")
	}
	return file.Sync()
}
