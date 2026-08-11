package service

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	syscmd "github.com/minipanel/minipanel/internal/utils/cmd"
)

type DatabaseService struct {
	repo   *repository.DatabaseRepository
	wdRepo *repository.WebsiteDatabaseRepository
}

func NewDatabaseService() *DatabaseService {
	return &DatabaseService{
		repo:   repository.NewDatabaseRepository(global.DB),
		wdRepo: repository.NewWebsiteDatabaseRepository(global.DB),
	}
}

// maskPassword 脱敏：密码只显示首尾各1位，中间用 *** 替代
func maskPassword(pwd string) string {
	if pwd == "" {
		return "(empty)"
	}
	if len(pwd) <= 2 {
		return "***"
	}
	return pwd[:1] + "***" + pwd[len(pwd)-1:]
}

// escapeIdent 转义 MySQL 标识符（库名/表名）：将内部反引号翻倍
func escapeIdent(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}

// escapeStr 转义 MySQL 字符串字面量（用户名/密码）：将反斜杠和单引号翻倍
func escapeStr(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "''")
	return s
}

// logItem 打印实例连接信息（密码脱敏）
func logItem(prefix string, item *model.DatabaseInstance) {
	global.LOG.Infof("[DB] %s id=%d name=%s type=%s host=%s port=%d user=%s pass=%s db=%s",
		prefix, item.ID, item.Name, item.Type, item.Host, item.Port, item.Username, maskPassword(item.Password), item.Database)
}

func (s *DatabaseService) Create(item *model.DatabaseInstance) error {
	if item.Host == "" {
		item.Host = "127.0.0.1"
	}
	if item.Type == "" {
		item.Type = "mysql"
	}
	switch item.Type {
	case "sqlite":
		item.Port = 0
		item.Username = ""
		item.Password = ""
		if item.Database == "" {
			item.Database = filepath.Join(global.GetDataDir(), "sqlite")
		}
	default:
		if item.Port == 0 {
			item.Port = 3306
		}
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		return fmt.Errorf("数据库实例名称不能为空")
	}
	// 检查重名（仅活跃记录）
	existing, _ := s.repo.GetByName(item.Name)
	if existing != nil && existing.ID > 0 {
		global.LOG.Warnf("[DB] Create rejected: name=%s already exists (id=%d)", item.Name, existing.ID)
		return fmt.Errorf("数据库实例名称 '%s' 已存在", item.Name)
	}
	// 检查是否存在同名软删除记录（uniqueIndex 不区分软删除，会导致插入失败）
	softDeleted, _ := s.repo.GetByNameWithUnscoped(item.Name)
	if softDeleted != nil && softDeleted.ID > 0 && softDeleted.DeletedAt.Valid {
		global.LOG.Infof("[DB] Create: 检测到同名软删除记录 (id=%d name=%s)，物理删除以释放唯一索引", softDeleted.ID, item.Name)
		if err := s.repo.RestoreSoftDeleted(item.Name); err != nil {
			global.LOG.Errorf("[DB] Create: 清理软删除记录失败: %v", err)
			return fmt.Errorf("清理历史同名记录失败: %v", err)
		}
	}
	if err := s.repo.Create(item); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			global.LOG.Warnf("[DB] Create UNIQUE constraint: name=%s", item.Name)
			return fmt.Errorf("数据库实例名称 '%s' 已存在", item.Name)
		}
		global.LOG.Errorf("[DB] Create failed: name=%s err=%v", item.Name, err)
		return err
	}
	global.LOG.Infof("[DB] Create OK: id=%d name=%s type=%s host=%s port=%d user=%s",
		item.ID, item.Name, item.Type, item.Host, item.Port, item.Username)
	return nil
}

func (s *DatabaseService) List() ([]DatabaseInstanceWithCount, error) {
	var items []DatabaseInstanceWithCount
	err := global.DB.Model(&model.DatabaseInstance{}).
		Select("database_instances.*, (SELECT COUNT(*) FROM website_databases WHERE website_databases.db_instance_id = database_instances.id AND website_databases.deleted_at IS NULL) as website_count").
		Order("id DESC").
		Find(&items).Error
	return items, err
}

// DatabaseInstanceWithCount 在 DatabaseInstance 基础上附加关联网站数量
type DatabaseInstanceWithCount struct {
	model.DatabaseInstance
	WebsiteCount int64 `json:"website_count" gorm:"-"`
}

func (s *DatabaseService) GetByID(id uint) (*model.DatabaseInstance, error) {
	return s.repo.GetByID(id)
}

func (s *DatabaseService) Update(item *model.DatabaseInstance) error {
	return s.repo.Update(item)
}

func (s *DatabaseService) Delete(id uint) error {
	global.LOG.Infof("[DB] Delete start: id=%d", id)
	// 引用检查：若有关联网站，拒绝删除以保护数据
	count, err := s.wdRepo.CountByInstanceID(id)
	if err != nil {
		global.LOG.Errorf("[DB] Delete reference check failed: id=%d err=%v", id, err)
		return fmt.Errorf("检查数据库实例引用失败: %v", err)
	}
	if count > 0 {
		global.LOG.Warnf("[DB] Delete rejected: id=%d is referenced by %d website(s)", id, count)
		return fmt.Errorf("该数据库实例被 %d 个网站关联，请先解除关联（删除对应网站）后再删除", count)
	}
	if err := s.repo.Delete(id); err != nil {
		global.LOG.Errorf("[DB] Delete failed: id=%d err=%v", id, err)
		return err
	}
	global.LOG.Infof("[DB] Delete OK: id=%d", id)
	return nil
}

func (s *DatabaseService) getMysqlArgs(item *model.DatabaseInstance, dbName string) []string {
	args := []string{
		fmt.Sprintf("-h%s", item.Host),
		fmt.Sprintf("-P%d", item.Port),
		fmt.Sprintf("-u%s", item.Username),
	}
	if dbName != "" {
		args = append(args, dbName)
	}
	return args
}

// runMysqlCmd 安全执行 mysql 命令（通过环境变量传密码，避免命令行暴露）
func (s *DatabaseService) runMysqlCmd(item *model.DatabaseInstance, args ...string) ([]byte, error) {
	global.LOG.Debugf("[DB] mysql CLI exec: host=%s port=%d user=%s pass=%s args=%v",
		item.Host, item.Port, item.Username, maskPassword(item.Password), args)
	start := time.Now()
	cmd := exec.Command("mysql", args...)
	if item.Password != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", item.Password))
	}
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	output := strings.TrimSpace(string(out))
	if len(output) > 500 {
		output = output[:500] + "...(truncated)"
	}
	if err != nil {
		global.LOG.Errorf("[DB] mysql CLI FAILED (%.2fs): %v | output: %s", elapsed.Seconds(), err, output)
	} else {
		global.LOG.Debugf("[DB] mysql CLI OK (%.2fs): %s", elapsed.Seconds(), output)
	}
	return out, err
}

// sqliteDataDir 返回 SQLite 数据目录（不存在则自动创建）
func (s *DatabaseService) sqliteDataDir(item *model.DatabaseInstance) (string, error) {
	dir := strings.TrimSpace(item.Database)
	if dir == "" {
		dir = filepath.Join(global.GetDataDir(), "sqlite")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建 SQLite 数据目录失败: %v", err)
	}
	return dir, nil
}

// sqliteFilePath 返回指定 SQLite 数据库文件的完整路径（自动补 .db 后缀并防路径穿越）
func (s *DatabaseService) sqliteFilePath(item *model.DatabaseInstance, dbName string) (string, error) {
	dir, err := s.sqliteDataDir(item)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(dbName)
	if name == "" {
		return "", fmt.Errorf("数据库名不能为空")
	}
	if !strings.HasSuffix(name, ".db") && !strings.HasSuffix(name, ".sqlite") && !strings.HasSuffix(name, ".sqlite3") {
		name += ".db"
	}
	if filepath.Base(name) != name {
		return "", fmt.Errorf("非法的数据库文件名: %s", dbName)
	}
	return filepath.Join(dir, name), nil
}

// runSqliteCmd 执行 sqlite3 命令。调用方需保证参数顺序：
// 选项（如 -header）在前，数据库文件路径在中间，SQL 在最后。
func (s *DatabaseService) runSqliteCmd(args ...string) ([]byte, error) {
	global.LOG.Debugf("[DB] sqlite3 CLI exec: args=%v", args)
	start := time.Now()
	cmd := exec.Command("sqlite3", args...)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	output := strings.TrimSpace(string(out))
	if len(output) > 500 {
		output = output[:500] + "...(truncated)"
	}
	if err != nil {
		global.LOG.Errorf("[DB] sqlite3 CLI FAILED (%.2fs): %v | output: %s", elapsed.Seconds(), err, output)
	} else {
		global.LOG.Debugf("[DB] sqlite3 CLI OK (%.2fs): %s", elapsed.Seconds(), output)
	}
	return out, err
}

func (s *DatabaseService) TestConnection(item *model.DatabaseInstance) (string, error) {
	logItem("TestConnection start", item)
	if item.Type == "sqlite" {
		if !syscmd.Which("sqlite3") {
			return "", fmt.Errorf("sqlite3 未安装，请先安装 SQLite")
		}
		if _, err := s.sqliteDataDir(item); err != nil {
			return "", err
		}
		global.LOG.Infof("[DB] TestConnection SQLite OK: dir=%s", item.Database)
		return "SQLite 就绪（嵌入式数据库，无需网络连接）", nil
	}
	addr := net.JoinHostPort(item.Host, fmt.Sprintf("%d", item.Port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		global.LOG.Errorf("[DB] TestConnection TCP dial failed: %v", err)
		return "", fmt.Errorf("connection failed: %v", err)
	}
	conn.Close()
	global.LOG.Infof("[DB] TestConnection TCP OK: %s", addr)

	if item.Type == "mysql" && item.Username != "" && syscmd.Which("mysql") {
		args := s.getMysqlArgs(item, "")
		args = append(args, "-e", "SELECT 1")
		if out, err := s.runMysqlCmd(item, args...); err != nil {
			global.LOG.Errorf("[DB] TestConnection MySQL auth failed: %v", err)
			return "", fmt.Errorf("mysql auth failed: %s: %v", string(out), err)
		}
		global.LOG.Infof("[DB] TestConnection MySQL auth OK")
		return "Connection successful", nil
	}
	global.LOG.Infof("[DB] TestConnection result: TCP only (type=%s)", item.Type)
	return "TCP connection successful", nil
}

func (s *DatabaseService) CreateDatabase(item *model.DatabaseInstance, dbName string) error {
	logItem("CreateDatabase start", item)
	global.LOG.Infof("[DB] CreateDatabase target: db=%s", dbName)
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return fmt.Errorf("mysql client not found, please install mysql first")
		}
		args := s.getMysqlArgs(item, "")
		query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", escapeIdent(dbName))
		args = append(args, "-e", query)
		if out, err := s.runMysqlCmd(item, args...); err != nil {
			global.LOG.Errorf("[DB] CreateDatabase FAILED: db=%s err=%v", dbName, err)
			return fmt.Errorf("create database failed: %s: %v", string(out), err)
		}
	case "sqlite":
		if !syscmd.Which("sqlite3") {
			return fmt.Errorf("sqlite3 未安装，请先安装 SQLite")
		}
		path, err := s.sqliteFilePath(item, dbName)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("SQLite 数据库文件已存在: %s", path)
		}
		if out, err := s.runSqliteCmd(path, "VACUUM;"); err != nil {
			return fmt.Errorf("create sqlite database failed: %s: %v", string(out), err)
		}
	default:
		return fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] CreateDatabase OK: db=%s", dbName)
	return nil
}

func (s *DatabaseService) CreateUser(item *model.DatabaseInstance, username, password string, privDB string) error {
	logItem("CreateUser start", item)
	global.LOG.Infof("[DB] CreateUser target: user=%s priv=%s", username, privDB)
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return fmt.Errorf("mysql client not found, please install mysql first")
		}
		if privDB == "" || privDB == "*" {
			privDB = "*.*"
		} else {
			privDB = fmt.Sprintf("`%s`.*", escapeIdent(privDB))
		}
		queries := []string{
			fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", escapeStr(username), escapeStr(password)),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON %s TO '%s'@'%%'", privDB, escapeStr(username)),
			"FLUSH PRIVILEGES",
		}
		fullQuery := strings.Join(queries, "; ")
		args := s.getMysqlArgs(item, "")
		args = append(args, "-e", fullQuery)
		if out, err := s.runMysqlCmd(item, args...); err != nil {
			global.LOG.Errorf("[DB] CreateUser FAILED: user=%s err=%v", username, err)
			return fmt.Errorf("create user failed: %s: %v", string(out), err)
		}
	case "sqlite":
		return fmt.Errorf("SQLite 不支持用户管理")
	default:
		return fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] CreateUser OK: user=%s", username)
	return nil
}

// DropDatabase 删除指定数据库
func (s *DatabaseService) DropDatabase(item *model.DatabaseInstance, dbName string) error {
	logItem("DropDatabase start", item)
	global.LOG.Infof("[DB] DropDatabase target: db=%s", dbName)
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return fmt.Errorf("mysql client not found, please install mysql first")
		}
		args := s.getMysqlArgs(item, "")
		query := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", escapeIdent(dbName))
		args = append(args, "-e", query)
		if out, err := s.runMysqlCmd(item, args...); err != nil {
			global.LOG.Errorf("[DB] DropDatabase FAILED: db=%s err=%v", dbName, err)
			return fmt.Errorf("drop database failed: %s: %v", string(out), err)
		}
	case "sqlite":
		path, err := s.sqliteFilePath(item, dbName)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("删除 SQLite 数据库文件失败: %v", err)
		}
	default:
		return fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] DropDatabase OK: db=%s", dbName)
	return nil
}

// DropUser 删除指定 MySQL 用户
func (s *DatabaseService) DropUser(item *model.DatabaseInstance, username string) error {
	logItem("DropUser start", item)
	global.LOG.Infof("[DB] DropUser target: user=%s", username)
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return fmt.Errorf("mysql client not found, please install mysql first")
		}
		args := s.getMysqlArgs(item, "")
		queries := []string{
			fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", escapeStr(username)),
			"FLUSH PRIVILEGES",
		}
		fullQuery := strings.Join(queries, "; ")
		args = append(args, "-e", fullQuery)
		if out, err := s.runMysqlCmd(item, args...); err != nil {
			global.LOG.Errorf("[DB] DropUser FAILED: user=%s err=%v", username, err)
			return fmt.Errorf("drop user failed: %s: %v", string(out), err)
		}
	case "sqlite":
		return fmt.Errorf("SQLite 不支持用户管理")
	default:
		return fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] DropUser OK: user=%s", username)
	return nil
}

// CreateDatabaseForWebsite 一站式建库+建用户+授权，供 WebsiteService 调用
// 任一步骤失败都会回滚（删除已建的库或用户）
func (s *DatabaseService) CreateDatabaseForWebsite(item *model.DatabaseInstance, dbName, username, password string) error {
	logItem("CreateDatabaseForWebsite start", item)
	global.LOG.Infof("[DB] CreateDatabaseForWebsite db=%s user=%s", dbName, username)
	if item.Type != "mysql" {
		return fmt.Errorf("数据库类型 %s 不支持联动建库", item.Type)
	}

	if err := s.CreateDatabase(item, dbName); err != nil {
		return fmt.Errorf("建库失败: %v", err)
	}
	if err := s.CreateUser(item, username, password, dbName); err != nil {
		// 建用户失败，回滚已建的库
		global.LOG.Warnf("[DB] CreateDatabaseForWebsite rollback: dropping db=%s due to user creation failure", dbName)
		if dropErr := s.DropDatabase(item, dbName); dropErr != nil {
			global.LOG.Errorf("[DB] rollback drop db failed: %v (db=%s remains, please clean manually)", dropErr, dbName)
		}
		return fmt.Errorf("建用户失败: %v", err)
	}
	global.LOG.Infof("[DB] CreateDatabaseForWebsite OK: db=%s user=%s", dbName, username)
	return nil
}

type DBInfo struct {
	Name string `json:"name"`
}

func (s *DatabaseService) ListDatabases(item *model.DatabaseInstance) ([]DBInfo, error) {
	logItem("ListDatabases start", item)
	var dbs []DBInfo
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return nil, fmt.Errorf("mysql client not found, please install mysql first")
		}
		args := s.getMysqlArgs(item, "")
		args = append(args, "-N", "-B", "-e", "SHOW DATABASES")
		out, err := s.runMysqlCmd(item, args...)
		if err != nil {
			global.LOG.Errorf("[DB] ListDatabases FAILED: %v", err)
			return nil, fmt.Errorf("list databases failed: %s: %v", string(out), err)
		}
		skip := map[string]bool{
			"information_schema": true,
			"performance_schema": true,
			"sys":                true,
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || skip[line] {
				continue
			}
			dbs = append(dbs, DBInfo{Name: line})
		}
	case "sqlite":
		if !syscmd.Which("sqlite3") {
			return nil, fmt.Errorf("sqlite3 未安装，请先安装 SQLite")
		}
		dir, err := s.sqliteDataDir(item)
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("读取 SQLite 数据目录失败: %v", err)
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".sqlite") || strings.HasSuffix(name, ".sqlite3") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			dbs = append(dbs, DBInfo{Name: name})
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] ListDatabases OK: count=%d", len(dbs))
	return dbs, nil
}

type TableInfo struct {
	Name string `json:"name"`
}

func (s *DatabaseService) ListTables(item *model.DatabaseInstance, dbName string) ([]TableInfo, error) {
	logItem("ListTables start", item)
	if dbName == "" {
		return nil, fmt.Errorf("please select a database first")
	}
	var tables []TableInfo
	var out []byte
	var err error
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return nil, fmt.Errorf("mysql client not found, please install mysql first")
		}
		args := s.getMysqlArgs(item, dbName)
		args = append(args, "-N", "-B", "-e", "SHOW TABLES")
		out, err = s.runMysqlCmd(item, args...)
	case "sqlite":
		if !syscmd.Which("sqlite3") {
			return nil, fmt.Errorf("sqlite3 未安装，请先安装 SQLite")
		}
		path, pathErr := s.sqliteFilePath(item, dbName)
		if pathErr != nil {
			return nil, pathErr
		}
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("SQLite 数据库文件不存在: %s", path)
		}
		out, err = s.runSqliteCmd(path,
			"SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name;")
	default:
		return nil, fmt.Errorf("unsupported database type: %s", item.Type)
	}
	if err != nil {
		global.LOG.Errorf("[DB] ListTables FAILED: db=%s err=%v", dbName, err)
		return nil, fmt.Errorf("list tables failed: %s: %v", string(out), err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tables = append(tables, TableInfo{Name: line})
		}
	}
	global.LOG.Infof("[DB] ListTables OK: db=%s count=%d", dbName, len(tables))
	return tables, nil
}

func (s *DatabaseService) ChangePassword(item *model.DatabaseInstance, newPassword string) error {
	logItem("ChangePassword start", item)
	global.LOG.Infof("[DB] ChangePassword target: user=%s new_pass=%s", item.Username, maskPassword(newPassword))
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return fmt.Errorf("mysql client not found")
		}
		args := s.getMysqlArgs(item, "")
		query := fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'", escapeStr(item.Username), escapeStr(newPassword))
		args = append(args, "-e", query)
		if out, err := s.runMysqlCmd(item, args...); err != nil {
			global.LOG.Errorf("[DB] ChangePassword FAILED: user=%s err=%v", item.Username, err)
			return fmt.Errorf("change password failed: %s: %v", string(out), err)
		}
	case "sqlite":
		return fmt.Errorf("SQLite 不支持用户密码管理")
	default:
		return fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] ChangePassword OK: user=%s", item.Username)
	return nil
}

// ColumnInfo 表列信息
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Null    string `json:"null"`
	Key     string `json:"key"`
	Default string `json:"default"`
	Extra   string `json:"extra"`
}

// DescribeTable 查看表结构
func (s *DatabaseService) DescribeTable(item *model.DatabaseInstance, dbName, tableName string) ([]ColumnInfo, error) {
	logItem("DescribeTable start", item)
	global.LOG.Infof("[DB] DescribeTable db=%s table=%s", dbName, tableName)
	var cols []ColumnInfo
	var out []byte
	var err error
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return nil, fmt.Errorf("mysql client not found")
		}
		args := s.getMysqlArgs(item, dbName)
		args = append(args, "-N", "-B", "-e", fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`", escapeIdent(tableName)))
		out, err = s.runMysqlCmd(item, args...)
		if err != nil {
			return nil, fmt.Errorf("describe table failed: %s: %v", string(out), err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) < 6 {
				continue
			}
			cols = append(cols, ColumnInfo{
				Name:    fields[0],
				Type:    fields[1],
				Null:    fields[3],
				Key:     fields[4],
				Default: fields[5],
				Extra:   fields[6],
			})
		}
	case "sqlite":
		if !syscmd.Which("sqlite3") {
			return nil, fmt.Errorf("sqlite3 未安装")
		}
		path, err := s.sqliteFilePath(item, dbName)
		if err != nil {
			return nil, err
		}
		out, err = s.runSqliteCmd(path, fmt.Sprintf("PRAGMA table_info(\"%s\");", strings.ReplaceAll(tableName, `"`, `""`)))
		if err != nil {
			return nil, fmt.Errorf("describe table failed: %s: %v", string(out), err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Split(line, "|")
			if len(fields) < 6 {
				continue
			}
			key := ""
			if strings.TrimSpace(fields[5]) != "0" {
				key = "PRI"
			}
			null := "NO"
			if strings.TrimSpace(fields[3]) == "0" {
				null = "YES"
			} else {
				null = "NO"
			}
			cols = append(cols, ColumnInfo{
				Name:    fields[1],
				Type:    fields[2],
				Null:    null,
				Key:     key,
				Default: fields[4],
				Extra:   "",
			})
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] DescribeTable OK: db=%s table=%s cols=%d", dbName, tableName, len(cols))
	return cols, nil
}

// QueryResult SQL 查询结果
type QueryResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// ExecuteQuery 执行 SQL 查询（仅允许只读操作）
func (s *DatabaseService) ExecuteQuery(item *model.DatabaseInstance, dbName, query string) (*QueryResult, error) {
	logItem("ExecuteQuery start", item)
	global.LOG.Infof("[DB] ExecuteQuery db=%s query=%s", dbName, query)
	query = strings.TrimSpace(query)
	upper := strings.ToUpper(query)
	if strings.HasPrefix(query, ".") {
		return nil, fmt.Errorf("不允许执行 sqlite 点命令，仅支持 SELECT/SHOW/DESCRIBE/EXPLAIN 查询")
	}
	// 安全检查：仅允许只读操作
	for _, keyword := range []string{"DROP", "DELETE", "UPDATE", "INSERT", "ALTER", "CREATE", "TRUNCATE", "RENAME", "REPLACE"} {
		if strings.HasPrefix(upper, keyword) || strings.Contains(upper, " "+keyword+" ") {
			return nil, fmt.Errorf("不允许执行 %s 操作，仅支持 SELECT/SHOW/DESCRIBE/EXPLAIN 查询", keyword)
		}
	}
	var out []byte
	var err error
	var columns []string
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return nil, fmt.Errorf("mysql client not found")
		}
		args := s.getMysqlArgs(item, dbName)
		args = append(args, "-N", "-B", "-e", query)
		out, err = s.runMysqlCmd(item, args...)
		if err != nil {
			return nil, fmt.Errorf("query failed: %s: %v", string(out), err)
		}
		// 获取列名
		colArgs2 := s.getMysqlArgs(item, dbName)
		colArgs2 = append(colArgs2, "-e", query+" LIMIT 0")
		colOut, colErr := s.runMysqlCmd(item, colArgs2...)
		if colErr == nil {
			colLines := strings.Split(strings.TrimSpace(string(colOut)), "\n")
			if len(colLines) > 0 {
				columns = strings.Split(colLines[0], "\t")
			}
		}
	case "sqlite":
		if !syscmd.Which("sqlite3") {
			return nil, fmt.Errorf("sqlite3 未安装")
		}
		path, pathErr := s.sqliteFilePath(item, dbName)
		if pathErr != nil {
			return nil, pathErr
		}
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("SQLite 数据库文件不存在: %s", path)
		}
		out, err = s.runSqliteCmd("-header", "-separator", "\t", path, query)
		if err != nil {
			return nil, fmt.Errorf("query failed: %s: %v", string(out), err)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 {
			columns = strings.Split(lines[0], "\t")
			lines = lines[1:]
			out = []byte(strings.Join(lines, "\n"))
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s", item.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("query failed: %s: %v", string(out), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var rows [][]string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}

	global.LOG.Infof("[DB] ExecuteQuery OK: db=%s rows=%d cols=%d", dbName, len(rows), len(columns))
	return &QueryResult{Columns: columns, Rows: rows}, nil
}

// BackupDatabase 使用 mysqldump 备份数据库
func (s *DatabaseService) BackupDatabase(item *model.DatabaseInstance, dbName string) (string, error) {
	logItem("BackupDatabase start", item)
	global.LOG.Infof("[DB] BackupDatabase db=%s", dbName)
	backupDir := filepath.Join(global.GetDataDir(), "backups", "database")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir failed: %v", err)
	}
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	outputPath := filepath.Join(backupDir, fileName)

	start := time.Now()
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysqldump") {
			return "", fmt.Errorf("mysqldump not found, please install mysql client")
		}
		args := []string{
			fmt.Sprintf("-h%s", item.Host),
			fmt.Sprintf("-P%d", item.Port),
			fmt.Sprintf("-u%s", item.Username),
			"--single-transaction",
			"--routines",
			"--triggers",
			dbName,
		}
		global.LOG.Debugf("[DB] mysqldump exec: host=%s port=%d user=%s db=%s output=%s",
			item.Host, item.Port, item.Username, dbName, outputPath)
		cmd := exec.Command("mysqldump", args...)
		if item.Password != "" {
			cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", item.Password))
		}
		outFile, err := os.Create(outputPath)
		if err != nil {
			return "", fmt.Errorf("create output file failed: %v", err)
		}
		cmd.Stdout = outFile
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err = cmd.Run()
		outFile.Close()
		if err != nil {
			global.LOG.Errorf("[DB] BackupDatabase FAILED (%.2fs): %v | stderr: %s", time.Since(start).Seconds(), err, stderr.String())
			os.Remove(outputPath)
			return "", fmt.Errorf("backup failed: %s: %v", stderr.String(), err)
		}
	case "sqlite":
		if !syscmd.Which("sqlite3") {
			return "", fmt.Errorf("sqlite3 未安装")
		}
		srcPath, err := s.sqliteFilePath(item, dbName)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			return "", fmt.Errorf("SQLite 数据库文件不存在: %s", srcPath)
		}
		fileName = fmt.Sprintf("%s_%s.sqlite", dbName, timestamp)
		outputPath = filepath.Join(backupDir, fileName)
		backupCmd := fmt.Sprintf(".backup '%s'", strings.ReplaceAll(outputPath, "'", "''"))
		if out, err := s.runSqliteCmd(srcPath, backupCmd); err != nil {
			global.LOG.Errorf("[DB] BackupDatabase(sqlite) FAILED: %v | output: %s", err, string(out))
			os.Remove(outputPath)
			return "", fmt.Errorf("backup failed: %s: %v", string(out), err)
		}
	default:
		return "", fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] BackupDatabase OK: db=%s output=%s (%.2fs)", dbName, outputPath, time.Since(start).Seconds())
	return outputPath, nil
}

// RestoreDatabase 使用 mysql 恢复数据库
func (s *DatabaseService) RestoreDatabase(item *model.DatabaseInstance, dbName, inputPath string) error {
	logItem("RestoreDatabase start", item)
	global.LOG.Infof("[DB] RestoreDatabase db=%s input=%s", dbName, inputPath)
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", inputPath)
	}
	start := time.Now()
	switch item.Type {
	case "mysql":
		if !syscmd.Which("mysql") {
			return fmt.Errorf("mysql client not found")
		}
		args := []string{
			fmt.Sprintf("-h%s", item.Host),
			fmt.Sprintf("-P%d", item.Port),
			fmt.Sprintf("-u%s", item.Username),
			dbName,
		}
		global.LOG.Debugf("[DB] mysql restore exec: host=%s port=%d user=%s db=%s input=%s",
			item.Host, item.Port, item.Username, dbName, inputPath)
		cmd := exec.Command("mysql", args...)
		if item.Password != "" {
			cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", item.Password))
		}
		inFile, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("open input file failed: %v", err)
		}
		cmd.Stdin = inFile
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err = cmd.Run()
		inFile.Close()
		if err != nil {
			global.LOG.Errorf("[DB] RestoreDatabase FAILED (%.2fs): %v | stderr: %s", time.Since(start).Seconds(), err, stderr.String())
			return fmt.Errorf("restore failed: %s: %v", stderr.String(), err)
		}
	case "sqlite":
		if !syscmd.Which("sqlite3") {
			return fmt.Errorf("sqlite3 未安装")
		}
		targetPath, err := s.sqliteFilePath(item, dbName)
		if err != nil {
			return err
		}
		restoreCmd := fmt.Sprintf(".restore '%s'", strings.ReplaceAll(inputPath, "'", "''"))
		if out, err := s.runSqliteCmd(targetPath, restoreCmd); err != nil {
			return fmt.Errorf("restore failed: %s: %v", string(out), err)
		}
	default:
		return fmt.Errorf("unsupported database type: %s", item.Type)
	}
	global.LOG.Infof("[DB] RestoreDatabase OK: db=%s input=%s (%.2fs)", dbName, inputPath, time.Since(start).Seconds())
	return nil
}
