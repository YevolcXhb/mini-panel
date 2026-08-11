package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	syscmd "github.com/minipanel/minipanel/internal/utils/cmd"
	"gorm.io/gorm"
)

type WebsiteService struct {
	repo      *repository.WebsiteRepository
	wdRepo    *repository.WebsiteDatabaseRepository
	dbService *DatabaseService
	deleteMu  sync.Map // 按 website id 加锁，防止并发删/改
	configMu  sync.Mutex
}

func NewWebsiteService() *WebsiteService {
	return &WebsiteService{
		repo:      repository.NewWebsiteRepository(global.DB),
		wdRepo:    repository.NewWebsiteDatabaseRepository(global.DB),
		dbService: NewDatabaseService(),
	}
}

var (
	websiteDomainRe = regexp.MustCompile(`^(?:\*\.)?([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)
	websiteRateRe   = regexp.MustCompile(`^\d+[rR]/[smhd]$`)
	websiteSizeRe   = regexp.MustCompile(`^\d+[kKmM]?$`)
)

// validWebsiteDomain 校验域名/主机名（支持 IPv4/IPv6 字面量与 *. 通配前缀）。
func validWebsiteDomain(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || len(s) > 253 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n;{}`$\"'\\/") {
		return false
	}
	if strings.Contains(s, ":") {
		return net.ParseIP(strings.Trim(s, "[]")) != nil
	}
	if net.ParseIP(s) != nil {
		return true
	}
	if strings.HasPrefix(s, "*.") {
		return websiteDomainRe.MatchString(s[2:])
	}
	return websiteDomainRe.MatchString(s)
}

func validIPOrCIDR(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	return net.ParseIP(s) != nil
}

func validateRedirectsJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rules []model.RedirectRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return fmt.Errorf("redirects 格式错误: %v", err)
	}
	for _, r := range rules {
		if r.From == "" || r.To == "" {
			return fmt.Errorf("redirects 中存在空字段")
		}
		if r.Code != 301 && r.Code != 302 {
			return fmt.Errorf("redirect code 仅支持 301/302")
		}
		if strings.ContainsAny(r.From+r.To, "\r\n;{}`$\"'#") {
			return fmt.Errorf("redirects 包含非法字符")
		}
		lowerTo := strings.ToLower(r.To)
		if strings.HasPrefix(lowerTo, "http://") || strings.HasPrefix(lowerTo, "https://") {
			u, err := url.Parse(r.To)
			if err != nil || u.Host == "" {
				return fmt.Errorf("redirect 目标 URL 非法")
			}
		} else if !strings.HasPrefix(r.To, "/") {
			return fmt.Errorf("redirect 目标必须是 http(s) URL 或以 / 开头的路径")
		}
	}
	return nil
}

// validateWebsiteFields 白名单校验所有会写入 nginx 配置或文件系统的网站字段。
func validateWebsiteFields(w *model.Website) error {
	w.Name = strings.TrimSpace(w.Name)
	w.Domain = strings.TrimSpace(w.Domain)
	if w.Name == "" {
		return fmt.Errorf("网站名称不能为空")
	}
	if len(w.Name) > 100 {
		return fmt.Errorf("网站名称过长")
	}
	if !validWebsiteDomain(w.Domain) {
		return fmt.Errorf("域名格式非法")
	}
	switch w.Type {
	case "", "static", "proxy", "php":
	default:
		return fmt.Errorf("网站类型非法: %s", w.Type)
	}
	if w.Port < 0 || w.Port > 65535 {
		return fmt.Errorf("端口非法")
	}
	if w.Root != "" {
		if !filepath.IsAbs(w.Root) {
			return fmt.Errorf("网站根目录必须是绝对路径")
		}
		if strings.ContainsAny(w.Root, "\r\n;{}`$\"'#\\") {
			return fmt.Errorf("网站根目录包含非法字符")
		}
		for _, seg := range strings.Split(filepath.Clean(w.Root), string(filepath.Separator)) {
			if seg == ".." {
				return fmt.Errorf("网站根目录不能包含 ..")
			}
		}
	}
	if w.ProxyTarget != "" {
		if strings.ContainsAny(w.ProxyTarget, "\r\n;{}`$\"'#") {
			return fmt.Errorf("代理目标包含非法字符")
		}
		u, err := url.Parse(w.ProxyTarget)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("代理目标必须是 http/https URL")
		}
	}
	if w.ClientMaxBodySize != "" && !websiteSizeRe.MatchString(w.ClientMaxBodySize) {
		return fmt.Errorf("client_max_body_size 格式非法")
	}
	if w.RateLimitRate != "" && !websiteRateRe.MatchString(w.RateLimitRate) {
		return fmt.Errorf("频率限制格式非法，示例: 10r/s")
	}
	if w.IndexPage != "" && strings.ContainsAny(w.IndexPage, "\r\n;{}`$\"'\\#") {
		return fmt.Errorf("index 配置包含非法字符")
	}
	if w.HotlinkExts != "" && !regexp.MustCompile(`^[a-zA-Z0-9,]+$`).MatchString(w.HotlinkExts) {
		return fmt.Errorf("防盗链扩展名格式非法")
	}
	if w.HotlinkDomains != "" {
		for _, d := range strings.Fields(w.HotlinkDomains) {
			d = strings.TrimSuffix(d, ",")
			if d != "none" && d != "blocked" && !validWebsiteDomain(d) {
				return fmt.Errorf("防盗链域名非法: %s", d)
			}
		}
	}
	if w.IPFilterEnabled {
		if w.IPFilterMode != "blacklist" && w.IPFilterMode != "whitelist" {
			return fmt.Errorf("IP 过滤模式非法")
		}
		for _, line := range strings.Split(w.IPFilterList, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !validIPOrCIDR(line) {
				return fmt.Errorf("IP 过滤列表包含非法地址: %s", line)
			}
		}
	}
	if err := validateRedirectsJSON(w.Redirects); err != nil {
		return err
	}
	if w.AuthEnabled {
		if w.AuthUser == "" || w.AuthPassword == "" {
			return fmt.Errorf("目录保护需要用户名和密码")
		}
		if strings.ContainsAny(w.AuthUser, ":\r\n") || len(w.AuthUser) > 64 {
			return fmt.Errorf("目录保护用户名非法")
		}
		if strings.ContainsAny(w.AuthPassword, "\r\n") || len(w.AuthPassword) > 128 {
			return fmt.Errorf("目录保护密码非法")
		}
	}
	for _, html := range []string{w.ErrorPage404, w.ErrorPage502, w.ErrorPage503} {
		if len(html) > 64*1024 {
			return fmt.Errorf("自定义错误页过大")
		}
		if strings.ContainsRune(html, 0) {
			return fmt.Errorf("自定义错误页包含空字节")
		}
	}
	if len(w.SSLCertPEM) > 1024*1024 || len(w.SSLKeyPEM) > 1024*1024 {
		return fmt.Errorf("SSL PEM 内容过大")
	}
	if w.PhpVersion != "" && !regexp.MustCompile(`^\d+(\.\d+)?$`).MatchString(w.PhpVersion) {
		return fmt.Errorf("PHP 版本格式非法")
	}
	if w.RateLimitBurst < 0 || w.RateLimitBurst > 100000 {
		return fmt.Errorf("频率限制 burst 非法")
	}
	return nil
}

// websiteDirName 生成用于文件系统目录的域名安全名（去除通配符与特殊字符）。
func websiteDirName(domain string) string {
	return sanitizeNginxFileName(strings.TrimPrefix(strings.TrimSpace(domain), "*."))
}

func (s *WebsiteService) Create(w *model.Website) error {
	w.Managed = true
	normalizeWebsitePort(w)
	if err := validateWebsiteFields(w); err != nil {
		return err
	}
	// 检查 Domain:Port 是否已存在，存在则改为更新
	existing, _ := s.repo.GetByDomainPort(w.Domain, w.Port)
	if existing != nil {
		w.ID = existing.ID
		w.ConfigFile = existing.ConfigFile
		if err := s.repo.Update(w); err != nil {
			return err
		}
		return s.applyConfig(w)
	}
	if err := s.repo.Create(w); err != nil {
		return err
	}
	return s.applyConfig(w)
}

// CreateWithDB 建站时联动建库（核心正向流程）
// 仅在"真新建"分支（非 upsert 命中）且 DBCreate=true 时联动
func (s *WebsiteService) CreateWithDB(req *dto.WebsiteCreateRequest) error {
	sessionID := time.Now().UnixNano()
	w := &req.Website
	w.Managed = true
	normalizeWebsitePort(w)
	if err := validateWebsiteFields(w); err != nil {
		return err
	}

	global.LOG.Infof("[Engine] 会话ID: %d | CreateWithDB 入口 | name=%s domain=%s port=%d type=%s php_version=%s db_create=%v",
		sessionID, w.Name, w.Domain, w.Port, w.Type, w.PhpVersion, req.DBCreate)

	if req.DBCreate {
		global.LOG.Infof("[Engine] 会话ID: %d | 联动建库参数 | instance_id=%d db_name=%s db_username=%s password_len=%d",
			sessionID, req.DBInstanceID, req.DBName, req.DBUsername, len(req.DBPassword))
	}

	// 检查 Domain:Port 是否已存在，存在则走更新分支，不重复建库
	global.LOG.Infof("[Engine] 会话ID: %d | 步骤1: 检查域名重复 | domain=%s port=%d", sessionID, w.Domain, w.Port)
	existing, _ := s.repo.GetByDomainPort(w.Domain, w.Port)
	if existing != nil {
		global.LOG.Infof("[Engine] 会话ID: %d | 步骤1结果: upsert命中 | existing_id=%d (跳过建库)", sessionID, existing.ID)
		w.ID = existing.ID
		w.ConfigFile = existing.ConfigFile
		if err := s.repo.Update(w); err != nil {
			global.LOG.Errorf("[Engine] 会话ID: %d | 步骤1失败: Update错误 | err=%v", sessionID, err)
			return err
		}
		return s.applyConfig(w)
	}
	global.LOG.Infof("[Engine] 会话ID: %d | 步骤1结果: 真新建", sessionID)

	// 真新建分支
	if req.DBCreate {
		global.LOG.Infof("[Engine] 会话ID: %d | 步骤2: 进入联动建库分支", sessionID)
		if err := s.createWebsiteWithDB(req, sessionID); err != nil {
			global.LOG.Errorf("[Engine] 会话ID: %d | 步骤2失败: 联动建库失败 | err=%v", sessionID, err)
			return err
		}
		global.LOG.Infof("[Engine] 会话ID: %d | 步骤2结果: 联动建库成功 | website_id=%d", sessionID, w.ID)
	} else {
		global.LOG.Infof("[Engine] 会话ID: %d | 步骤2: 不联动建库，走旧逻辑", sessionID)
		if err := s.repo.Create(w); err != nil {
			global.LOG.Errorf("[Engine] 会话ID: %d | 步骤2失败: repo.Create错误 | err=%v", sessionID, err)
			return err
		}
	}

	global.LOG.Infof("[Engine] 会话ID: %d | 步骤3: 生成 Nginx 配置 | website_id=%d", sessionID, w.ID)
	if err := s.applyConfig(w); err != nil {
		global.LOG.Errorf("[Engine] 会话ID: %d | 步骤3失败: applyConfig错误 | err=%v", sessionID, err)
		return err
	}
	global.LOG.Infof("[Engine] 会话ID: %d | CreateWithDB 完成 | website_id=%d", sessionID, w.ID)
	return nil
}

// createWebsiteWithDB 在事务内创建网站记录 + MySQL 库/用户 + 关联记录
// MySQL 操作失败时回滚 SQLite 事务
func (s *WebsiteService) createWebsiteWithDB(req *dto.WebsiteCreateRequest, sessionID int64) error {
	w := &req.Website
	dbInstanceID := req.DBInstanceID
	if dbInstanceID == 0 {
		return fmt.Errorf("联动建库需要指定数据库实例 ID")
	}

	// 拉取数据库实例
	global.LOG.Infof("[Engine] 会话ID: %d | 事务前置: 拉取数据库实例 | instance_id=%d", sessionID, dbInstanceID)
	instance, err := s.dbService.GetByID(dbInstanceID)
	if err != nil {
		global.LOG.Errorf("[Engine] 会话ID: %d | 事务前置失败: 数据库实例不存在 | err=%v", sessionID, err)
		return fmt.Errorf("数据库实例不存在: %v", err)
	}
	global.LOG.Infof("[Engine] 会话ID: %d | 事务前置: 实例信息 | name=%s type=%s host=%s:%d", sessionID, instance.Name, instance.Type, instance.Host, instance.Port)

	// 参数兜底
	if req.DBName == "" {
		return fmt.Errorf("数据库名不能为空")
	}
	if req.DBUsername == "" {
		return fmt.Errorf("数据库用户名不能为空")
	}
	if req.DBPassword == "" {
		req.DBPassword = dto.GenerateRandomPassword()
		global.LOG.Infof("[Engine] 会话ID: %d | 事务前置: 自动生成密码 | user=%s pwd_len=%d", sessionID, req.DBUsername, len(req.DBPassword))
	}

	// 用户名全局唯一性校验（查关联表）
	global.LOG.Infof("[Engine] 会话ID: %d | 事务前置: 用户名唯一性校验 | user=%s", sessionID, req.DBUsername)
	existUser, _ := s.wdRepo.GetByInstanceAndUsername(dbInstanceID, req.DBUsername)
	if existUser != nil && existUser.ID > 0 {
		global.LOG.Warnf("[Engine] 会话ID: %d | 事务前置失败: 用户名冲突 | user=%s 占用网站=#%d", sessionID, req.DBUsername, existUser.WebsiteID)
		return fmt.Errorf("数据库用户名 '%s' 已被网站 #%d 使用，请换一个", req.DBUsername, existUser.WebsiteID)
	}
	// 库名全局唯一性校验
	global.LOG.Infof("[Engine] 会话ID: %d | 事务前置: 库名唯一性校验 | db=%s", sessionID, req.DBName)
	existDB, _ := s.wdRepo.GetByInstanceAndDBName(dbInstanceID, req.DBName)
	if existDB != nil && existDB.ID > 0 {
		global.LOG.Warnf("[Engine] 会话ID: %d | 事务前置失败: 库名冲突 | db=%s 占用网站=#%d", sessionID, req.DBName, existDB.WebsiteID)
		return fmt.Errorf("数据库名 '%s' 已被网站 #%d 使用，请换一个", req.DBName, existDB.WebsiteID)
	}
	global.LOG.Infof("[Engine] 会话ID: %d | 事务前置通过", sessionID)

	// 事务：先建 Website 记录拿 ID → 调 MySQL 建库建用户 → 写关联记录
	// 任何一步失败回滚 SQLite；MySQL 失败回滚已建的库
	global.LOG.Infof("[Engine] 会话ID: %d | 开启 SQLite 事务", sessionID)
	return global.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 创建 Website 记录
		global.LOG.Infof("[Engine] 会话ID: %d | 事务步骤1: 创建 Website 记录 | name=%s domain=%s", sessionID, w.Name, w.Domain)
		if err := tx.Create(w).Error; err != nil {
			global.LOG.Errorf("[Engine] 会话ID: %d | 事务步骤1失败: tx.Create | err=%v", sessionID, err)
			return fmt.Errorf("创建网站记录失败: %v", err)
		}
		global.LOG.Infof("[Engine] 会话ID: %d | 事务步骤1成功: website_id=%d", sessionID, w.ID)

		// 2. 调用 MySQL 建库+建用户+授权
		global.LOG.Infof("[Engine] 会话ID: %d | 事务步骤2: MySQL 建库+建用户 | db=%s user=%s", sessionID, req.DBName, req.DBUsername)
		if err := s.dbService.CreateDatabaseForWebsite(instance, req.DBName, req.DBUsername, req.DBPassword); err != nil {
			global.LOG.Errorf("[Engine] 会话ID: %d | 事务步骤2失败: MySQL 建库错误 | err=%v (将回滚 SQLite, website_id=%d)", sessionID, err, w.ID)
			return fmt.Errorf("联动建库失败: %v", err)
		}
		global.LOG.Infof("[Engine] 会话ID: %d | 事务步骤2成功: MySQL 库和用户已创建", sessionID)

		// 3. 写关联记录
		wd := &model.WebsiteDatabase{
			WebsiteID:    w.ID,
			DBInstanceID: dbInstanceID,
			DBName:       req.DBName,
			DBUsername:   req.DBUsername,
			DBPassword:   req.DBPassword,
		}
		global.LOG.Infof("[Engine] 会话ID: %d | 事务步骤3: 写关联记录 | website_id=%d db_instance_id=%d db=%s", sessionID, w.ID, dbInstanceID, req.DBName)
		if err := tx.Create(wd).Error; err != nil {
			// 关联记录写入失败，回滚已建的 MySQL 库（事务回滚后通过补偿删除）
			global.LOG.Warnf("[Engine] 会话ID: %d | 事务步骤3失败: tx.Create wd 错误 | err=%v, 补偿删除 MySQL db=%s user=%s", sessionID, err, req.DBName, req.DBUsername)
			_ = s.dbService.DropDatabase(instance, req.DBName)
			_ = s.dbService.DropUser(instance, req.DBUsername)
			return fmt.Errorf("写入关联记录失败: %v", err)
		}
		global.LOG.Infof("[Engine] 会话ID: %d | 事务步骤3成功: wd_id=%d", sessionID, wd.ID)

		global.LOG.Infof("[Engine] 会话ID: %d | 事务提交中 | website_id=%d db=%s user=%s instance=%d",
			sessionID, w.ID, req.DBName, req.DBUsername, dbInstanceID)
		return nil
	})
}

func (s *WebsiteService) Update(w *model.Website) error {
	old, err := s.repo.GetByID(w.ID)
	if err != nil {
		return err
	}
	if old.Domain != w.Domain {
		_ = s.removeConfig(old)
		// 域名变了，清除旧 ConfigFile 让 applyConfig 生成新路径
		w.ConfigFile = ""
	}
	w.Managed = true
	normalizeWebsitePort(w)
	if err := validateWebsiteFields(w); err != nil {
		return err
	}
	if err := s.repo.Update(w); err != nil {
		return err
	}
	return s.applyConfig(w)
}

// ListDatabasesByWebsiteID 查询网站关联的数据库列表
func (s *WebsiteService) ListDatabasesByWebsiteID(websiteID uint) ([]model.WebsiteDatabase, error) {
	return s.wdRepo.GetByWebsiteID(websiteID)
}

// ListWebsitesByInstanceID 查询数据库实例被哪些网站引用
func (s *WebsiteService) ListWebsitesByInstanceID(instanceID uint) ([]model.WebsiteDatabase, error) {
	return s.wdRepo.GetByInstanceID(instanceID)
}

func (s *WebsiteService) Delete(id uint) error {
	return s.DeleteWithCascade(id, true)
}

// DeleteWithCascade 删除网站并按需级联清理数据库
// cascadeDB=true 时：备份关联库 → DROP DATABASE + DROP USER（失败标 DropFailed 不阻塞删站）
// cascadeDB=false 时：保留 db 但删 WebsiteDatabase 关联记录
func (s *WebsiteService) DeleteWithCascade(id uint, cascadeDB bool) error {
	// 按 website id 加锁，防止并发删/改
	lockKey := fmt.Sprintf("delete-%d", id)
	_, _ = s.deleteMu.LoadOrStore(lockKey, struct{}{})
	defer s.deleteMu.Delete(lockKey)

	w, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("网站不存在: %v", err)
	}

	global.LOG.Infof("[Website] DeleteWithCascade start: id=%d name=%s cascade_db=%v", id, w.Name, cascadeDB)

	// 查询关联的数据库
	wdList, err := s.wdRepo.GetByWebsiteID(id)
	if err != nil {
		global.LOG.Warnf("[Website] GetByWebsiteID failed: %v (continue without cascade)", err)
		wdList = nil
	}

	// 级联清理 MySQL 库
	if cascadeDB && len(wdList) > 0 {
		for _, wd := range wdList {
			instance, err := s.dbService.GetByID(wd.DBInstanceID)
			if err != nil {
				global.LOG.Errorf("[Website] cascade: instance %d not found, mark DropFailed: %v", wd.DBInstanceID, err)
				wd.DropFailed = true
				_ = s.wdRepo.Update(&wd)
				continue
			}

			// 强制备份（DROP 不可逆）
			backupPath, backupErr := s.dbService.BackupDatabase(instance, wd.DBName)
			if backupErr != nil {
				// 备份失败：MySQL 离线或不可达 → 拒绝删站以保护数据
				global.LOG.Errorf("[Website] cascade: backup db=%s FAILED: %v (refuse to delete website)", wd.DBName, backupErr)
				return fmt.Errorf("删除网站失败：关联数据库 %s 备份失败（MySQL 可能不可达），请先恢复 MySQL 或使用 cascade_db=false: %v", wd.DBName, backupErr)
			}
			global.LOG.Infof("[Website] cascade: backup db=%s OK: %s", wd.DBName, backupPath)

			// DROP DATABASE
			if dropErr := s.dbService.DropDatabase(instance, wd.DBName); dropErr != nil {
				global.LOG.Errorf("[Website] cascade: drop db=%s FAILED: %v (mark DropFailed, continue)", wd.DBName, dropErr)
				wd.DropFailed = true
				_ = s.wdRepo.Update(&wd)
				continue
			}
			// DROP USER
			if dropErr := s.dbService.DropUser(instance, wd.DBUsername); dropErr != nil {
				global.LOG.Warnf("[Website] cascade: drop user=%s FAILED: %v (db already dropped, mark DropFailed)", wd.DBUsername, dropErr)
				wd.DropFailed = true
				_ = s.wdRepo.Update(&wd)
				continue
			}
			global.LOG.Infof("[Website] cascade: drop db=%s user=%s OK", wd.DBName, wd.DBUsername)
		}
	}

	// 删除 WebsiteDatabase 关联记录（无论 cascadeDB）
	if err := s.wdRepo.DeleteByWebsiteID(id); err != nil {
		global.LOG.Warnf("[Website] DeleteByWebsiteID failed: %v (continue)", err)
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()

	// 删除 Nginx 配置（仅删除面板生成的单站点文件，手动/共享文件保留）
	_ = s.removeConfig(w)

	// 删除 Website 记录
	if err := s.repo.Delete(id); err != nil {
		return err
	}

	// 同步默认 catch-all：最后一个 SSL 站点被删除时移除 443 兜底块
	_ = s.syncIsolationConfig("", "")
	_ = s.ReloadNginx()
	global.LOG.Infof("[Website] DeleteWithCascade OK: id=%d", id)
	return nil
}

// DeleteExternal 删除外部站点（仅删除配置文件，不入库）
func (s *WebsiteService) DeleteExternal(domain string, port int) error {
	if !validWebsiteDomain(domain) || port < 1 || port > 65535 {
		return fmt.Errorf("域名或端口非法")
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()

	_ = s.repo.DeleteByDomainPort(domain, port)
	confDirs := []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/usr/local/nginx/conf/conf.d",
		"/usr/local/nginx/conf/vhost",
	}
	confDirs = append(confDirs, s.GetNginxConfigDir())
	candidates := []string{
		fmt.Sprintf("%s.conf", domain),
		fmt.Sprintf("%d-%s.conf", port, domain),
	}
	for _, dir := range confDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		for _, name := range candidates {
			path := filepath.Join(dir, name)
			if s.safeToOverwriteConfig(path) {
				_ = os.Remove(path)
			}
		}
	}
	_ = s.syncIsolationConfig("", "")
	_ = s.ReloadNginx()
	return nil
}

// ToggleExternal 切换外部站点状态
func (s *WebsiteService) ToggleExternal(domain string, port int, enabled bool) error {
	if !validWebsiteDomain(domain) || port < 1 || port > 65535 {
		return fmt.Errorf("域名或端口非法")
	}
	existing, _ := s.repo.GetByDomainPort(domain, port)
	if existing != nil {
		existing.Enabled = enabled
		if err := s.repo.Update(existing); err != nil {
			return err
		}
		return s.applyConfig(existing)
	}
	if enabled {
		// 启用：先扫描查找原始配置信息
		scannedSites := s.scanNginxWebsites()
		var info *model.Website
		for i := range scannedSites {
			if scannedSites[i].Domain == domain && scannedSites[i].Port == port {
				info = &scannedSites[i]
				break
			}
		}
		if info == nil {
			return fmt.Errorf("找不到该外部站点的配置信息")
		}
		// 入库并启用
		info.Enabled = true
		info.Managed = true
		if err := s.repo.Create(info); err != nil {
			return err
		}
		return s.applyConfig(info)
	}
	// 停用：扫描找到配置文件，仅删除面板生成的单站点文件
	s.configMu.Lock()
	defer s.configMu.Unlock()
	confPath := s.findExternalConfigFile(domain, port)
	if confPath != "" && s.safeToOverwriteConfig(confPath) {
		_ = os.Remove(confPath)
	}
	_ = s.syncIsolationConfig("", "")
	_ = s.ReloadNginx()
	return nil
}

// findExternalConfigFile 查找外部站点的配置文件路径
func (s *WebsiteService) findExternalConfigFile(domain string, port int) string {
	scannedSites := s.scanNginxWebsites()
	for _, site := range scannedSites {
		if site.Domain == domain && site.Port == port {
			return site.ConfigFile
		}
	}
	return ""
}

func (s *WebsiteService) GetByID(id uint) (*model.Website, error) {
	return s.repo.GetByID(id)
}

// List 获取所有网站，包括数据库中面板创建的和nginx中外部创建的
func (s *WebsiteService) List() ([]model.Website, error) {
	// 1. 获取数据库中的网站
	dbSites, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	// 兼容历史数据：SSL 站点端口 0/80 统一规范为 443，避免与扫描结果重复展示
	for i := range dbSites {
		if dbSites[i].SSL && (dbSites[i].Port == 0 || dbSites[i].Port == 80) {
			dbSites[i].Port = 443
			_ = s.repo.Update(&dbSites[i])
		}
	}

	// 2. 扫描nginx配置中的网站
	scannedSites := s.scanNginxWebsites()

	// 3. 合并：数据库中已有的不重复添加，新发现的外部站点也展示
	existing := make(map[string]bool)
	for _, site := range dbSites {
		key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
		existing[key] = true
	}

	for _, site := range scannedSites {
		key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
		if !existing[key] {
			// 纯外部站点：标记 Managed=false、ID=0，只展示不持久化
			site.Managed = false
			site.ID = 0
			dbSites = append(dbSites, site)
			existing[key] = true
		} else {
			// DB 中已有该域名+端口的记录，以 DB 为权威，但用扫描结果补充 ConfigFile
			for i := range dbSites {
				if fmt.Sprintf("%s:%d", dbSites[i].Domain, dbSites[i].Port) == key && dbSites[i].ConfigFile == "" && site.ConfigFile != "" {
					dbSites[i].ConfigFile = site.ConfigFile
					_ = s.repo.Update(&dbSites[i])
					break
				}
			}
		}
	}

	return dbSites, nil
}

func (s *WebsiteService) ToggleEnable(id uint, enabled bool) error {
	w, err := s.repo.GetByID(id)
	if err != nil {
		// 外部站点（id=0）不存在于DB，无需切换
		if id == 0 {
			return fmt.Errorf("外部站点请先在面板中编辑保存后再操作")
		}
		return err
	}
	w.Enabled = enabled
	w.Managed = true
	if err := s.repo.Update(w); err != nil {
		return err
	}
	return s.applyConfig(w)
}

// GetNginxStatus 获取Nginx服务状态
func (s *WebsiteService) GetNginxStatus() (*model.NginxStatus, error) {
	status := &model.NginxStatus{
		Installed: false,
		Running:   false,
		Version:   "",
		ConfigDir: s.GetNginxConfigDir(),
		Pid:       0,
	}

	// 检查nginx是否安装
	if !syscmd.Which("nginx") {
		return status, nil
	}
	status.Installed = true

	// 获取版本
	output, err := exec.Command("nginx", "-v").CombinedOutput()
	if err == nil {
		verStr := string(output)
		re := regexp.MustCompile(`nginx/(\d+\.\d+\.\d+)`)
		if matches := re.FindStringSubmatch(verStr); len(matches) >= 2 {
			status.Version = matches[1]
		}
	}

	// 检查是否运行 - 多种方式交叉验证
	pid := s.findRunningNginxPID()
	if pid > 0 {
		status.Running = true
		status.Pid = pid
	}

	return status, nil
}

// findRunningNginxPID 通过多种方式查找正在运行的nginx master PID
func (s *WebsiteService) findRunningNginxPID() int {
	// 方式1: ps 命令查找 master 进程
	if pid := s.findNginxPIDByPS(); pid > 0 {
		return pid
	}
	// 方式2: pgrep
	if pid := s.findNginxPIDByPGrep(); pid > 0 {
		return pid
	}
	// 方式3: pid 文件 + kill -0 验证
	if pid := s.findNginxPIDByPidFile(); pid > 0 {
		return pid
	}
	// 方式4: 检查端口监听（80/443）
	if s.isNginxListening() {
		return -1
	}
	return 0
}

func (s *WebsiteService) findNginxPIDByPS() int {
	output, err := exec.Command("ps", "-eo", "pid,comm,args").CombinedOutput()
	if err != nil {
		// 尝试 ps aux
		output, err = exec.Command("ps", "aux").CombinedOutput()
		if err != nil {
			return 0
		}
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "grep") {
			continue
		}
		// 匹配 nginx: master process
		if !strings.Contains(line, "nginx") {
			continue
		}
		// 排除 worker 进程（含 "worker"）
		if strings.Contains(line, "worker") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pidStr := fields[0]
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			// ps aux 格式不同
			pid, err = strconv.Atoi(fields[1])
			if err != nil {
				continue
			}
		}
		// 验证进程确实存在
		if s.isProcessAlive(pid) {
			return pid
		}
	}
	return 0
}

func (s *WebsiteService) findNginxPIDByPGrep() int {
	output, err := exec.Command("pgrep", "-f", "nginx: master").CombinedOutput()
	if err != nil {
		return 0
	}
	pidStr := strings.TrimSpace(string(output))
	lines := strings.Split(pidStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err == nil && s.isProcessAlive(pid) {
			return pid
		}
	}
	return 0
}

func (s *WebsiteService) findNginxPIDByPidFile() int {
	pidFile := s.findNginxPidFile()
	if pidFile == "" {
		return 0
	}
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pidStr := strings.TrimSpace(string(pidData))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0
	}
	if s.isProcessAlive(pid) {
		return pid
	}
	return 0
}

// isProcessAlive 跨平台检查进程是否存活
func (s *WebsiteService) isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// 使用 kill -0 检查（Linux/macOS）
	cmd := exec.Command("kill", "-0", strconv.Itoa(pid))
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

// isNginxListening 检查 nginx 是否在监听 80/443 端口
func (s *WebsiteService) isNginxListening() bool {
	for _, port := range []string{"80", "443", "8080"} {
		cmd := exec.Command("sh", "-c", "ss -tln 2>/dev/null | grep ':"+port+" ' || netstat -tln 2>/dev/null | grep ':"+port+" '")
		if output, err := cmd.CombinedOutput(); err == nil && len(strings.TrimSpace(string(output))) > 0 {
			return true
		}
	}
	return false
}

func (s *WebsiteService) findNginxPidFile() string {
	candidates := []string{
		"/var/run/nginx.pid",
		"/run/nginx.pid",
		"/usr/local/nginx/logs/nginx.pid",
		"/var/log/nginx/nginx.pid",
	}
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	// 尝试从nginx配置中查找
	output, err := exec.Command("nginx", "-T").CombinedOutput()
	if err == nil {
		re := regexp.MustCompile(`pid\s+([^;]+);`)
		if matches := re.FindStringSubmatch(string(output)); len(matches) >= 2 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

// StartNginx 启动Nginx
func (s *WebsiteService) StartNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx 未安装")
	}

	// 先检查是否已经在运行
	if pid := s.findRunningNginxPID(); pid > 0 {
		return nil
	}

	// 测试配置（不阻塞启动，仅警告）
	configOutput, configErr := exec.Command("nginx", "-t").CombinedOutput()
	if configErr != nil {
		global.LOG.Warnf("nginx -t warning: %s", string(configOutput))
	}

	// 尝试多种方式启动
	// 方式1: systemctl
	if err := exec.Command("systemctl", "start", "nginx").Run(); err == nil {
		if s.waitForNginxRunning(3) {
			return nil
		}
	}
	// 方式2: service
	if err := exec.Command("service", "nginx", "start").Run(); err == nil {
		if s.waitForNginxRunning(3) {
			return nil
		}
	}
	// 方式3: 直接启动
	cmd := exec.Command("nginx")
	if err := cmd.Run(); err != nil {
		// nginx 默认是 daemon 模式，Run() 立即返回，需要等待检查
		global.LOG.Warnf("direct nginx start returned: %v", err)
	}
	if s.waitForNginxRunning(3) {
		return nil
	}
	return fmt.Errorf("启动nginx失败，进程未运行 (config test: %s)", string(configOutput))
}

// waitForNginxRunning 轮询等待 nginx 运行起来
func (s *WebsiteService) waitForNginxRunning(timeoutSec int) bool {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		if pid := s.findRunningNginxPID(); pid > 0 {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

// StopNginx 停止Nginx
func (s *WebsiteService) StopNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx 未安装")
	}

	// 收集所有要杀的 PID，避免 pid 文件在 kill 过程中被覆盖
	pidToKill := s.findRunningNginxPID()
	if pidToKill <= 0 {
		return nil
	}

	// 尝试 systemctl
	if err := exec.Command("systemctl", "stop", "nginx").Run(); err == nil {
		if s.waitForNginxStopped(5) {
			return nil
		}
	}
	// 尝试 service
	if err := exec.Command("service", "nginx", "stop").Run(); err == nil {
		if s.waitForNginxStopped(5) {
			return nil
		}
	}
	// 发送 stop 信号（优雅关闭）
	_ = exec.Command("nginx", "-s", "stop").Run()
	if s.waitForNginxStopped(3) {
		return nil
	}
	// 发送 quit 信号
	_ = exec.Command("nginx", "-s", "quit").Run()
	if s.waitForNginxStopped(3) {
		return nil
	}
	// 最后直接用 kill 杀进程
	cmd := exec.Command("kill", "-9", strconv.Itoa(pidToKill))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("停止nginx失败: kill -9 %d 失败", pidToKill)
	}
	if s.waitForNginxStopped(3) {
		return nil
	}
	return fmt.Errorf("停止nginx失败: 进程 %d 无法被杀死", pidToKill)
}

// RestartNginx 重启Nginx
func (s *WebsiteService) RestartNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx 未安装")
	}

	// 尝试 systemctl restart
	if err := exec.Command("systemctl", "restart", "nginx").Run(); err == nil {
		if s.waitForNginxRunning(3) {
			return nil
		}
	}
	// 尝试 service restart
	if err := exec.Command("service", "nginx", "restart").Run(); err == nil {
		if s.waitForNginxRunning(3) {
			return nil
		}
	}
	// 手动 stop + start
	if err := s.StopNginx(); err != nil {
		return fmt.Errorf("重启失败(停止阶段): %w", err)
	}
	if err := s.StartNginx(); err != nil {
		return fmt.Errorf("重启失败(启动阶段): %w", err)
	}
	return nil
}

// waitForNginxStopped 等待 nginx 停止
func (s *WebsiteService) waitForNginxStopped(timeoutSec int) bool {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		if pid := s.findRunningNginxPID(); pid == 0 {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func (s *WebsiteService) ReloadNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx is not installed")
	}
	output, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx config test failed: %s", string(output))
	}
	// 尝试reload
	reloadCmd := exec.Command("nginx", "-s", "reload")
	if _, err := reloadCmd.CombinedOutput(); err == nil {
		return nil
	}
	// nginx -s reload 失败，尝试 systemctl reload
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err == nil {
		return nil
	} else {
		global.LOG.Warnf("[Website] systemctl reload nginx failed: %s, trying restart", strings.TrimSpace(string(out)))
	}
	// 最后尝试 restart（影响连接，但确保配置生效）
	if out, err := exec.Command("systemctl", "restart", "nginx").CombinedOutput(); err == nil {
		if s.waitForNginxRunning(3) {
			return nil
		}
		return fmt.Errorf("nginx restart 后未检测到运行: %s", strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("nginx reload 失败，请检查 nginx 是否正在运行")
}

func (s *WebsiteService) GetNginxConfigDir() string {
	candidates := []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/usr/local/nginx/conf/conf.d",
		"/usr/local/nginx/conf/vhost",
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	// 尝试从nginx -T输出获取配置目录
	output, err := exec.Command("nginx", "-T").CombinedOutput()
	if err == nil {
		content := string(output)
		// 查找include配置
		re := regexp.MustCompile(`include\s+([^;]+/conf\.d|\*/conf\.d|\*/sites-enabled);`)
		if matches := re.FindStringSubmatch(content); len(matches) >= 2 {
			dir := strings.TrimSpace(matches[1])
			dir = strings.ReplaceAll(dir, "*", "/etc/nginx")
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	panelDir := filepath.Join(global.GetDataDir(), "nginx")
	_ = os.MkdirAll(panelDir, 0755)
	return panelDir
}

// scanNginxWebsites 扫描所有nginx配置文件，提取网站信息
func (s *WebsiteService) scanNginxWebsites() []model.Website {
	var sites []model.Website
	seen := make(map[string]bool)

	// 优先通过 nginx -T 拿到合并后的完整配置（包含所有 include 的文件）
	if mergedSites := s.parseNginxMergedConfig(); len(mergedSites) > 0 {
		for _, site := range mergedSites {
			key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
			if !seen[key] {
				sites = append(sites, site)
				seen[key] = true
			}
		}
		return sites
	}

	// 退路：直接遍历常见配置目录
	configDirs := []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/usr/local/nginx/conf/conf.d",
		"/usr/local/nginx/conf/vhost",
	}
	configDirs = append(configDirs, s.GetNginxConfigDir())

	for _, dir := range configDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".conf") {
				return nil
			}
			parsedSites := s.parseNginxConfig(path)
			for _, site := range parsedSites {
				key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
				if !seen[key] {
					sites = append(sites, site)
					seen[key] = true
				}
			}
			return nil
		})
	}

	// 也扫描主配置文件中的server块
	mainConf := "/etc/nginx/nginx.conf"
	if _, err := os.Stat(mainConf); err == nil {
		parsedSites := s.parseNginxConfig(mainConf)
		for _, site := range parsedSites {
			key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
			if !seen[key] {
				sites = append(sites, site)
				seen[key] = true
			}
		}
	}

	return sites
}

// parseNginxMergedConfig 通过 nginx -T 获取合并后的完整配置并解析
func (s *WebsiteService) parseNginxMergedConfig() []model.Website {
	if !syscmd.Which("nginx") {
		return nil
	}
	output, err := exec.Command("nginx", "-T").CombinedOutput()
	if err != nil {
		return nil
	}
	content := string(output)
	if len(strings.TrimSpace(content)) == 0 {
		return nil
	}

	// 优先用 # configuration file 注释拆分（nginx -T 自带标记）
	parts := expandNginxMergedConfig(content)
	if len(parts) > 1 {
		var sites []model.Website
		seen := make(map[string]bool)
		for file, body := range parts {
			if file == "__main__" {
				continue
			}
			fileSites := s.parseNginxConfigContent(body, file)
			for _, site := range fileSites {
				key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
				if !seen[key] {
					sites = append(sites, site)
					seen[key] = true
				}
			}
		}
		if len(sites) > 0 {
			return sites
		}
	}
	// 回退：当作单一文件解析
	return s.parseNginxConfigContent(content, "")
}

// parseNginxConfig 解析nginx配置文件，提取所有server块信息
func (s *WebsiteService) parseNginxConfig(configPath string) []model.Website {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	fileName := filepath.Base(configPath)
	fileName = strings.TrimSuffix(fileName, ".conf")
	return s.parseNginxConfigContent(string(data), fileName)
}

// parseNginxConfigContent 解析nginx配置内容，使用栈跟踪 server 块和 include 上下文
func (s *WebsiteService) parseNginxConfigContent(content string, defaultName string) []model.Website {
	// 面板生成的默认 catch-all 配置不当作站点展示
	if strings.Contains(filepath.Base(defaultName), "minipanel-default") {
		return nil
	}

	var sites []model.Website

	// 先去掉注释
	noComments := stripNginxComments(content)

	blocks := extractServerBlocks(noComments, defaultName)

	for _, b := range blocks {
		site := extractSiteFromServerBlock(noComments[b.start:b.end], b.filename, defaultName)
		if site != nil {
			sites = append(sites, *site)
		}
	}
	return sites
}

// expandNginxMergedConfig 将 nginx -T 输出按 # configuration file 注释拆分，
// 解决嵌套 include 中 server 块归属问题
func expandNginxMergedConfig(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	confRe := regexp.MustCompile(`^#\s*configuration file\s+(\S+):`)
	var currentFile string
	var sb strings.Builder
	flush := func() {
		if currentFile != "" {
			result[currentFile] = sb.String()
		} else {
			result["__main__"] = sb.String()
		}
		sb.Reset()
	}
	for _, line := range lines {
		if m := confRe.FindStringSubmatch(line); len(m) >= 2 {
			flush()
			currentFile = m[1]
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	flush()
	return result
}

// stripNginxComments 去掉 # 后的行尾注释（保留引号内的 #）
func stripNginxComments(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			sb.WriteByte(c)
			sb.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			sb.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			sb.WriteByte(c)
			continue
		}
		if c == '#' && !inSingle && !inDouble {
			// 跳过到行尾
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				sb.WriteByte('\n')
			}
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// extractServerBlocks 提取所有 server { ... } 块的位置
func extractServerBlocks(content string, defaultFile string) []struct {
	start, end int
	filename   string
} {
	var result []struct {
		start, end int
		filename   string
	}
	serverRe := regexp.MustCompile(`(?is)server\s*\{`)
	locs := serverRe.FindAllStringIndex(content, -1)
	for _, loc := range locs {
		openIdx := loc[1] - 1
		depth := 1
		pos := openIdx + 1
		inSingle := false
		inDouble := false
		for pos < len(content) {
			c := content[pos]
			if c == '\\' && pos+1 < len(content) {
				pos += 2
				continue
			}
			if c == '\'' && !inDouble {
				inSingle = !inSingle
			} else if c == '"' && !inSingle {
				inDouble = !inDouble
			} else if !inSingle && !inDouble {
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						result = append(result, struct {
							start, end int
							filename   string
						}{start: loc[0], end: pos + 1, filename: defaultFile})
						break
					}
				}
			}
			pos++
		}
	}
	return result
}

// extractSiteFromServerBlock 从 server 块内容中提取 site 信息
func extractSiteFromServerBlock(block, fileName, defaultName string) *model.Website {
	if fileName == "" {
		fileName = defaultName
		if fileName == "nginx" {
			fileName = "default"
		}
	}
	if fileName == "" {
		fileName = "default"
	}

	// 收集 server_name
	serverName := ""
	serverNameRe := regexp.MustCompile(`(?m)server_name\s+([^;]+);`)
	if m := serverNameRe.FindStringSubmatch(block); len(m) >= 2 {
		names := strings.Fields(m[1])
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" && name != "_" && !strings.HasPrefix(name, "*.") && !strings.HasPrefix(name, "~") {
				serverName = name
				break
			}
		}
		if serverName == "" && len(names) > 0 {
			first := strings.TrimSpace(names[0])
			if first != "_" {
				serverName = first
			}
		}
	}

	// 纯 catch-all 块（server_name _）不作为站点展示
	if m := serverNameRe.FindStringSubmatch(block); len(m) >= 2 {
		allCatchAll := true
		for _, n := range strings.Fields(m[1]) {
			n = strings.TrimSpace(n)
			if n != "" && n != "_" {
				allCatchAll = false
				break
			}
		}
		if allCatchAll {
			return nil
		}
	}

	// 收集 listen 端口
	var listenPorts []int
	hasSSL := false
	listenRe := regexp.MustCompile(`(?m)listen\s+([^;]+);`)
	listenMatches := listenRe.FindAllStringSubmatch(block, -1)
	for _, m := range listenMatches {
		if len(m) < 2 {
			continue
		}
		args := strings.Fields(m[1])
		port := 0
		defaultPort := false
		ssl := false
		for _, a := range args {
			if a == "default_server" {
				defaultPort = true
			}
			if a == "ssl" {
				ssl = true
			}
			if p, err := strconv.Atoi(a); err == nil {
				port = p
			}
		}
		if port == 0 && defaultPort {
			port = 80
		}
		if port > 0 {
			listenPorts = append(listenPorts, port)
			if ssl {
				hasSSL = true
			}
		}
	}
	if len(listenPorts) == 0 {
		// 没有 listen 指令的 server 块跳过
		return nil
	}

	// root
	rootDir := ""
	if m := regexp.MustCompile(`(?m)root\s+([^;]+);`).FindStringSubmatch(block); len(m) >= 2 {
		rootDir = strings.TrimSpace(m[1])
	}

	// proxy_pass
	proxyPass := ""
	if m := regexp.MustCompile(`(?m)proxy_pass\s+([^;]+);`).FindStringSubmatch(block); len(m) >= 2 {
		proxyPass = strings.TrimSpace(m[1])
	}

	if serverName == "" || serverName == "_" {
		serverName = fileName
		if serverName == "nginx" {
			serverName = "default"
		}
	}
	if serverName == "" {
		serverName = "default"
	}

	port := listenPorts[0]
	// SSL 站点优先展示 443；无 443 时优先使用第一个非 80/8080 端口（兼容自定义 HTTPS 端口）
	if hasSSL {
		for _, p := range listenPorts {
			if p == 443 {
				port = p
				break
			}
		}
		if port == 80 || port == 8080 {
			for _, p := range listenPorts {
				if p != 80 && p != 8080 {
					port = p
					break
				}
			}
		}
	}

	siteType := "static"
	if proxyPass != "" {
		siteType = "proxy"
	}

	name := serverName
	if serverName == "default" {
		name = "默认站点"
	}

	return &model.Website{
		Name:        name,
		Domain:      serverName,
		Port:        port,
		Root:        rootDir,
		Type:        siteType,
		ProxyTarget: proxyPass,
		SSL:         hasSSL,
		Enabled:     true,
		Managed:     false,
		ConfigFile:  fileName,
		Remark:      "检测到外部创建的网站",
	}
}

func testWritable(dir string) bool {
	testFile := filepath.Join(dir, ".minipanel_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return false
	}
	os.Remove(testFile)
	return true
}

func (s *WebsiteService) nginxConfPath(w *model.Website) string {
	confDir := s.GetNginxConfigDir()
	return filepath.Join(confDir, sanitizeNginxFileName(w.Domain)+".conf")
}

func (s *WebsiteService) removeConfig(w *model.Website) error {
	// 优先使用已知的配置文件路径（外部扫描的站点）
	path := w.ConfigFile
	if path == "" {
		path = s.nginxConfPath(w)
	}
	// 仅删除面板生成的单站点文件，手动维护或共享配置文件一律保留
	if s.safeToOverwriteConfig(path) {
		_ = os.Remove(path)
	}
	return nil
}

func (s *WebsiteService) buildWebsiteNginxConfig(w *model.Website) (content string, certPath, keyPath string) {
	var sb strings.Builder
	httpPort, httpsPort := nginxListenPorts(w)

	sb.WriteString("# Managed by MiniPanel\n")

	// 频率限制 zone（全局，在所有 server 块之前）
	if w.RateLimitEnabled && w.RateLimitRate != "" {
		zoneName := fmt.Sprintf("zone_%s_%d", websiteDirName(w.Domain), w.Port)
		burst := w.RateLimitBurst
		if burst <= 0 {
			burst = 10
		}
		sb.WriteString(fmt.Sprintf("limit_req_zone $binary_remote_addr zone=%s:10m rate=%s;\n", zoneName, w.RateLimitRate))
		sb.WriteString("\n")
	}

	// 处理 SSL 证书 PEM 内容：写入文件并取得路径
	if w.SSL {
		certPath = w.SSLCert
		keyPath = w.SSLKey
		if w.SSLCertPEM != "" || w.SSLKeyPEM != "" {
			sslDir := filepath.Join(global.GetDataDir(), "ssl", websiteDirName(w.Domain))
			_ = os.MkdirAll(sslDir, 0700)
			if w.SSLCertPEM != "" {
				certPath = filepath.Join(sslDir, "fullchain.pem")
				_ = os.WriteFile(certPath, []byte(w.SSLCertPEM), 0600)
			}
			if w.SSLKeyPEM != "" {
				keyPath = filepath.Join(sslDir, "privkey.pem")
				_ = os.WriteFile(keyPath, []byte(w.SSLKeyPEM), 0600)
			}
		}
	}

	sb.WriteString("server {\n")
	if w.SSL {
		sb.WriteString(fmt.Sprintf("    listen %d;\n", httpPort))
		sb.WriteString(fmt.Sprintf("    listen [::]:%d;\n", httpPort))
		sb.WriteString(fmt.Sprintf("    server_name %s;\n", w.Domain))
		redirectTarget := "https://$host$request_uri"
		if httpsPort != 443 {
			redirectTarget = fmt.Sprintf("https://$host:%d$request_uri", httpsPort)
		}
		sb.WriteString(fmt.Sprintf("    return 301 %s;\n", redirectTarget))
		sb.WriteString("}\n\n")

		sb.WriteString("server {\n")
		sb.WriteString(fmt.Sprintf("    listen %d ssl http2;\n", httpsPort))
		sb.WriteString(fmt.Sprintf("    listen [::]:%d ssl http2;\n", httpsPort))
		sb.WriteString(fmt.Sprintf("    server_name %s;\n", w.Domain))
		if certPath != "" {
			sb.WriteString(fmt.Sprintf("    ssl_certificate %s;\n", certPath))
		}
		if keyPath != "" {
			sb.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n", keyPath))
		}
		sb.WriteString("    ssl_protocols TLSv1.2 TLSv1.3;\n")
		sb.WriteString("    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;\n")
		sb.WriteString("    ssl_prefer_server_ciphers on;\n")
	} else {
		sb.WriteString(fmt.Sprintf("    listen %d;\n", httpPort))
		sb.WriteString(fmt.Sprintf("    listen [::]:%d;\n", httpPort))
		sb.WriteString(fmt.Sprintf("    server_name %s;\n", w.Domain))
	}

	// 代理站点大文件上传限制（如 10240M）
	if w.Type == "proxy" && w.ClientMaxBodySize != "" {
		sb.WriteString(fmt.Sprintf("    client_max_body_size %s;\n", w.ClientMaxBodySize))
	}

	if w.Type == "proxy" && w.ProxyTarget != "" {
		sb.WriteString("\n    location / {\n")
		sb.WriteString(fmt.Sprintf("        proxy_pass %s;\n", w.ProxyTarget))
		sb.WriteString("        proxy_set_header Host $host;\n")
		sb.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		sb.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		sb.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
		if w.ProxyWS {
			sb.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
			sb.WriteString("        proxy_set_header Connection \"upgrade\";\n")
			sb.WriteString("        proxy_http_version 1.1;\n")
		}
		sb.WriteString("    }\n")
	} else {
		root := w.Root
		if root == "" {
			root = filepath.Join(global.GetDataDir(), "www", websiteDirName(w.Domain))
		}
		_ = os.MkdirAll(root, 0755)
		indexFile := filepath.Join(root, "index.html")
		if _, err := os.Stat(indexFile); os.IsNotExist(err) {
			defaultHtml := fmt.Sprintf(`<html>
		<head><title>Welcome to %s</title></head>
		<body>
		<h1>Website %s is running!</h1>
		<p>Managed by MiniPanel.</p>
		</body>
		</html>`, w.Domain, w.Domain)
			os.WriteFile(indexFile, []byte(defaultHtml), 0644)
		}
		sb.WriteString(fmt.Sprintf("\n    root %s;\n", root))

		// 默认首页
		indexPage := w.IndexPage
		if indexPage == "" {
			indexPage = "index.html index.htm index.php"
		}
		sb.WriteString(fmt.Sprintf("    index %s;\n", indexPage))

		sb.WriteString("\n    location / {\n")
		sb.WriteString("        try_files $uri $uri/ =404;\n")
		sb.WriteString("    }\n")

		// PHP-FPM 配置（PHP 网站类型）
		if w.Type == "php" && w.PhpVersion != "" {
			phpSvc := NewPhpService()
			socket := phpSvc.GetFpmSocket(w.PhpVersion)
			sb.WriteString("\n    # PHP-FPM 处理\n")
			sb.WriteString("    location ~ \\.php$ {\n")
			sb.WriteString(fmt.Sprintf("        fastcgi_pass %s;\n", socket))
			sb.WriteString("        fastcgi_index index.php;\n")
			sb.WriteString("        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;\n")
			sb.WriteString("        include fastcgi_params;\n")
			sb.WriteString("    }\n")
			_ = phpSvc.SetFpmPool(w.PhpVersion, w.Domain, root, w.Port)
		}

		// 安全文件访问控制
		sb.WriteString("\n    # 禁止访问敏感文件\n")
		sb.WriteString("    location ~ ^/(\\.user\\.ini|\\.htaccess|\\.git|\\.svn|\\.env|\\.project|LICENSE|README\\.md|\\.DS_Store) {\n")
		sb.WriteString("        return 404;\n")
		sb.WriteString("    }\n")

		// 静态资源缓存
		sb.WriteString("\n    # 图片/字体 30天缓存\n")
		sb.WriteString("    location ~ .*\\.(gif|jpg|jpeg|png|bmp|swf|ico|svg|woff|woff2|ttf|eot)$ {\n")
		sb.WriteString("        expires 30d;\n")
		sb.WriteString("        access_log off;\n")
		sb.WriteString("    }\n")
		sb.WriteString("\n    # JS/CSS 12小时缓存\n")
		sb.WriteString("    location ~ .*\\.(js|css)$ {\n")
		sb.WriteString("        expires 12h;\n")
		sb.WriteString("        access_log off;\n")
		sb.WriteString("    }\n")
	}

	// 301/302 重定向规则（静态和代理均生效）
	if w.Redirects != "" {
		var rules []model.RedirectRule
		if err := json.Unmarshal([]byte(w.Redirects), &rules); err == nil {
			for _, r := range rules {
				if r.From != "" && r.To != "" && (r.Code == 301 || r.Code == 302) {
					sb.WriteString(fmt.Sprintf("\n    # 重定向 %s -> %s (%d)\n", r.From, r.To, r.Code))
					sb.WriteString(fmt.Sprintf("    if ($host = \"%s\") {\n", r.From))
					sb.WriteString(fmt.Sprintf("        return %d %s$request_uri;\n", r.Code, r.To))
					sb.WriteString("    }\n")
				}
			}
		}
	}

	// 目录密码保护（静态和代理均生效）
	if w.AuthEnabled && w.AuthUser != "" && w.AuthPassword != "" {
		authDir := filepath.Join(global.GetDataDir(), "auth", websiteDirName(w.Domain))
		_ = os.MkdirAll(authDir, 0700)
		htpasswdPath := filepath.Join(authDir, ".htpasswd")
		_ = s.generateHtpasswd(htpasswdPath, w.AuthUser, w.AuthPassword)
		sb.WriteString("\n    # 目录密码保护\n")
		sb.WriteString("    auth_basic \"Restricted Area\";\n")
		sb.WriteString(fmt.Sprintf("    auth_basic_user_file %s;\n", htpasswdPath))
	}

	// 自定义错误页面
	if w.ErrorPage404 != "" || w.ErrorPage502 != "" || w.ErrorPage503 != "" {
		errDir := filepath.Join(global.GetDataDir(), "error", websiteDirName(w.Domain))
		_ = os.MkdirAll(errDir, 0755)
		errPages := map[int]string{404: w.ErrorPage404, 502: w.ErrorPage502, 503: w.ErrorPage503}
		for code, html := range errPages {
			if html == "" {
				continue
			}
			fileName := fmt.Sprintf("%d.html", code)
			_ = os.WriteFile(filepath.Join(errDir, fileName), []byte(html), 0644)
			sb.WriteString(fmt.Sprintf("\n    error_page %d /%s;\n", code, fileName))
		}
		sb.WriteString(fmt.Sprintf("\n    location ~ ^/\\d+\\.html$ {\n        root %s;\n    }\n", errDir))
	}

	// 频率限制（server 块内）
	if w.RateLimitEnabled && w.RateLimitRate != "" {
		zoneName := fmt.Sprintf("zone_%s_%d", websiteDirName(w.Domain), w.Port)
		burst := w.RateLimitBurst
		if burst <= 0 {
			burst = 10
		}
		sb.WriteString(fmt.Sprintf("\n    limit_req zone=%s burst=%d;\n", zoneName, burst))
	}

	// 防盗链
	if w.HotlinkProtection {
		exts := w.HotlinkExts
		if exts == "" {
			exts = "jpg,jpeg,png,gif,svg,webp,js,css,woff,woff2"
		}
		domains := w.HotlinkDomains
		if domains == "" {
			domains = "none blocked"
		} else {
			domains = "none blocked " + domains
		}
		sb.WriteString(fmt.Sprintf("\n    location ~ .*\\.(%s)$ {\n", strings.ReplaceAll(exts, ",", "|")))
		sb.WriteString(fmt.Sprintf("        valid_referers %s;\n", domains))
		sb.WriteString("        if ($invalid_referer) {\n            return 403;\n        }\n")
		sb.WriteString("    }\n")
	}

	// IP 黑白名单
	if w.IPFilterEnabled && w.IPFilterList != "" {
		ips := strings.Split(w.IPFilterList, "\n")
		sb.WriteString("\n    # IP 过滤\n")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if w.IPFilterMode == "whitelist" {
				sb.WriteString(fmt.Sprintf("    allow %s;\n", ip))
			} else {
				sb.WriteString(fmt.Sprintf("    deny %s;\n", ip))
			}
		}
		if w.IPFilterMode == "whitelist" {
			sb.WriteString("    deny all;\n")
		}
	}

	sb.WriteString("}\n")

	return sb.String(), certPath, keyPath
}

func (s *WebsiteService) applyConfig(w *model.Website) error {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	if !syscmd.Which("nginx") {
		global.LOG.Warn("nginx not installed, config saved but not applied")
		return nil
	}

	if !w.Enabled {
		if err := s.removeConfig(w); err != nil {
			return err
		}
		_ = s.syncIsolationConfig("", "")
		return s.ReloadNginx()
	}

	if normalizeWebsitePort(w) {
		_ = s.repo.Update(w)
	}
	confDir := s.GetNginxConfigDir()
	_ = os.MkdirAll(confDir, 0755)

	content, certPath, keyPath := s.buildWebsiteNginxConfig(w)

	path := s.resolveConfigPath(w)
	// 确保父目录存在
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write nginx config failed: %w", err)
	}

	// 写完文件后更新记录中的 ConfigFile，以便后续操作能找到
	if w.ConfigFile != path {
		w.ConfigFile = path
		_ = s.repo.Update(w)
	}

	// 同步默认 catch-all，保证未知域名/IP 被直接断开
	_ = s.syncIsolationConfig(certPath, keyPath)

	if err := s.ReloadNginx(); err != nil {
		global.LOG.Warnf("nginx reload failed: %v", err)
	}
	return nil
}

// sanitizeNginxFileName 将域名转换为安全的配置文件文件名
func sanitizeNginxFileName(s string) string {
	s = strings.TrimSpace(s)
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	if sb.Len() == 0 {
		return "default"
	}
	return sb.String()
}

// normalizeWebsitePort 统一网站端口语义：
// SSL 站点默认/80 规范为 443；非 SSL 站点端口 0 规范为 80；返回端口是否发生变化
func normalizeWebsitePort(w *model.Website) bool {
	old := w.Port
	if w.SSL {
		if w.Port == 0 || w.Port == 80 {
			w.Port = 443
		}
	} else if w.Port == 0 {
		w.Port = 80
	}
	return w.Port != old
}

// nginxListenPorts 计算站点的 HTTP/HTTPS 监听端口：
// SSL 站点固定使用 80 做 301 跳转、443 做 HTTPS；显式配置的非 80/443 端口作为自定义 HTTPS 端口
func nginxListenPorts(w *model.Website) (httpPort, httpsPort int) {
	p := w.Port
	if p == 0 {
		p = 80
	}
	if !w.SSL {
		return p, 0
	}
	httpPort = 80
	httpsPort = 443
	if p != 80 && p != 443 {
		httpsPort = p
	}
	return httpPort, httpsPort
}

const (
	minipanelManagedMarker  = "# Managed by MiniPanel"
	isolationConfigFileName = "00-minipanel-default.conf"
)

func (s *WebsiteService) isolationConfigPath() string {
	return filepath.Join(s.GetNginxConfigDir(), isolationConfigFileName)
}

// isPanelManagedConfig 判断配置文件是否由 MiniPanel 生成
func (s *WebsiteService) isPanelManagedConfig(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), minipanelManagedMarker)
}

// countDistinctServerNames 统计配置文件中非 _ 的 server_name 数量
func (s *WebsiteService) countDistinctServerNames(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	names := make(map[string]struct{})
	re := regexp.MustCompile(`(?m)server_name\s+([^;]+);`)
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		for _, n := range strings.Fields(m[1]) {
			n = strings.TrimSpace(n)
			if n != "" && n != "_" {
				names[n] = struct{}{}
			}
		}
	}
	return len(names)
}

// safeToOverwriteConfig 判断现有配置文件是否可以被面板安全覆盖：
// 仅允许覆盖 MiniPanel 生成的、且只包含本站点的单站点文件；
// 手动维护或包含多个站点的共享文件一律保留，另行写入独立文件
func (s *WebsiteService) safeToOverwriteConfig(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}
	if !s.isPanelManagedConfig(path) {
		return false
	}
	return s.countDistinctServerNames(path) <= 1
}

// resolveConfigPath 确定配置文件最终写入路径：
// 已有的面板单站点 ConfigFile 直接复用；手动维护/共享配置则写入面板默认目录的独立文件
func (s *WebsiteService) resolveConfigPath(w *model.Website) string {
	if w.ConfigFile != "" && s.safeToOverwriteConfig(w.ConfigFile) {
		return w.ConfigFile
	}
	return s.nginxConfPath(w)
}

// hasExternalDefaultServer 检查其他配置文件是否已声明 default_server，避免重复声明导致 nginx 启动失败
func (s *WebsiteService) hasExternalDefaultServer() bool {
	dirs := []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/usr/local/nginx/conf/conf.d",
		"/usr/local/nginx/conf/vhost",
	}
	dirs = append(dirs, s.GetNginxConfigDir())
	re := regexp.MustCompile(`(?m)listen\s+[^;]*default_server`)
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
				continue
			}
			if e.Name() == isolationConfigFileName {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if re.Match(data) {
				return true
			}
		}
	}
	if data, err := os.ReadFile("/etc/nginx/nginx.conf"); err == nil {
		if re.Match(data) {
			return true
		}
	}
	return false
}

// findAvailableSSLCert 从启用中的 SSL 站点里找一组可用的证书，用于默认 catch-all 的 443 监听
func (s *WebsiteService) findAvailableSSLCert() (string, string) {
	if s.repo == nil {
		return "", ""
	}
	sites, err := s.repo.List()
	if err != nil {
		return "", ""
	}
	for i := range sites {
		if !sites[i].Enabled || !sites[i].SSL {
			continue
		}
		cert, key := sites[i].SSLCert, sites[i].SSLKey
		if sites[i].SSLCertPEM != "" || sites[i].SSLKeyPEM != "" {
			cert = filepath.Join(global.GetDataDir(), "ssl", sites[i].Domain, "fullchain.pem")
			key = filepath.Join(global.GetDataDir(), "ssl", sites[i].Domain, "privkey.pem")
		}
		if cert == "" || key == "" {
			continue
		}
		if _, err := os.Stat(cert); err == nil {
			if _, err := os.Stat(key); err == nil {
				return cert, key
			}
		}
	}
	return "", ""
}

// syncIsolationConfig 生成/更新默认 catch-all 配置，实现未知域名/IP 直接断开（站点隔离）
// 调用方需持有 configMu；无可用 SSL 证书时仅保留 80 端口 catch-all
func (s *WebsiteService) syncIsolationConfig(certPath, keyPath string) error {
	confDir := s.GetNginxConfigDir()
	_ = os.MkdirAll(confDir, 0755)
	path := s.isolationConfigPath()

	// 其他配置文件已声明 default_server 时不再写入，避免 nginx 报 duplicate default server
	if s.hasExternalDefaultServer() {
		_ = os.Remove(path)
		return nil
	}

	if certPath == "" || keyPath == "" {
		certPath, keyPath = s.findAvailableSSLCert()
	}

	var sb strings.Builder
	sb.WriteString("# Managed by MiniPanel - default catch-all (site isolation)\n")
	sb.WriteString("server {\n")
	sb.WriteString("    listen 80 default_server;\n")
	sb.WriteString("    listen [::]:80 default_server;\n")
	sb.WriteString("    server_name _;\n")
	sb.WriteString("    return 444;\n")
	sb.WriteString("}\n\n")
	if certPath != "" && keyPath != "" {
		sb.WriteString("server {\n")
		sb.WriteString("    listen 443 ssl http2 default_server;\n")
		sb.WriteString("    listen [::]:443 ssl http2 default_server;\n")
		sb.WriteString("    server_name _;\n")
		sb.WriteString(fmt.Sprintf("    ssl_certificate %s;\n", certPath))
		sb.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n", keyPath))
		sb.WriteString("    return 444;\n")
		sb.WriteString("}\n")
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// generateHtpasswd 生成 htpasswd 文件（优先 htpasswd 命令，回退 openssl）
func (s *WebsiteService) generateHtpasswd(path, user, password string) error {
	// 尝试使用 htpasswd 命令
	if syscmd.Which("htpasswd") {
		cmd := exec.Command("htpasswd", "-bc", path, user, password)
		if err := cmd.Run(); err == nil {
			os.Chmod(path, 0600)
			return nil
		}
	}
	// 回退：使用 openssl 生成 crypt 密码
	if syscmd.Which("openssl") {
		hash, err := exec.Command("openssl", "passwd", "-crypt", password).CombinedOutput()
		if err == nil {
			line := fmt.Sprintf("%s:%s\n", user, strings.TrimSpace(string(hash)))
			return os.WriteFile(path, []byte(line), 0600)
		}
	}
	// 最后回退：明文存储
	line := fmt.Sprintf("%s:%s\n", user, password)
	return os.WriteFile(path, []byte(line), 0600)
}

// AccessLogEntry 访问日志条目
type AccessLogEntry struct {
	Time       string `json:"time"`
	IP         string `json:"ip"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Size       int64  `json:"size"`
	Referer    string `json:"referer"`
	UserAgent  string `json:"user_agent"`
}

// AccessLogFilter 日志过滤条件
type AccessLogFilter struct {
	Date       string `json:"date"`        // 日期过滤 "2026-07-06"
	IP         string `json:"ip"`          // IP 过滤
	StatusCode string `json:"status_code"` // 状态码过滤
	URL        string `json:"url"`         // URL 关键词
	Page       int    `json:"page"`        // 页码，从 1 开始
	PageSize   int    `json:"page_size"`   // 每页条数
}

// ParseAccessLogs 解析网站访问日志
func (s *WebsiteService) ParseAccessLogs(w *model.Website, filters AccessLogFilter) ([]AccessLogEntry, int64, error) {
	logPath := s.findAccessLogPath(w.Domain)
	if logPath == "" {
		return nil, 0, fmt.Errorf("找不到 %s 的访问日志", w.Domain)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, 0, fmt.Errorf("读取日志失败: %w", err)
	}

	// combined log 正则
	logRe := regexp.MustCompile(`^(\S+) - \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) "([^"]*)" "([^"]*)"`)
	lines := strings.Split(string(data), "\n")

	var allEntries []AccessLogEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := logRe.FindStringSubmatch(line)
		if len(m) < 9 {
			continue
		}
		statusCode, _ := strconv.Atoi(m[5])
		size, _ := strconv.ParseInt(m[6], 10, 64)
		entry := AccessLogEntry{
			IP:         m[1],
			Time:       m[2],
			Method:     m[3],
			URL:        m[4],
			StatusCode: statusCode,
			Size:       size,
			Referer:    m[7],
			UserAgent:  m[8],
		}

		// 过滤
		if filters.Date != "" && !strings.HasPrefix(entry.Time, filters.Date) {
			continue
		}
		if filters.IP != "" && entry.IP != filters.IP {
			continue
		}
		if filters.StatusCode != "" {
			code, _ := strconv.Atoi(filters.StatusCode)
			if entry.StatusCode != code {
				continue
			}
		}
		if filters.URL != "" && !strings.Contains(entry.URL, filters.URL) {
			continue
		}
		allEntries = append(allEntries, entry)
	}

	total := int64(len(allEntries))

	// 分页（倒序，最新在前）
	if filters.PageSize <= 0 {
		filters.PageSize = 50
	}
	if filters.Page <= 0 {
		filters.Page = 1
	}
	start := int64(len(allEntries)) - int64(filters.Page)*int64(filters.PageSize)
	end := start + int64(filters.PageSize)
	if start < 0 {
		start = 0
	}
	if end > int64(len(allEntries)) {
		end = int64(len(allEntries))
	}

	// 反转结果（最新在前）
	var result []AccessLogEntry
	for i := end - 1; i >= start; i-- {
		result = append(result, allEntries[i])
	}

	return result, total, nil
}

// findAccessLogPath 查找站点的访问日志文件
func (s *WebsiteService) findAccessLogPath(domain string) string {
	candidates := []string{
		fmt.Sprintf("/var/log/nginx/%s.access.log", domain),
		"/var/log/nginx/access.log",
		"/usr/local/nginx/logs/access.log",
		"/var/log/nginx/access_log",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TrafficPoint 流量统计点
type TrafficPoint struct {
	Time      string `json:"time"`
	Requests  int64  `json:"requests"`
	Bandwidth int64  `json:"bandwidth"`
}

// GetTrafficStats 获取网站流量统计（按小时聚合）
func (s *WebsiteService) GetTrafficStats(w *model.Website, period string) ([]TrafficPoint, error) {
	logPath := s.findAccessLogPath(w.Domain)
	if logPath == "" {
		return nil, fmt.Errorf("找不到 %s 的访问日志", w.Domain)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, fmt.Errorf("读取日志失败: %w", err)
	}

	logRe := regexp.MustCompile(`^(\S+) - \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) "([^"]*)" "([^"]*)"`)
	lines := strings.Split(string(data), "\n")

	// 确定时间桶大小
	bucketHours := 1
	lookbackHours := 24
	switch period {
	case "7d":
		lookbackHours = 168
		bucketHours = 6
	case "30d":
		lookbackHours = 720
		bucketHours = 24
	}

	// 解析并聚合
	buckets := make(map[string]*TrafficPoint)
	now := time.Now()
	cutoff := now.Add(-time.Duration(lookbackHours) * time.Hour)

	timeRe := regexp.MustCompile(`(\d{2})/(\w{3})/(\d{4}):(\d{2}):(\d{2})`)
	monthMap := map[string]time.Month{
		"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4, "May": 5, "Jun": 6,
		"Jul": 7, "Aug": 8, "Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := logRe.FindStringSubmatch(line)
		if len(m) < 9 {
			continue
		}

		// 解析时间
		tm := timeRe.FindStringSubmatch(m[2])
		if len(tm) < 6 {
			continue
		}
		day, _ := strconv.Atoi(tm[1])
		month := monthMap[tm[2]]
		year, _ := strconv.Atoi(tm[3])
		hour, _ := strconv.Atoi(tm[4])
		minute, _ := strconv.Atoi(tm[5])
		logTime := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)

		if logTime.Before(cutoff) {
			continue
		}

		// 按 bucket 对齐
		bucketHour := (logTime.Hour() / bucketHours) * bucketHours
		key := logTime.Format("2006-01-02") + fmt.Sprintf(" %02d:00", bucketHour)

		if buckets[key] == nil {
			buckets[key] = &TrafficPoint{Time: key}
		}
		buckets[key].Requests++
		size, _ := strconv.ParseInt(m[6], 10, 64)
		buckets[key].Bandwidth += size
	}

	// 按时间排序
	var result []TrafficPoint
	for _, v := range buckets {
		result = append(result, *v)
	}
	// 简单冒泡排序
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Time > result[j].Time {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}
