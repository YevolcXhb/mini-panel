package model

// PhpVersion PHP 版本信息
type PhpVersion struct {
	Version    string `json:"version"` // 7.4, 8.0, 8.1, 8.2, 8.3
	Installed  bool   `json:"installed"`
	Running    bool   `json:"running"`
	FpmSocket  string `json:"fpm_socket"`
	PhpIniPath string `json:"php_ini_path"`
	BinPath    string `json:"bin_path"`
}

// PhpExtension PHP 扩展信息
type PhpExtension struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
}

// PhpConfigItem PHP 配置项
type PhpConfigItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PhpInstallRequest 安装 PHP 版本请求
type PhpInstallRequest struct {
	Version string `json:"version" binding:"required"`
}

// PhpExtensionRequest 扩展安装/卸载请求
type PhpExtensionRequest struct {
	Name string `json:"name" binding:"required"`
}
