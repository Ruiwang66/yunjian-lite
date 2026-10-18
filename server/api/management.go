package api

import (
	"cloudease-lite/server/models"
	"cloudease-lite/server/service"
	"cloudease-lite/shared"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io/fs"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (a *API) managementRoutes(r *gin.RouterGroup) {
	r.GET("/management/info", a.managementInfo)
	r.GET("/system/info", a.systemInfo)
	r.GET("/docker/info", a.dockerInfo)
	r.GET("/docker/container", a.manage("docker.list"))
	r.GET("/docker/container/logs", a.manage("docker.logs"))
	for _, action := range []string{"start", "stop", "restart"} {
		r.POST("/docker/container/"+action, a.manage("docker."+action))
	}
	r.GET("/service/list", a.manage("service.list"))
	r.POST("/service/action", a.manage("service.action"))
	r.GET("/files/list", a.manage("file.list"))
	r.GET("/files/read", a.manage("file.read"))
	r.GET("/files/download", a.manage("file.download"))
	r.POST("/files/upload", a.manage("file.upload"))
	r.POST("/commands/execute", a.manage("command.execute"))
	r.GET("/operations/:task", a.operation)
	r.GET("/operation-logs", a.operationLogs)
	r.GET("/users", a.users)
	r.POST("/users", a.createUser)
	r.PATCH("/users/:id", a.updateUser)
	r.DELETE("/users/:id", a.deleteUser)
	r.GET("/roles", a.roles)
}
func (a *API) managedServer(c *gin.Context, id string) (models.Server, bool) {
	var s models.Server
	if !numericID(id) {
		fail(c, 400, "请指定有效的 server_id")
		return s, false
	}
	ok := foundServer(c, a.DB, id, &s)
	return s, ok
}
func foundServer(c *gin.Context, db *gorm.DB, id string, s *models.Server) bool {
	return found(c, db.First(s, "id = ?", id).Error)
}
func (a *API) managementInfo(c *gin.Context) {
	s, ok := a.managedServer(c, c.Query("server_id"))
	if !ok {
		return
	}
	var info shared.ManagementInfo
	_ = json.Unmarshal([]byte(s.ManagementJSON), &info)
	c.JSON(200, gin.H{"server": a.view(s), "capabilities": info, "updated_at": s.ManagementSeenAt})
}
func (a *API) systemInfo(c *gin.Context) {
	s, ok := a.managedServer(c, c.Query("server_id"))
	if !ok {
		return
	}
	var info shared.SystemInfo
	_ = json.Unmarshal([]byte(s.SystemJSON), &info)
	c.JSON(200, gin.H{"server": a.view(s), "system": info})
}
func (a *API) dockerInfo(c *gin.Context) {
	s, ok := a.managedServer(c, c.Query("server_id"))
	if !ok {
		return
	}
	var info shared.ManagementInfo
	_ = json.Unmarshal([]byte(s.ManagementJSON), &info)
	c.JSON(200, gin.H{"docker": info.Docker, "server_status": a.view(s).Status, "updated_at": s.ManagementSeenAt})
}

var containerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
var serviceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}(?:\.service)?$`)

func operatorAction(action string) bool {
	return shared.MutatingAction(action) || strings.HasPrefix(action, "command.")
}
func validManagementRequest(req shared.ManagementRequest) bool {
	if req.Website != nil || shared.WebsiteAction(req.Action) {
		return false
	} // Website routes bind targets to a persisted, host-authorized site.
	if !shared.KnownAction(req.Action) || req.Root < 0 || req.Root > 7 || req.Action != "file.upload" && req.Content != "" {
		return false
	}
	if strings.HasPrefix(req.Action, "docker.") && req.Action != "docker.info" && req.Action != "docker.list" && !containerID.MatchString(req.Target) {
		return false
	}
	if strings.HasPrefix(req.Action, "service.") && req.Action != "service.list" && !serviceName.MatchString(req.Target) {
		return false
	}
	if strings.HasPrefix(req.Action, "file.") {
		if !fs.ValidPath(req.Path) || len(req.Path) > 2048 || strings.ContainsAny(req.Path, "\\:\x00") {
			return false
		}
		if req.Action == "file.upload" {
			data, err := base64.StdEncoding.DecodeString(req.Content)
			if err != nil || len(data) > shared.MaxFileBytes {
				return false
			}
		}
	} else if req.Path != "" || req.Content != "" || req.Root != 0 {
		return false
	}
	if !strings.HasPrefix(req.Action, "file.") && len(req.Target) > 160 {
		return false
	}
	return true
}
func (a *API) manage(baseAction string) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := baseAction
		var in struct {
			ServerID uint   `json:"server_id"`
			ID       string `json:"id"`
			Name     string `json:"name"`
			Action   string `json:"action"`
			Root     int    `json:"root"`
			Path     string `json:"path"`
			Content  string `json:"content"`
		}
		id := c.Query("server_id")
		if c.Request.Method == "POST" {
			if !decodeJSON(c, &in) {
				return
			}
			id = strconv.FormatUint(uint64(in.ServerID), 10)
		} else {
			in.ID = c.Query("id")
			in.Name = c.Query("name")
			in.Path = c.DefaultQuery("path", ".")
			if !strings.HasPrefix(action, "file.") {
				in.Path = ""
			}
			if raw := c.Query("root"); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil {
					fail(c, 400, "目录编号无效")
					return
				}
				in.Root = n
			}
		}
		if action == "service.action" {
			if in.Action != "start" && in.Action != "stop" && in.Action != "restart" {
				fail(c, 400, "服务操作只允许启动、停止和重启")
				return
			}
			action = "service." + in.Action
		} else if action == "command.execute" {
			action = "command." + in.Name
		}
		req := shared.ManagementRequest{Action: action, Target: in.ID, Root: in.Root, Path: in.Path, Content: in.Content}
		if strings.HasPrefix(action, "service.") {
			req.Target = in.Name
		}
		if !validManagementRequest(req) {
			fail(c, 400, "运维参数无效或操作不在白名单")
			return
		}
		u := c.MustGet("user").(models.User)
		if operatorAction(action) && u.Role == "viewer" {
			fail(c, 403, "只读用户不能执行此操作")
			return
		}
		s, ok := a.managedServer(c, id)
		if !ok {
			return
		}
		if a.view(s).Status != "online" {
			fail(c, 409, "服务器离线，无法执行运维操作")
			return
		}
		var info shared.ManagementInfo
		_ = json.Unmarshal([]byte(s.ManagementJSON), &info)
		if !info.Enabled || s.OS != "linux" {
			fail(c, 409, "请升级 Linux 监控客户端，并在客户端启用运维管理")
			return
		}
		if (strings.HasPrefix(action, "service.") && shared.MutatingAction(action) && !info.ServiceWrite) || (strings.HasPrefix(action, "docker.") && shared.MutatingAction(action) && !info.DockerWrite) || (action == "file.upload" && !info.FileWrite) {
			fail(c, 403, "客户端未授予对应操作权限")
			return
		}
		if strings.HasPrefix(action, "file.") && req.Root >= len(info.FileRoots) {
			fail(c, 400, "目录编号无效")
			return
		}
		taskID, err := service.RandomToken()
		if err != nil {
			fail(c, 500, "创建操作失败")
			return
		}
		raw, _ := json.Marshal(req)
		now := time.Now().UTC()
		logID := c.MustGet("operation_log").(uint)
		task := models.ManagementTask{ID: taskID, ServerID: s.ID, UserID: u.ID, SessionID: c.MustGet("session").(models.Session).ID, KeyHash: s.Token, Action: action, RequestJSON: string(raw), Status: "queued", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(45 * time.Second), OperationLogID: logID}
		err = a.DB.Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&models.ManagementTask{}).Where("status IN ? AND expires_at > ? AND server_id = ?", []string{"queued", "running"}, now, s.ID).Count(&count).Error; err != nil {
				return err
			}
			if count >= 4 {
				return errors.New("queue full")
			}
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
			target := req.Target
			if strings.HasPrefix(action, "file.") {
				target = strconv.Itoa(req.Root) + ":" + req.Path
			}
			return tx.Model(&models.OperationLog{}).Where("id = ?", logID).Updates(map[string]any{"server_id": s.ID, "action": action, "target": shared.SafeLabel(target), "status": "queued", "updated_at": now}).Error
		})
		if err != nil {
			fail(c, 503, "当前运维队列繁忙，请稍后重试")
			return
		}
		c.Set("audit_handled", true)
		c.JSON(202, gin.H{"id": task.ID, "status": task.Status, "expires_at": task.ExpiresAt})
	}
}
func (a *API) operation(c *gin.Context) {
	if !shared.ValidKey(c.Param("task")) {
		fail(c, 400, "操作编号无效")
		return
	}
	var task models.ManagementTask
	if !found(c, a.DB.First(&task, "id = ?", c.Param("task")).Error) {
		return
	}
	u := c.MustGet("user").(models.User)
	if task.UserID != u.ID && u.Role != "admin" {
		fail(c, 403, "无权读取此操作")
		return
	}
	if operatorAction(task.Action) && u.Role == "viewer" {
		fail(c, 403, "无权读取此操作")
		return
	}
	if (task.Status == "queued" || task.Status == "running") && time.Now().After(task.ExpiresAt) {
		if service.ExpireOperations(a.DB, time.Now().UTC()) != nil {
			dbError(c)
			return
		}
		if a.DB.First(&task, "id = ?", task.ID).Error != nil {
			dbError(c)
			return
		}
	}
	var result shared.ManagementResult
	if task.ResultJSON != "" {
		_ = json.Unmarshal([]byte(task.ResultJSON), &result)
	}
	c.JSON(200, gin.H{"id": task.ID, "status": task.Status, "data": result.Data, "error": result.Error, "expires_at": task.ExpiresAt})
}
func (a *API) nextOperation(c *gin.Context) {
	serverID := c.MustGet("agent_server_id").(uint)
	var reply *shared.ManagementTask
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var running int64
		if err := tx.Model(&models.ManagementTask{}).Where("server_id = ? AND status = ? AND expires_at > ?", serverID, "running", now).Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return nil
		}
		var s models.Server
		if err := tx.First(&s, "id = ? AND token = ?", serverID, c.MustGet("agent_key_hash").(string)).Error; err != nil {
			return err
		}
		var task models.ManagementTask
		err := tx.Where("server_id = ? AND status = ? AND expires_at > ?", serverID, "queued", now).Order("created_at").First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var u models.User
		var session models.Session
		if task.KeyHash != s.Token || tx.First(&u, task.UserID).Error != nil || tx.Where("id = ? AND user_id = ? AND expires_at > ?", task.SessionID, task.UserID, now).First(&session).Error != nil || !validRole(u.Role) || operatorAction(task.Action) && u.Role == "viewer" {
			if err := tx.Model(&task).Updates(map[string]any{"status": "cancelled", "request_json": "", "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.BackupTask{}).Where("task_id = ?", task.ID).Updates(map[string]any{"status": "cancelled", "updated_at": now}).Error; err != nil {
				return err
			}
			return tx.Model(&models.OperationLog{}).Where("id = ?", task.OperationLogID).Updates(map[string]any{"status": "cancelled", "updated_at": now}).Error
		}
		lease, err := service.RandomToken()
		if err != nil {
			return err
		}
		if err := tx.Model(&task).Where("status = ?", "queued").Updates(map[string]any{"status": "running", "lease_hash": service.HashToken(lease), "updated_at": now}).Error; err != nil {
			return err
		}
		var request shared.ManagementRequest
		if json.Unmarshal([]byte(task.RequestJSON), &request) != nil {
			return errors.New("invalid operation")
		}
		reply = &shared.ManagementTask{ID: task.ID, Lease: lease, ExpiresAt: task.ExpiresAt.UnixMilli(), Request: request}
		return tx.Model(&models.OperationLog{}).Where("id = ?", task.OperationLogID).Updates(map[string]any{"status": "running", "updated_at": now}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, 401, "连接密钥已失效")
		return
	}
	if err != nil {
		dbError(c)
		return
	}
	if reply == nil {
		c.Status(204)
		return
	}
	c.JSON(200, reply)
}
func (a *API) operationResult(c *gin.Context) {
	var in shared.ManagementResult
	if !decodeJSON(c, &in) {
		return
	}
	if !shared.ValidKey(in.ID) || !shared.ValidKey(in.Lease) || len(in.Data) > 48000 || len(in.Error) > 512 || (in.Success && len(in.Data) == 0) {
		fail(c, 400, "操作结果无效")
		return
	}
	serverID := c.MustGet("agent_server_id").(uint)
	conflict := false
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var task models.ManagementTask
		var s models.Server
		now := time.Now().UTC()
		if err := tx.First(&task, "id = ? AND server_id = ?", in.ID, serverID).Error; err != nil {
			return err
		}
		if err := tx.First(&s, "id = ? AND token = ?", serverID, c.MustGet("agent_key_hash").(string)).Error; err != nil {
			return err
		}
		if task.LeaseHash != service.HashToken(in.Lease) || task.KeyHash != s.Token || task.Status != "running" || now.After(task.ExpiresAt) {
			conflict = true
			return nil
		}
		if in.Success {
			if !strings.HasPrefix(task.Action, "file.") {
				var decoded any
				if json.Unmarshal(in.Data, &decoded) != nil {
					return errors.New("invalid data")
				}
				decoded = redactData(decoded)
				in.Data, _ = json.Marshal(decoded)
			}
			if err := cacheResult(tx, task, in.Data, now); err != nil {
				return err
			}
		}
		in.Lease = ""
		in.Error = shared.RedactOutput(shared.SafeLabel(in.Error))
		// Only the temporary task holds results; audit logs contain no output or file contents.
		raw, _ := json.Marshal(in)
		status := "failed"
		if in.Success {
			status = "success"
		}
		if err := tx.Model(&task).Updates(map[string]any{"status": status, "request_json": "", "result_json": string(raw), "updated_at": now}).Error; err != nil {
			return err
		}
		if strings.HasPrefix(task.Action, "backup.") {
			if err := tx.Model(&models.BackupTask{}).Where("task_id = ?", task.ID).Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.OperationLog{}).Where("id = ?", task.OperationLogID).Updates(map[string]any{"status": status, "updated_at": now}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, 404, "操作不存在")
		return
	}
	if err != nil {
		fail(c, 400, "操作结果格式无效或无法保存")
		return
	}
	if conflict {
		fail(c, 409, "操作已结束、过期或连接密钥已失效")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func cacheResult(tx *gorm.DB, task models.ManagementTask, data json.RawMessage, now time.Time) error {
	if shared.WebsiteAction(task.Action) {
		return cacheWebsiteResult(tx, task, data, now)
	}
	switch task.Action {
	case "docker.list":
		var items []shared.Container
		if json.Unmarshal(data, &items) != nil || len(items) > 200 {
			return errors.New("invalid containers")
		}
		if err := tx.Where("server_id = ?", task.ServerID).Delete(&models.DockerContainer{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if !containerID.MatchString(item.ID) || len(item.Name) > 256 || len(item.Ports) > 2048 {
				return errors.New("invalid container")
			}
			row := models.DockerContainer{ServerID: task.ServerID, ContainerID: item.ID, Name: item.Name, State: item.State, Ports: item.Ports, CreatedAt: item.CreatedAt, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
	case "service.list":
		var items []shared.Service
		if json.Unmarshal(data, &items) != nil || len(items) > 200 {
			return errors.New("invalid services")
		}
		if err := tx.Where("server_id = ?", task.ServerID).Delete(&models.Service{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if !validServiceDisplayName(item.Name) {
				return errors.New("invalid service")
			}
			row := models.Service{ServerID: task.ServerID, Name: item.Name, State: item.State, Enabled: item.Enabled, Allowed: item.Allowed, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
	case "file.list":
		var items []shared.FileEntry
		var req shared.ManagementRequest
		if json.Unmarshal(data, &items) != nil || len(items) > 1000 || json.Unmarshal([]byte(task.RequestJSON), &req) != nil {
			return errors.New("invalid files")
		}
		// Keep only the current directory snapshot for this root, bounding cache growth.
		if err := tx.Where("server_id = ? AND root = ?", task.ServerID, req.Root).Delete(&models.File{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if item.Name == "" || strings.ContainsAny(item.Name, "/\\\x00") || len(item.Name) > 255 || item.Size < 0 {
				return errors.New("invalid file")
			}
			path := item.Name
			if req.Path != "." {
				path = req.Path + "/" + item.Name
			}
			row := models.File{ServerID: task.ServerID, Root: req.Root, Path: path, Size: item.Size, Directory: item.Directory, ModifiedAt: item.ModifiedAt, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
func (a *API) operationLogs(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 10000 {
		fail(c, 400, "页码无效")
		return
	}
	q := a.DB.Model(&models.OperationLog{})
	if id := c.Query("server_id"); id != "" {
		if !numericID(id) {
			fail(c, 400, "服务器编号无效")
			return
		}
		q = q.Where("server_id = ?", id)
	}
	if status := c.Query("status"); status != "" {
		if !regexp.MustCompile(`^[a-z]{1,16}$`).MatchString(status) {
			fail(c, 400, "状态无效")
			return
		}
		q = q.Where("status = ?", status)
	}
	var total int64
	items := []models.OperationLog{}
	if q.Count(&total).Error != nil || q.Order("id desc").Limit(50).Offset((page-1)*50).Find(&items).Error != nil {
		dbError(c)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total, "page": page})
}

// Enhanced reports are additive; v0.1 reports without either extension remain valid.
func validExtensions(r shared.Report) bool {
	if r.System != nil {
		s := r.System
		if len(s.CPUModel) > 256 || len(s.KernelVersion) > 128 || len(s.Virtualization) > 128 || len(s.PrivateIPs)+len(s.PublicIPs) > 32 {
			return false
		}
		for _, ip := range append(append([]string{}, s.PrivateIPs...), s.PublicIPs...) {
			if net.ParseIP(ip) == nil {
				return false
			}
		}
	}
	if r.Management != nil {
		m := r.Management
		if m.Websites != nil {
			w := m.Websites
			if len(w.Sites) > 20 || len(w.Checks) > 20 || len(w.Nginx.Version) > 256 || len(w.Nginx.State) > 64 || len(w.PHP.Version) > 256 || len(w.PHP.FPMState) > 128 || len(w.PHP.Extensions) > 64 || len(w.PHP.Warnings) > 20 {
				return false
			}
			for _, s := range w.Sites {
				if !shared.ValidWebsite(s) {
					return false
				}
			}
			for _, check := range w.Checks {
				if !validWebsiteCheck(check) {
					return false
				}
			}
			for _, s := range append(append([]string{}, w.PHP.Extensions...), w.PHP.Warnings...) {
				if len(s) > 512 {
					return false
				}
			}
		}
		if len(m.Version) > 32 || len(m.FileRoots) > 8 || len(m.Services) > 200 || len(m.Docker.Version) > 128 || m.Docker.Containers < 0 || m.Docker.Images < 0 {
			return false
		}
		for _, root := range m.FileRoots {
			if len(root) > 2048 {
				return false
			}
		}
		for _, s := range m.Services {
			if !validServiceDisplayName(s.Name) || len(s.State) > 64 || len(s.Enabled) > 64 {
				return false
			}
		}
	}
	return true
}

var secretField = regexp.MustCompile(`(?i)^(password|passwd|token|secret|api[_-]?key|authorization)$`)

func redactData(value any) any {
	switch v := value.(type) {
	case string:
		return shared.RedactOutput(v)
	case []any:
		for i, item := range v {
			v[i] = redactData(item)
		}
		return v
	case map[string]any:
		for key, item := range v {
			if secretField.MatchString(key) {
				v[key] = "[已隐藏]"
			} else {
				v[key] = redactData(item)
			}
		}
		return v
	default:
		return value
	}
}

func validServiceDisplayName(name string) bool {
	if len(name) > 256 || name == ".service" || !strings.HasSuffix(name, ".service") {
		return false
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
