<template>
  <div>
    <div class="page-title">🌐 网站管理</div>

    <el-alert v-if="!serviceStatus.installed" type="warning" show-icon style="margin-bottom: 16px">
      <template #title>
        Nginx 未检测到，请先安装 Nginx 服务
      </template>
      <template #default>
        <div style="margin-top: 8px">
          <el-button type="primary" :loading="installing" @click="installService">安装 Nginx</el-button>
        </div>
      </template>
    </el-alert>

    <template v-else>
      <el-card style="margin-bottom: 16px">
        <div style="display: flex; justify-content: space-between; align-items: center">
          <div style="display: flex; align-items: center; gap: 12px">
            <el-tag :type="serviceStatus.running ? 'success' : 'danger'" size="large">
              {{ serviceStatus.running ? '运行中' : '已停止' }}
            </el-tag>
            <span v-if="serviceStatus.version">Nginx v{{ serviceStatus.version }}</span>
          </div>
          <div style="display: flex; gap: 8px">
            <el-button size="small" type="primary" @click="startService" v-if="!serviceStatus.running" :loading="actionLoading">启动</el-button>
            <el-button size="small" type="warning" @click="stopService" v-if="serviceStatus.running" :loading="actionLoading">停止</el-button>
            <el-button size="small" @click="restartService" :loading="actionLoading">重启</el-button>
            <el-button size="small" @click="reloadNginx">重载配置</el-button>
            <el-button size="small" @click="checkStatus">刷新状态</el-button>
          </div>
        </div>
      </el-card>

      <div style="margin-bottom:16px;display:flex;gap:8px;justify-content:space-between;align-items:center">
        <div style="display:flex;gap:8px">
          <el-button type="primary" @click="showAdd">添加网站</el-button>
          <el-button @click="showLnmp = true; loadPhpVersions()">LNMP 管理</el-button>
        </div>
      </div>

      <div class="table-wrap">
        <el-table :data="websites" size="small" v-loading="loading">
          <el-table-column prop="name" label="名称" width="140" />
          <el-table-column prop="domain" label="域名" />
          <el-table-column prop="port" label="端口" width="80" />
          <el-table-column label="类型" width="80">
            <template #default="{ row }">
              <el-tag size="small" :type="row.type === 'php' ? 'warning' : row.type === 'proxy' ? 'primary' : 'info'">{{ row.type === 'php' ? 'PHP' : row.type === 'proxy' ? '代理' : '静态' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="SSL" width="80">
            <template #default="{ row }">
              <el-tag size="small" :type="row.ssl ? 'success' : 'info'">{{ row.ssl ? '启用' : '关闭' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="WS" width="70">
            <template #default="{ row }">
              <el-tag v-if="row.proxy_ws" size="small" type="warning">WS</el-tag>
              <span v-else style="color: #c0c4cc">-</span>
            </template>
          </el-table-column>
          <el-table-column label="密码" width="70">
            <template #default="{ row }">
              <el-tag v-if="row.auth_enabled" size="small" type="danger">🔒</el-tag>
              <span v-else style="color: #c0c4cc">-</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag size="small" :type="row.enabled ? 'success' : 'danger'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="根目录" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              <span style="color: #909399; font-family: monospace">{{ row.root || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="280" fixed="right">
            <template #default="{ row }">
              <el-button size="small" @click="editWebsite(row)">编辑</el-button>
              <el-button size="small" @click="toggleWebsite(row)">{{ row.enabled ? '停用' : '启用' }}</el-button>
              <el-button size="small" @click="showLogs(row)">日志</el-button>
              <el-button size="small" type="danger" @click="deleteWebsite(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <el-dialog v-model="showForm" :title="editing ? '编辑网站' : '添加网站'" width="780px">
        <el-form :model="form" label-width="100px">
          <el-form-item label="名称" required>
            <el-input v-model="form.name" placeholder="如：主站" />
          </el-form-item>
          <el-form-item label="域名" required>
            <el-input v-model="form.domain" placeholder="如：example.com" @blur="validateDomain" />
            <div v-if="domainError" style="color: #f56c6c; font-size: 12px; margin-top: 4px">{{ domainError }}</div>
          </el-form-item>
          <el-form-item label="端口">
            <el-input-number v-model="form.port" :min="1" :max="65535" :default-value="80" />
          </el-form-item>
          <el-form-item label="类型">
            <el-radio-group v-model="form.type">
              <el-radio-button label="static">静态网站</el-radio-button>
              <el-radio-button label="proxy">反向代理</el-radio-button>
              <el-radio-button label="php">PHP 网站</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="网站目录" v-if="form.type === 'static' || form.type === 'php'">
            <el-input v-model="form.root" placeholder="留空自动创建 /data/www/domain" />
          </el-form-item>
          <el-form-item label="默认首页" v-if="form.type === 'static' || form.type === 'php'">
            <el-input v-model="form.index_page" placeholder="index.html index.htm index.php" />
            <div style="font-size: 12px; color: #909399; margin-top: 4px">按优先级排列，空格分隔</div>
          </el-form-item>
          <el-form-item label="PHP 版本" v-if="form.type === 'php'">
            <el-select v-model="form.php_version" placeholder="选择 PHP 版本" style="width: 100%">
              <el-option v-for="v in phpVersions" :key="v.version" :label="`PHP ${v.version}${v.installed ? '' : ' (未安装)'}`" :value="v.version" :disabled="!v.installed" />
            </el-select>
            <div style="font-size: 12px; color: #909399; margin-top: 4px">请先在 LNMP 管理中安装 PHP 版本</div>
          </el-form-item>
          <el-form-item label="代理目标" v-if="form.type === 'proxy'">
            <el-input v-model="form.proxy_target" placeholder="如：http://localhost:8080" />
          </el-form-item>
          <el-form-item label="WebSocket" v-if="form.type === 'proxy'">
            <el-switch v-model="form.proxy_ws" />
            <span style="margin-left: 8px; color: #909399; font-size: 12px">启用后支持 WebSocket 连接升级</span>
          </el-form-item>
          <el-form-item label="上传大小限制" v-if="form.type === 'proxy'">
            <el-input v-model="form.client_max_body_size" placeholder="如：10240M（留空为 Nginx 默认限制）" />
          </el-form-item>
          <el-form-item label="SSL">
            <el-switch v-model="form.ssl" />
          </el-form-item>
          <template v-if="form.ssl">
            <el-form-item label="证书路径">
              <el-input v-model="form.ssl_cert" placeholder="/path/to/cert.pem" />
            </el-form-item>
            <el-form-item label="私钥路径">
              <el-input v-model="form.ssl_key" placeholder="/path/to/key.pem" />
            </el-form-item>
            <el-divider content-position="left">或直接粘贴证书内容</el-divider>
            <el-form-item label="证书(PEM)">
              <el-input v-model="form.ssl_cert_pem" type="textarea" :rows="4" placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----" />
            </el-form-item>
            <el-form-item label="私钥(PEM)">
              <el-input v-model="form.ssl_key_pem" type="textarea" :rows="4" placeholder="-----BEGIN PRIVATE KEY-----&#10;...&#10;-----END PRIVATE KEY-----" />
            </el-form-item>
          </template>
          <el-divider content-position="left">301/302 重定向</el-divider>
          <div v-for="(rule, idx) in form.redirects" :key="idx" style="display: flex; gap: 8px; margin-bottom: 8px; align-items: center">
            <el-input v-model="rule.from" placeholder="源域名" style="width: 180px" size="small" />
            <span style="color: #909399">→</span>
            <el-input v-model="rule.to" placeholder="目标 URL" style="width: 200px" size="small" />
            <el-select v-model="rule.code" style="width: 100px" size="small">
              <el-option :value="301" label="301 永久" />
              <el-option :value="302" label="302 临时" />
            </el-select>
            <el-button size="small" type="danger" @click="form.redirects.splice(idx, 1)" circle>✕</el-button>
          </div>
          <el-button size="small" @click="form.redirects.push({ from: '', to: '', code: 301 })" style="margin-bottom: 16px">+ 添加重定向</el-button>
          <el-divider content-position="left">目录密码保护</el-divider>
          <el-form-item label="启用密码">
            <el-switch v-model="form.auth_enabled" />
          </el-form-item>
          <template v-if="form.auth_enabled">
            <el-form-item label="用户名">
              <el-input v-model="form.auth_user" placeholder="访问用户名" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="form.auth_password" type="password" placeholder="访问密码" show-password />
            </el-form-item>
          </template>
          <el-divider content-position="left">自定义错误页面</el-divider>
          <el-form-item label="404 页面">
            <el-input v-model="form.error_page_404" type="textarea" :rows="3" placeholder="404 页面的 HTML 内容" />
          </el-form-item>
          <el-form-item label="502 页面">
            <el-input v-model="form.error_page_502" type="textarea" :rows="3" placeholder="502 页面的 HTML 内容" />
          </el-form-item>
          <el-form-item label="503 页面">
            <el-input v-model="form.error_page_503" type="textarea" :rows="3" placeholder="503 页面的 HTML 内容" />
          </el-form-item>
          <el-divider content-position="left">频率限制</el-divider>
          <el-form-item label="启用限流">
            <el-switch v-model="form.rate_limit_enabled" />
          </el-form-item>
          <template v-if="form.rate_limit_enabled">
            <el-form-item label="速率">
              <el-input v-model="form.rate_limit_rate" placeholder="10r/s" style="width: 200px" />
              <span style="margin-left: 8px; color: #909399; font-size: 12px">如 10r/s、100r/m</span>
            </el-form-item>
            <el-form-item label="突发容量">
              <el-input-number v-model="form.rate_limit_burst" :min="1" :max="1000" />
            </el-form-item>
          </template>
          <el-divider content-position="left">防盗链</el-divider>
          <el-form-item label="启用防盗链">
            <el-switch v-model="form.hotlink_protection" />
          </el-form-item>
          <template v-if="form.hotlink_protection">
            <el-form-item label="允许域名">
              <el-input v-model="form.hotlink_domains" placeholder="如 mydomain.com other.com（逗号分隔）" />
            </el-form-item>
            <el-form-item label="保护扩展名">
              <el-input v-model="form.hotlink_exts" placeholder="jpg,jpeg,png,gif,svg,webp,js,css,woff,woff2" />
            </el-form-item>
          </template>
          <el-divider content-position="left">IP 黑白名单</el-divider>
          <el-form-item label="启用过滤">
            <el-switch v-model="form.ip_filter_enabled" />
          </el-form-item>
          <template v-if="form.ip_filter_enabled">
            <el-form-item label="模式">
              <el-radio-group v-model="form.ip_filter_mode">
                <el-radio label="blacklist">黑名单</el-radio>
                <el-radio label="whitelist">白名单</el-radio>
              </el-radio-group>
            </el-form-item>
            <el-form-item label="IP 列表">
              <el-input v-model="form.ip_filter_list" type="textarea" :rows="3" placeholder="每行一个 IP 或 CIDR&#10;如：&#10;192.168.1.100&#10;10.0.0.0/8" />
            </el-form-item>
          </template>
          <el-form-item label="备注">
            <el-input v-model="form.remark" type="textarea" :rows="2" />
          </el-form-item>
          <el-divider content-position="left">联动数据库（仅新建网站时生效）</el-divider>
          <el-form-item label="同时建库">
            <el-switch v-model="form.db_create" :disabled="editing" />
            <span style="margin-left: 8px; color: #909399; font-size: 12px">勾选后会同步创建 MySQL 数据库和专用用户</span>
          </el-form-item>
          <template v-if="form.db_create && !editing">
            <el-form-item label="数据库实例">
              <el-select v-model="form.db_instance_id" placeholder="选择数据库实例" style="width: 100%">
                <el-option v-for="db in databaseInstances" :key="db.id" :label="`${db.name} (${db.host}:${db.port})`" :value="db.id" />
              </el-select>
              <div style="font-size: 12px; color: #909399; margin-top: 4px">若下拉为空，请先在数据库管理页面添加 MySQL 实例</div>
            </el-form-item>
            <el-form-item label="数据库名">
              <el-input v-model="form.db_name" placeholder="如 myapp_db" @focus="autoFillDB" />
            </el-form-item>
            <el-form-item label="用户名">
              <el-input v-model="form.db_username" placeholder="如 myapp_user" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="form.db_password" placeholder="留空自动生成 16 位随机密码" show-password>
                <template #append>
                  <el-button size="small" @click="genRandomPassword">随机</el-button>
                </template>
              </el-input>
            </el-form-item>
          </template>
        </el-form>
        <template #footer>
          <el-button @click="showForm = false">取消</el-button>
          <el-button type="primary" @click="saveWebsite">保存</el-button>
        </template>
      </el-dialog>

      <el-dialog v-model="showLogDialog" :title="`访问日志 - ${logSite?.domain || ''}`" width="900px" @close="logEntries = []; logTotal = 0">
        <el-tabs v-model="logTab" @tab-change="onLogTabChange">
          <el-tab-pane label="访问日志" name="logs">
            <div style="display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap">
              <el-input v-model="logFilter.date" placeholder="日期 2026-07-06" size="small" style="width: 140px" clearable @change="loadLogs" />
              <el-input v-model="logFilter.ip" placeholder="IP" size="small" style="width: 140px" clearable @change="loadLogs" />
              <el-select v-model="logFilter.status_code" placeholder="状态码" size="small" style="width: 100px" clearable @change="loadLogs">
                <el-option label="200" value="200" />
                <el-option label="301" value="301" />
                <el-option label="302" value="302" />
                <el-option label="403" value="403" />
                <el-option label="404" value="404" />
                <el-option label="500" value="500" />
                <el-option label="502" value="502" />
              </el-select>
              <el-input v-model="logFilter.url" placeholder="URL 关键词" size="small" style="width: 160px" clearable @change="loadLogs" />
              <el-button size="small" @click="logFilter = { date: '', ip: '', status_code: '', url: '', page: 1, page_size: 50 }; loadLogs()">重置</el-button>
            </div>
            <el-table :data="logEntries" size="small" v-loading="logLoading" max-height="400">
              <el-table-column prop="time" label="时间" width="180" />
              <el-table-column prop="ip" label="IP" width="130" />
              <el-table-column label="请求" min-width="200">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.method === 'GET' ? 'success' : row.method === 'POST' ? 'warning' : 'info'" style="margin-right: 6px">{{ row.method }}</el-tag>
                  <span style="font-family: monospace; font-size: 12px">{{ row.url }}</span>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="80">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.status_code < 300 ? 'success' : row.status_code < 400 ? 'warning' : 'danger'">{{ row.status_code }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="大小" width="80">
                <template #default="{ row }">{{ formatLogSize(row.size) }}</template>
              </el-table-column>
            </el-table>
            <div style="margin-top: 12px; display: flex; justify-content: center">
              <el-pagination v-model:current-page="logFilter.page" :page-size="logFilter.page_size" :total="logTotal" layout="prev, pager, next" @current-change="loadLogs" small />
            </div>
          </el-tab-pane>
          <el-tab-pane label="流量统计" name="stats">
            <div style="margin-bottom: 8px">
              <el-radio-group v-model="statsPeriod" size="small" @change="loadStats">
                <el-radio-button label="24h">24小时</el-radio-button>
                <el-radio-button label="7d">7天</el-radio-button>
                <el-radio-button label="30d">30天</el-radio-button>
              </el-radio-group>
            </div>
            <v-chart v-if="statsOption" :option="statsOption" style="height: 300px" autoresize />
            <div v-else style="text-align: center; padding: 40px; color: #909399">暂无统计数据</div>
          </el-tab-pane>
        </el-tabs>
      </el-dialog>

      <!-- LNMP 管理 -->
      <el-dialog v-model="showLnmp" title="LNMP 套件管理" width="800px">
        <el-tabs v-model="lnmpTab">
          <el-tab-pane label="PHP 版本" name="versions">
            <el-table :data="phpVersions" v-loading="phpLoading" size="small">
              <el-table-column prop="version" label="版本" width="100">
                <template #default="{ row }">PHP {{ row.version }}</template>
              </el-table-column>
              <el-table-column label="状态" width="160">
                <template #default="{ row }">
                  <el-tag v-if="!row.installed" size="small" type="info">未安装</el-tag>
                  <el-tag v-else-if="row.running" size="small" type="success">运行中</el-tag>
                  <el-tag v-else size="small" type="danger">已停止</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="Socket" min-width="200">
                <template #default="{ row }">
                  <span v-if="row.fpm_socket" style="font-family: monospace; font-size: 12px">{{ row.fpm_socket }}</span>
                  <span v-else style="color: #c0c4cc">-</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="320">
                <template #default="{ row }">
                  <template v-if="!row.installed">
                    <el-button size="small" type="primary" @click="installPhpVersion(row.version)">安装</el-button>
                  </template>
                  <template v-else>
                    <el-button size="small" type="success" v-if="!row.running" @click="startPhpFpm(row.version)">启动</el-button>
                    <el-button size="small" type="warning" v-if="row.running" @click="stopPhpFpm(row.version)">停止</el-button>
                    <el-button size="small" @click="restartPhpFpm(row.version)">重启</el-button>
                    <el-button size="small" @click="showPhpExt(row.version)">扩展</el-button>
                    <el-button size="small" @click="showPhpConfig(row.version)">配置</el-button>
                    <el-button size="small" type="danger" @click="removePhpVersion(row.version)">卸载</el-button>
                  </template>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <el-tab-pane v-if="selectedPhpVersion" label="扩展管理" name="extensions">
            <div style="margin-bottom: 12px; display: flex; gap: 8px; align-items: center">
              <span style="font-weight: 600">PHP {{ selectedPhpVersion }}</span>
              <el-input v-model="newExtName" placeholder="扩展名 (如 redis, imagick)" size="small" style="width: 200px" />
              <el-button size="small" type="primary" @click="installExt">安装扩展</el-button>
              <el-button size="small" @click="loadPhpExts">刷新</el-button>
            </div>
            <el-table :data="phpExts" v-loading="extLoading" size="small">
              <el-table-column prop="name" label="扩展名" />
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.installed ? 'success' : 'info'">{{ row.installed ? '已安装' : '未安装' }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="100">
                <template #default="{ row }">
                  <el-button v-if="row.installed" size="small" type="danger" @click="removeExt(row.name)">卸载</el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <el-tab-pane v-if="selectedPhpVersion" label="PHP 配置" name="config">
            <div style="margin-bottom: 12px; display: flex; gap: 8px; align-items: center">
              <span style="font-weight: 600">PHP {{ selectedPhpVersion }} php.ini</span>
              <el-button size="small" type="primary" @click="savePhpConfig">保存配置</el-button>
              <el-button size="small" @click="loadPhpConfig">刷新</el-button>
            </div>
            <el-table :data="phpConfig" v-loading="configLoading" size="small">
              <el-table-column prop="key" label="配置项" width="220" />
              <el-table-column label="值">
                <template #default="{ row }">
                  <el-input v-model="row.value" size="small" />
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </el-dialog>

      <!-- PHP 安装进度对话框 -->
      <el-dialog
        v-model="phpInstallDialog"
        :title="`正在安装 PHP ${phpInstallVersion}`"
        width="560px"
        :close-on-click-modal="false"
        :close-on-press-escape="false"
        :show-close="phpInstallDone"
      >
        <div class="php-install-container">
          <div class="php-install-spinner">
            <el-icon v-if="!phpInstallDone" class="is-loading" :size="42" color="#409eff">
              <Loading />
            </el-icon>
            <el-icon v-else :size="42" :color="phpInstallError ? '#f56c6c' : '#67c23a'">
              <CircleCheck v-if="!phpInstallError" />
              <CircleClose v-else />
            </el-icon>
          </div>
          <div class="php-install-status">
            {{ phpInstallError ? '安装失败' : (phpInstallDone ? '安装完成' : '正在安装，请耐心等待...') }}
          </div>
          <div class="php-install-tip">
            安装过程可能需要 1-5 分钟，期间会自动添加 PHP 源、刷新索引、下载并配置包
          </div>
          <div class="php-steps">
            <div
              v-for="(step, idx) in phpInstallSteps"
              :key="idx"
              class="php-step"
              :class="{
                active: idx === phpInstallCurrentStep && !phpInstallDone,
                done: idx < phpInstallCurrentStep || (phpInstallDone && !phpInstallError),
                error: phpInstallError && idx === phpInstallCurrentStep
              }"
            >
              <span class="php-step-icon">
                <el-icon v-if="idx < phpInstallCurrentStep || (phpInstallDone && !phpInstallError)"><Check /></el-icon>
                <el-icon v-else-if="idx === phpInstallCurrentStep && !phpInstallDone" class="is-loading"><Loading /></el-icon>
                <el-icon v-else-if="phpInstallError && idx === phpInstallCurrentStep"><Close /></el-icon>
                <span v-else>{{ idx + 1 }}</span>
              </span>
              <span class="php-step-text">{{ step }}</span>
            </div>
          </div>
          <el-progress
            v-if="!phpInstallDone"
            :percentage="phpInstallProgress"
            :stroke-width="8"
            :show-text="false"
            status="success"
            style="margin-top: 16px"
          />
          <div v-if="phpInstallError" class="php-install-error">
            {{ phpInstallError }}
          </div>
        </div>
        <template #footer>
          <el-button v-if="phpInstallDone" type="primary" @click="phpInstallDialog = false">关闭</el-button>
          <el-button v-else disabled>安装中...</el-button>
        </template>
      </el-dialog>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Loading, CircleCheck, CircleClose, Check, Close } from '@element-plus/icons-vue'
import { websiteApi, systemApi, phpApi, databaseApi } from '../api'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

const websites = ref<any[]>([])
const loading = ref(false)
const showForm = ref(false)
const editing = ref(false)
const installing = ref(false)
const actionLoading = ref(false)
const serviceStatus = ref<any>({ installed: false, running: false, version: '' })

const form = ref<any>({ name: '', domain: '', port: 80, root: '', type: 'static', php_version: '', proxy_target: '', proxy_ws: false, client_max_body_size: '', ssl: false, ssl_cert: '', ssl_key: '', ssl_cert_pem: '', ssl_key_pem: '', index_page: '', redirects: [], auth_enabled: false, auth_user: '', auth_password: '',
  error_page_404: '', error_page_502: '', error_page_503: '',
  rate_limit_enabled: false, rate_limit_rate: '', rate_limit_burst: 10,
  hotlink_protection: false, hotlink_domains: '', hotlink_exts: 'jpg,jpeg,png,gif,svg,webp,js,css,woff,woff2',
  ip_filter_enabled: false, ip_filter_mode: 'blacklist', ip_filter_list: '',
  remark: '',
  // 联动建库
  db_create: false, db_instance_id: 0, db_name: '', db_username: '', db_password: '' })
const domainError = ref('')
const databaseInstances = ref<any[]>([])
const deleteCascadeDB = ref(true)

const domainRegex = /^([\w\-\*]{1,100}\.){1,8}([\w\-]{1,24}|[\w\-]{1,24}\.[\w\-]{1,24})$/

function validateDomain() {
  const domain = form.value.domain?.trim()
  if (!domain) {
    domainError.value = ''
    return true
  }
  if (!domainRegex.test(domain)) {
    domainError.value = '域名格式不正确，请输入如 example.com 或 www.example.com'
    return false
  }
  domainError.value = ''
  return true
}

async function checkStatus() {
  try {
    const res: any = await websiteApi.getNginxStatus()
    serviceStatus.value = res.data || { installed: false, running: false }
  } catch (e: any) {
    // 如果新接口失败，回退到旧接口
    try {
      const res: any = await systemApi.checkServices()
      serviceStatus.value = res.data?.nginx || { installed: false, running: false }
    } catch (e2: any) {
      ElMessage.error(e2?.message || '检查服务状态失败')
    }
  }
}

async function installService() {
  installing.value = true
  try {
    await systemApi.installService('nginx')
    ElMessage.success('Nginx 安装成功')
    await checkStatus()
    if (serviceStatus.value.installed) {
      loadWebsites()
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '安装失败')
  } finally {
    installing.value = false
  }
}

async function startService() {
  actionLoading.value = true
  try {
    await websiteApi.startNginx()
    ElMessage.success('Nginx 启动成功')
    await checkStatus()
  } catch (e: any) {
    ElMessage.error(e?.message || '启动失败')
  } finally {
    actionLoading.value = false
  }
}

async function stopService() {
  actionLoading.value = true
  try {
    await websiteApi.stopNginx()
    ElMessage.success('Nginx 停止成功')
    await checkStatus()
  } catch (e: any) {
    ElMessage.error(e?.message || '停止失败')
  } finally {
    actionLoading.value = false
  }
}

async function restartService() {
  actionLoading.value = true
  try {
    await websiteApi.restartNginx()
    ElMessage.success('Nginx 重启成功')
    await checkStatus()
  } catch (e: any) {
    ElMessage.error(e?.message || '重启失败')
  } finally {
    actionLoading.value = false
  }
}

async function loadWebsites() {
  loading.value = true
  try {
    const res: any = await websiteApi.list()
    websites.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

function showAdd() {
  editing.value = false
  form.value = { name: '', domain: '', port: 80, root: '', type: 'static', php_version: '', proxy_target: '', proxy_ws: false, client_max_body_size: '', ssl: false, ssl_cert: '', ssl_key: '', ssl_cert_pem: '', ssl_key_pem: '', index_page: '', redirects: [],
    error_page_404: '', error_page_502: '', error_page_503: '',
    rate_limit_enabled: false, rate_limit_rate: '', rate_limit_burst: 10,
    hotlink_protection: false, hotlink_domains: '', hotlink_exts: 'jpg,jpeg,png,gif,svg,webp,js,css,woff,woff2',
    ip_filter_enabled: false, ip_filter_mode: 'blacklist', ip_filter_list: '',
    auth_enabled: false, auth_user: '', auth_password: '', remark: '',
    db_create: false, db_instance_id: 0, db_name: '', db_username: '', db_password: '' }
  domainError.value = ''
  loadDatabaseInstances()
  showForm.value = true
}

// 加载数据库实例列表供联动建库选择
async function loadDatabaseInstances() {
  try {
    const res: any = await databaseApi.list()
    databaseInstances.value = res.data || []
  } catch (e: any) {
    databaseInstances.value = []
  }
}

// 随机生成 16 位密码
function genRandomPassword() {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
  let pwd = ''
  for (let i = 0; i < 16; i++) pwd += chars[Math.floor(Math.random() * chars.length)]
  form.value.db_password = pwd
}

// 根据域名自动生成默认库名和用户名
function autoFillDB() {
  if (form.value.domain && form.value.db_name === '') {
    const base = form.value.domain.replace(/\./g, '_').replace(/[^a-zA-Z0-9_]/g, '').toLowerCase().slice(0, 32)
    form.value.db_name = base
    form.value.db_username = base.slice(0, 16)
  }
}

function editWebsite(row: any) {
  editing.value = true
  form.value = { ...row }
  // 解析 redirects JSON 字符串
  if (typeof form.value.redirects === 'string') {
    try {
      form.value.redirects = JSON.parse(form.value.redirects)
    } catch {
      form.value.redirects = []
    }
  }
  if (!Array.isArray(form.value.redirects)) {
    form.value.redirects = []
  }
  showForm.value = true
}

async function saveWebsite() {
  if (!validateDomain()) {
    ElMessage.warning('请检查域名格式')
    return
  }
  // 序列化 redirects 为 JSON 字符串
  const payload = { ...form.value }
  if (Array.isArray(payload.redirects)) {
    payload.redirects = JSON.stringify(payload.redirects.filter((r: any) => r.from && r.to))
  }
  try {
    if (editing.value) {
      await websiteApi.update(payload.id, payload)
      ElMessage.success('更新成功')
    } else {
      await websiteApi.create(payload)
      ElMessage.success('添加成功')
    }
    showForm.value = false
    loadWebsites()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

async function toggleWebsite(row: any) {
  try {
    if (row.id) {
      await websiteApi.toggle(row.id, !row.enabled)
    } else {
      // 外部站点：通过 domain+port 切换
      await websiteApi.toggleExternal(row.domain, row.port, !row.enabled)
    }
    ElMessage.success('状态已更新')
    loadWebsites()
  } catch (e: any) {
    ElMessage.error(e?.message || '操作失败')
  }
}

async function deleteWebsite(row: any) {
  try {
    // 询问是否级联删除关联数据库
    deleteCascadeDB.value = true
    await ElMessageBox.confirm(
      `<div><p>确定删除 ${row.name} (${row.domain}) 吗？</p>
       <div style="margin-top:12px;display:flex;align-items:center">
         <input type="checkbox" id="cascade_db" checked style="margin-right:6px" />
         <label for="cascade_db">同时删除关联的 MySQL 数据库和用户（删除前会自动备份）</label>
       </div></div>`,
      '确认删除',
      {
        dangerouslyUseHTMLString: true,
        confirmButtonClass: 'el-button--danger',
        showCancelButton: true,
        beforeClose: (action, instance, done) => {
          if (action === 'confirm') {
            const cb = document.getElementById('cascade_db') as HTMLInputElement
            deleteCascadeDB.value = cb ? cb.checked : true
          }
          done()
        }
      }
    )
    if (row.id) {
      await websiteApi.delete(row.id, deleteCascadeDB.value)
    } else {
      // 外部站点：通过 domain+port 删除
      await websiteApi.deleteExternal(row.domain, row.port)
    }
    ElMessage.success('删除成功')
    loadWebsites()
  } catch (e) {}
}

async function reloadNginx() {
  try {
    await websiteApi.reloadNginx()
    ElMessage.success('Nginx 重载成功')
    await checkStatus()
  } catch (e: any) {
    ElMessage.error(e?.message || '重载失败')
  }
}

// 访问日志
const showLogDialog = ref(false)
const logSite = ref<any>(null)
const logEntries = ref<any[]>([])
const logTotal = ref(0)
const logLoading = ref(false)
const logFilter = ref({ date: '', ip: '', status_code: '', url: '', page: 1, page_size: 50 })

function showLogs(row: any) {
  logSite.value = row
  logFilter.value = { date: '', ip: '', status_code: '', url: '', page: 1, page_size: 50 }
  showLogDialog.value = true
  loadLogs()
}

async function loadLogs() {
  if (!logSite.value?.id) return
  logLoading.value = true
  try {
    const res: any = await websiteApi.getAccessLogs(logSite.value.id, logFilter.value)
    logEntries.value = res.data?.entries || []
    logTotal.value = res.data?.total || 0
  } catch (e: any) {
    ElMessage.error(e?.message || '加载日志失败')
  } finally {
    logLoading.value = false
  }
}

function formatLogSize(bytes: number) {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// 流量统计
const logTab = ref('logs')
const statsPeriod = ref('24h')
const statsOption = ref<any>(null)

function onLogTabChange(tab: string) {
  if (tab === 'stats') loadStats()
}

async function loadStats() {
  if (!logSite.value?.id) return
  try {
    const res: any = await websiteApi.getTrafficStats(logSite.value.id, statsPeriod.value)
    const data = res.data || []
    if (data.length === 0) {
      statsOption.value = null
      return
    }
    const times = data.map((d: any) => d.time)
    const requests = data.map((d: any) => d.requests)
    const bandwidth = data.map((d: any) => d.bandwidth)
    statsOption.value = {
      tooltip: { trigger: 'axis' },
      legend: { data: ['请求数', '流量'], textStyle: { color: '#888' } },
      grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
      xAxis: { type: 'category', data: times, axisLabel: { color: '#888', rotate: 30 } },
      yAxis: [
        { type: 'value', name: '请求数', axisLabel: { color: '#888' }, splitLine: { lineStyle: { color: '#2a2a2a' } } },
        { type: 'value', name: '流量', axisLabel: { color: '#888', formatter: (v: number) => formatLogSize(v) }, splitLine: { show: false } }
      ],
      series: [
        { name: '请求数', type: 'line', smooth: true, data: requests, itemStyle: { color: '#4f8cff' } },
        { name: '流量', type: 'line', smooth: true, yAxisIndex: 1, data: bandwidth, itemStyle: { color: '#00d26a' } }
      ]
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '加载统计失败')
  }
}

// LNMP 管理
const showLnmp = ref(false)
const lnmpTab = ref('versions')
const phpVersions = ref<any[]>([])
const phpLoading = ref(false)
const selectedPhpVersion = ref('')

// PHP 安装进度对话框状态
const phpInstallDialog = ref(false)
const phpInstallVersion = ref('')
const phpInstallDone = ref(false)
const phpInstallError = ref('')
const phpInstallProgress = ref(0)
const phpInstallCurrentStep = ref(0)
const phpInstallSteps = ref([
  '检查系统环境与包管理器',
  '添加 ondrej/php PPA 或 sury 源',
  'apt update 刷新索引',
  '下载并安装 PHP 及扩展包',
  '配置 PHP-FPM 与 php.ini',
  '安装完成'
])
const phpExts = ref<any[]>([])
const extLoading = ref(false)
const newExtName = ref('')
const phpConfig = ref<any[]>([])
const configLoading = ref(false)

async function loadPhpVersions() {
  phpLoading.value = true
  try {
    const res: any = await phpApi.getVersions()
    phpVersions.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载 PHP 版本失败')
  } finally { phpLoading.value = false }
}

async function installPhpVersion(version: string) {
  try {
    await ElMessageBox.confirm(`确定安装 PHP ${version} 吗？这将通过包管理器安装`, '安装 PHP', { type: 'warning' })
  } catch {
    return
  }

  // 重置对话框状态
  phpInstallVersion.value = version
  phpInstallDialog.value = true
  phpInstallDone.value = false
  phpInstallError.value = ''
  phpInstallCurrentStep.value = 0
  phpInstallProgress.value = 0

  // 启动伪进度动画（每个步骤的预估时长总和约 5 分钟内会推完前 5 步）
  // 后端实际只有"完成/失败"两种状态，前端通过分阶段动画让用户感知进度
  const stepTimings = [8000, 15000, 30000, 60000, 90000] // 各步骤大致耗时（毫秒）
  let stepIdx = 0
  let stepElapsed = 0
  const tickMs = 500
  const timer = window.setInterval(() => {
    if (phpInstallDone.value) {
      window.clearInterval(timer)
      return
    }
    stepElapsed += tickMs
    if (stepIdx < stepTimings.length && stepElapsed >= stepTimings[stepIdx]) {
      stepIdx++
      if (stepIdx > phpInstallCurrentStep.value && phpInstallCurrentStep.value < phpInstallSteps.value.length - 1) {
        phpInstallCurrentStep.value = stepIdx
      }
      stepElapsed = 0
    }
    // 进度条按剩余时间推进，最多到 95%，等真正返回后再跳到 100%
    if (phpInstallProgress.value < 95) {
      phpInstallProgress.value += 0.3
    }
  }, tickMs)

  try {
    await phpApi.installVersion(version)
    phpInstallProgress.value = 100
    phpInstallCurrentStep.value = phpInstallSteps.value.length - 1
    phpInstallDone.value = true
    ElMessage.success(`PHP ${version} 安装成功`)
    loadPhpVersions()
  } catch (e: any) {
    phpInstallDone.value = true
    phpInstallError.value = e?.message || '安装失败，请查看后端日志获取详细信息'
    ElMessage.error(phpInstallError.value)
  } finally {
    window.clearInterval(timer)
  }
}

async function removePhpVersion(version: string) {
  try {
    await ElMessageBox.confirm(`确定卸载 PHP ${version} 吗？所有相关扩展将被删除`, '卸载 PHP', { type: 'error' })
    await phpApi.removeVersion(version)
    ElMessage.success(`PHP ${version} 卸载成功`)
    loadPhpVersions()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '卸载失败')
  }
}

async function startPhpFpm(version: string) {
  try {
    await phpApi.startFpm(version)
    ElMessage.success(`PHP-FPM ${version} 启动成功`)
    loadPhpVersions()
  } catch (e: any) { ElMessage.error(e?.message || '启动失败') }
}

async function stopPhpFpm(version: string) {
  try {
    await phpApi.stopFpm(version)
    ElMessage.success(`PHP-FPM ${version} 停止成功`)
    loadPhpVersions()
  } catch (e: any) { ElMessage.error(e?.message || '停止失败') }
}

async function restartPhpFpm(version: string) {
  try {
    await phpApi.restartFpm(version)
    ElMessage.success(`PHP-FPM ${version} 重启成功`)
    loadPhpVersions()
  } catch (e: any) { ElMessage.error(e?.message || '重启失败') }
}

function showPhpExt(version: string) {
  selectedPhpVersion.value = version
  lnmpTab.value = 'extensions'
  loadPhpExts()
}

async function loadPhpExts() {
  if (!selectedPhpVersion.value) return
  extLoading.value = true
  try {
    const res: any = await phpApi.getExtensions(selectedPhpVersion.value)
    phpExts.value = res.data || []
  } catch (e: any) { ElMessage.error(e?.message || '加载扩展失败') }
  finally { extLoading.value = false }
}

async function installExt() {
  if (!newExtName.value.trim()) { ElMessage.warning('请输入扩展名'); return }
  try {
    await phpApi.installExtension(selectedPhpVersion.value, newExtName.value.trim())
    ElMessage.success('扩展安装成功')
    newExtName.value = ''
    loadPhpExts()
  } catch (e: any) { ElMessage.error(e?.message || '安装失败') }
}

async function removeExt(name: string) {
  try {
    await ElMessageBox.confirm(`确定卸载扩展 ${name} 吗？`, '卸载扩展', { type: 'warning' })
    await phpApi.removeExtension(selectedPhpVersion.value, name)
    ElMessage.success('扩展卸载成功')
    loadPhpExts()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '卸载失败')
  }
}

function showPhpConfig(version: string) {
  selectedPhpVersion.value = version
  lnmpTab.value = 'config'
  loadPhpConfig()
}

async function loadPhpConfig() {
  if (!selectedPhpVersion.value) return
  configLoading.value = true
  try {
    const res: any = await phpApi.getConfig(selectedPhpVersion.value)
    phpConfig.value = res.data || []
  } catch (e: any) { ElMessage.error(e?.message || '加载配置失败') }
  finally { configLoading.value = false }
}

async function savePhpConfig() {
  try {
    await phpApi.updateConfig(selectedPhpVersion.value, phpConfig.value)
    ElMessage.success('配置已保存，重启 PHP-FPM 后生效')
  } catch (e: any) { ElMessage.error(e?.message || '保存失败') }
}

onMounted(async () => {
  await checkStatus()
  if (serviceStatus.value.installed) {
    loadWebsites()
  }
})
</script>

<style scoped>
.php-install-container {
  text-align: center;
  padding: 12px 0;
}
.php-install-spinner {
  margin-bottom: 12px;
}
.php-install-status {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
}
.php-install-tip {
  font-size: 12px;
  color: #909399;
  margin-bottom: 18px;
}
.php-steps {
  text-align: left;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 16px;
  background: var(--card, #f5f7fa);
  border-radius: 6px;
}
.php-step {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: #606266;
  transition: all 0.2s;
}
.php-step.active {
  color: #409eff;
  font-weight: 600;
}
.php-step.done {
  color: #67c23a;
}
.php-step.error {
  color: #f56c6c;
}
.php-step-icon {
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: rgba(64, 158, 255, 0.1);
  font-size: 13px;
}
.php-step.done .php-step-icon {
  background: rgba(103, 194, 58, 0.15);
}
.php-step.error .php-step-icon {
  background: rgba(245, 108, 108, 0.15);
}
.php-install-error {
  margin-top: 16px;
  padding: 10px 12px;
  background: rgba(245, 108, 108, 0.08);
  border-left: 3px solid #f56c6c;
  color: #f56c6c;
  font-size: 12px;
  text-align: left;
  white-space: pre-wrap;
  max-height: 200px;
  overflow-y: auto;
}
</style>
