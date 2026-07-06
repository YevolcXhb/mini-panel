package service

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

// logItem 打印实例连接信息（密码脱敏）
func logItem(prefix string, item *model.DatabaseInstance) {
	global.LOG.Infof("[DB] %s id=%d name=%s type=%s host=%s port=%d user=%s pass=%s db=%s",
		prefix, item.ID, item.Name, item.Type, item.Host, item.Port, item.Username, maskPassword(item.Password), item.Database)
}

func (s *DatabaseService) Create(item *model.DatabaseInstance) error {
	if item.Port == 0 {
		item.Port = 3306
	}
	if item.Host == "" {
		item.Host = "127.0.0.1"
	}
	if item.Type == "" {
		item.Type = "mysql"
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		return fmt.Errorf("数据库实例名称不能为空")
	}
	// 检查重名
	existing, _ := s.repo.GetByName(item.Name)
	if existing != nil && existing.ID > 0 {
		global.LOG.Warnf("[DB] Create rejected: name=%s already exists (id=%d)", item.Name, existing.ID)
		return fmt.Errorf("数据库实例名称 '%s' 已存在", item.Name)
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

func (s *DatabaseService) TestConnection(item *model.DatabaseInstance) (string, error) {
	logItem("TestConnection start", item)
	addr := fmt.Sprintf("%s:%d", item.Host, item.Port)
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
	global.LOG.Infof("[DB] TestConnection result: TCP only (type=%s mysql_client=%v)", item.Type, syscmd.Which("mysql"))
	return "TCP connection successful", nil
}

func (s *DatabaseService) CreateDatabase(item *model.DatabaseInstance, dbName string) error {
	logItem("CreateDatabase start", item)
	global.LOG.Infof("[DB] CreateDatabase target: db=%s", dbName)
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql database creation is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found, please install mysql first")
	}
	args := s.getMysqlArgs(item, "")
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName)
	args = append(args, "-e", query)
	if out, err := s.runMysqlCmd(item, args...); err != nil {
		global.LOG.Errorf("[DB] CreateDatabase FAILED: db=%s err=%v", dbName, err)
		return fmt.Errorf("create database failed: %s: %v", string(out), err)
	}
	global.LOG.Infof("[DB] CreateDatabase OK: db=%s", dbName)
	return nil
}

func (s *DatabaseService) CreateUser(item *model.DatabaseInstance, username, password string, privDB string) error {
	logItem("CreateUser start", item)
	global.LOG.Infof("[DB] CreateUser target: user=%s priv=%s", username, privDB)
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql user creation is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found, please install mysql first")
	}
	if privDB == "" || privDB == "*" {
		privDB = "*.*"
	} else {
		privDB = fmt.Sprintf("`%s`.*", privDB)
	}
	queries := []string{
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", username, password),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON %s TO '%s'@'%%'", privDB, username),
		"FLUSH PRIVILEGES",
	}
	fullQuery := strings.Join(queries, "; ")
	args := s.getMysqlArgs(item, "")
	args = append(args, "-e", fullQuery)
	if out, err := s.runMysqlCmd(item, args...); err != nil {
		global.LOG.Errorf("[DB] CreateUser FAILED: user=%s err=%v", username, err)
		return fmt.Errorf("create user failed: %s: %v", string(out), err)
	}
	global.LOG.Infof("[DB] CreateUser OK: user=%s", username)
	return nil
}

// DropDatabase 删除指定数据库
func (s *DatabaseService) DropDatabase(item *model.DatabaseInstance, dbName string) error {
	logItem("DropDatabase start", item)
	global.LOG.Infof("[DB] DropDatabase target: db=%s", dbName)
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql database drop is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found, please install mysql first")
	}
	args := s.getMysqlArgs(item, "")
	query := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)
	args = append(args, "-e", query)
	if out, err := s.runMysqlCmd(item, args...); err != nil {
		global.LOG.Errorf("[DB] DropDatabase FAILED: db=%s err=%v", dbName, err)
		return fmt.Errorf("drop database failed: %s: %v", string(out), err)
	}
	global.LOG.Infof("[DB] DropDatabase OK: db=%s", dbName)
	return nil
}

// DropUser 删除指定 MySQL 用户
func (s *DatabaseService) DropUser(item *model.DatabaseInstance, username string) error {
	logItem("DropUser start", item)
	global.LOG.Infof("[DB] DropUser target: user=%s", username)
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql user drop is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found, please install mysql first")
	}
	args := s.getMysqlArgs(item, "")
	queries := []string{
		fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", username),
		"FLUSH PRIVILEGES",
	}
	fullQuery := strings.Join(queries, "; ")
	args = append(args, "-e", fullQuery)
	if out, err := s.runMysqlCmd(item, args...); err != nil {
		global.LOG.Errorf("[DB] DropUser FAILED: user=%s err=%v", username, err)
		return fmt.Errorf("drop user failed: %s: %v", string(out), err)
	}
	global.LOG.Infof("[DB] DropUser OK: user=%s", username)
	return nil
}

// CreateDatabaseForWebsite 一站式建库+建用户+授权，供 WebsiteService 调用
// 任一步骤失败都会回滚（删除已建的库或用户）
func (s *DatabaseService) CreateDatabaseForWebsite(item *model.DatabaseInstance, dbName, username, password string) error {
	logItem("CreateDatabaseForWebsite start", item)
	global.LOG.Infof("[DB] CreateDatabaseForWebsite db=%s user=%s", dbName, username)

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
	if item.Type != "mysql" {
		return nil, fmt.Errorf("only mysql list databases is supported currently")
	}
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
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var dbs []DBInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || skip[line] {
			continue
		}
		dbs = append(dbs, DBInfo{Name: line})
	}
	global.LOG.Infof("[DB] ListDatabases OK: count=%d (total=%d)", len(dbs), len(lines))
	return dbs, nil
}

type TableInfo struct {
	Name string `json:"name"`
}

func (s *DatabaseService) ListTables(item *model.DatabaseInstance) ([]TableInfo, error) {
	logItem("ListTables start", item)
	if item.Type != "mysql" || item.Database == "" {
		return nil, fmt.Errorf("please select a database first")
	}
	if !syscmd.Which("mysql") {
		return nil, fmt.Errorf("mysql client not found, please install mysql first")
	}
	args := s.getMysqlArgs(item, item.Database)
	args = append(args, "-N", "-B", "-e", "SHOW TABLES")
	out, err := s.runMysqlCmd(item, args...)
	if err != nil {
		global.LOG.Errorf("[DB] ListTables FAILED: db=%s err=%v", item.Database, err)
		return nil, fmt.Errorf("list tables failed: %s: %v", string(out), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var tables []TableInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			tables = append(tables, TableInfo{Name: line})
		}
	}
	global.LOG.Infof("[DB] ListTables OK: db=%s count=%d", item.Database, len(tables))
	return tables, nil
}

func (s *DatabaseService) ChangePassword(item *model.DatabaseInstance, newPassword string) error {
	logItem("ChangePassword start", item)
	global.LOG.Infof("[DB] ChangePassword target: user=%s new_pass=%s", item.Username, maskPassword(newPassword))
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql password change is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found")
	}
	args := s.getMysqlArgs(item, "")
	query := fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'", item.Username, newPassword)
	args = append(args, "-e", query)
	if out, err := s.runMysqlCmd(item, args...); err != nil {
		global.LOG.Errorf("[DB] ChangePassword FAILED: user=%s err=%v", item.Username, err)
		return fmt.Errorf("change password failed: %s: %v", string(out), err)
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
	if item.Type != "mysql" {
		return nil, fmt.Errorf("only mysql is supported currently")
	}
	if !syscmd.Which("mysql") {
		return nil, fmt.Errorf("mysql client not found")
	}
	args := s.getMysqlArgs(item, dbName)
	args = append(args, "-N", "-B", "-e", fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`", tableName))
	out, err := s.runMysqlCmd(item, args...)
	if err != nil {
		return nil, fmt.Errorf("describe table failed: %s: %v", string(out), err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var cols []ColumnInfo
	for _, line := range lines {
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
	// 安全检查：仅允许只读操作
	for _, keyword := range []string{"DROP", "DELETE", "UPDATE", "INSERT", "ALTER", "CREATE", "TRUNCATE", "RENAME", "REPLACE"} {
		if strings.HasPrefix(upper, keyword) || strings.Contains(upper, " "+keyword+" ") {
			return nil, fmt.Errorf("不允许执行 %s 操作，仅支持 SELECT/SHOW/DESCRIBE/EXPLAIN 查询", keyword)
		}
	}
	if item.Type != "mysql" {
		return nil, fmt.Errorf("only mysql is supported currently")
	}
	if !syscmd.Which("mysql") {
		return nil, fmt.Errorf("mysql client not found")
	}
	args := s.getMysqlArgs(item, dbName)
	args = append(args, "-N", "-B", "-e", query)
	out, err := s.runMysqlCmd(item, args...)
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

	// 获取列名
	colArgs := s.getMysqlArgs(item, dbName)
	colArgs = append(colArgs, "-N", "-B", "-e", query+" LIMIT 0")
	// 尝试用 --column-names 获取列名
	colArgs2 := s.getMysqlArgs(item, dbName)
	colArgs2 = append(colArgs2, "-e", query+" LIMIT 0")
	colOut, colErr := s.runMysqlCmd(item, colArgs2...)
	var columns []string
	if colErr == nil {
		colLines := strings.Split(strings.TrimSpace(string(colOut)), "\n")
		if len(colLines) > 0 {
			columns = strings.Split(colLines[0], "\t")
		}
	}
	_ = colArgs // unused but kept for fallback

	global.LOG.Infof("[DB] ExecuteQuery OK: db=%s rows=%d cols=%d", dbName, len(rows), len(columns))
	return &QueryResult{Columns: columns, Rows: rows}, nil
}

// BackupDatabase 使用 mysqldump 备份数据库
func (s *DatabaseService) BackupDatabase(item *model.DatabaseInstance, dbName string) (string, error) {
	logItem("BackupDatabase start", item)
	global.LOG.Infof("[DB] BackupDatabase db=%s", dbName)
	if item.Type != "mysql" {
		return "", fmt.Errorf("only mysql backup is supported currently")
	}
	if !syscmd.Which("mysqldump") {
		return "", fmt.Errorf("mysqldump not found, please install mysql client")
	}
	backupDir := filepath.Join(global.GetDataDir(), "backups", "database")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir failed: %v", err)
	}
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	outputPath := filepath.Join(backupDir, fileName)

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
	start := time.Now()
	cmd := exec.Command("mysqldump", args...)
	if item.Password != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", item.Password))
	}
	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("create output file failed: %v", err)
	}
	defer outFile.Close()
	cmd.Stdout = outFile
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		global.LOG.Errorf("[DB] BackupDatabase FAILED (%.2fs): %v | stderr: %s", time.Since(start).Seconds(), err, stderr.String())
		os.Remove(outputPath)
		return "", fmt.Errorf("backup failed: %s: %v", stderr.String(), err)
	}
	global.LOG.Infof("[DB] BackupDatabase OK: db=%s output=%s (%.2fs)", dbName, outputPath, time.Since(start).Seconds())
	return outputPath, nil
}

// RestoreDatabase 使用 mysql 恢复数据库
func (s *DatabaseService) RestoreDatabase(item *model.DatabaseInstance, dbName, inputPath string) error {
	logItem("RestoreDatabase start", item)
	global.LOG.Infof("[DB] RestoreDatabase db=%s input=%s", dbName, inputPath)
	if item.Type != "mysql" {
		return fmt.Errorf("only mysql restore is supported currently")
	}
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found")
	}
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", inputPath)
	}
	args := []string{
		fmt.Sprintf("-h%s", item.Host),
		fmt.Sprintf("-P%d", item.Port),
		fmt.Sprintf("-u%s", item.Username),
		dbName,
	}
	global.LOG.Debugf("[DB] mysql restore exec: host=%s port=%d user=%s db=%s input=%s",
		item.Host, item.Port, item.Username, dbName, inputPath)
	start := time.Now()
	cmd := exec.Command("mysql", args...)
	if item.Password != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", item.Password))
	}
	inFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input file failed: %v", err)
	}
	defer inFile.Close()
	cmd.Stdin = inFile
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		global.LOG.Errorf("[DB] RestoreDatabase FAILED (%.2fs): %v | stderr: %s", time.Since(start).Seconds(), err, stderr.String())
		return fmt.Errorf("restore failed: %s: %v", stderr.String(), err)
	}
	global.LOG.Infof("[DB] RestoreDatabase OK: db=%s input=%s (%.2fs)", dbName, inputPath, time.Since(start).Seconds())
	return nil
}
