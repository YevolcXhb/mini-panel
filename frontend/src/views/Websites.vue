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
        </div>
      </div>

      <div class="table-wrap">
        <el-table :data="websites" size="small" v-loading="loading">
          <el-table-column prop="name" label="名称" width="140" />
          <el-table-column prop="domain" label="域名" />
          <el-table-column prop="port" label="端口" width="80" />
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
            </el-radio-group>
          </el-form-item>
          <el-form-item label="网站目录" v-if="form.type === 'static'">
            <el-input v-model="form.root" placeholder="留空自动创建 /data/www/domain" />
          </el-form-item>
          <el-form-item label="默认首页" v-if="form.type === 'static'">
            <el-input v-model="form.index_page" placeholder="index.html index.htm index.php" />
            <div style="font-size: 12px; color: #909399; margin-top: 4px">按优先级排列，空格分隔</div>
          </el-form-item>
          <el-form-item label="代理目标" v-if="form.type === 'proxy'">
            <el-input v-model="form.proxy_target" placeholder="如：http://localhost:8080" />
          </el-form-item>
          <el-form-item label="WebSocket" v-if="form.type === 'proxy'">
            <el-switch v-model="form.proxy_ws" />
            <span style="margin-left: 8px; color: #909399; font-size: 12px">启用后支持 WebSocket 连接升级</span>
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
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { websiteApi, systemApi } from '../api'
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

const form = ref<any>({ name: '', domain: '', port: 80, root: '', type: 'static', proxy_target: '', proxy_ws: false, ssl: false, ssl_cert: '', ssl_key: '', ssl_cert_pem: '', ssl_key_pem: '', index_page: '', redirects: [], auth_enabled: false, auth_user: '', auth_password: '',
  error_page_404: '', error_page_502: '', error_page_503: '',
  rate_limit_enabled: false, rate_limit_rate: '', rate_limit_burst: 10,
  hotlink_protection: false, hotlink_domains: '', hotlink_exts: 'jpg,jpeg,png,gif,svg,webp,js,css,woff,woff2',
  ip_filter_enabled: false, ip_filter_mode: 'blacklist', ip_filter_list: '',
  remark: '' })
const domainError = ref('')

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
  form.value = { name: '', domain: '', port: 80, root: '', type: 'static', proxy_target: '', proxy_ws: false, ssl: false, ssl_cert: '', ssl_key: '', ssl_cert_pem: '', ssl_key_pem: '', index_page: '', redirects: [],
    error_page_404: '', error_page_502: '', error_page_503: '',
    rate_limit_enabled: false, rate_limit_rate: '', rate_limit_burst: 10,
    hotlink_protection: false, hotlink_domains: '', hotlink_exts: 'jpg,jpeg,png,gif,svg,webp,js,css,woff,woff2',
    ip_filter_enabled: false, ip_filter_mode: 'blacklist', ip_filter_list: '',
    auth_enabled: false, auth_user: '', auth_password: '', remark: '' }
  domainError.value = ''
  showForm.value = true
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
    await ElMessageBox.confirm(`确定删除 ${row.name} (${row.domain}) 吗？`, '确认删除', { confirmButtonClass: 'el-button--danger' })
    if (row.id) {
      await websiteApi.delete(row.id)
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

onMounted(async () => {
  await checkStatus()
  if (serviceStatus.value.installed) {
    loadWebsites()
  }
})
</script>
