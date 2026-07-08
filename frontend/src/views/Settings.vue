<template>
  <div class="settings-page">
    <div class="settings-header">
      <h2 class="page-title">🔧 系统设置</h2>
      <p class="page-subtitle">管理面板配置、安全选项和系统信息</p>
    </div>

    <el-tabs v-model="activeTab" class="settings-tabs" tab-position="left">
      <!-- 常规设置 -->
      <el-tab-pane label="🎨 常规" name="general">
        <div class="tab-content">
          <div class="section-header">
            <h3>外观设置</h3>
            <p>自定义面板的视觉风格</p>
          </div>
          <div class="form-card">
            <el-form label-width="120px" label-position="right">
              <el-form-item label="主题模式">
                <el-radio-group v-model="themeStore.mode" @change="themeStore.setTheme(themeStore.mode)">
                  <el-radio-button label="light">☀️ 浅色</el-radio-button>
                  <el-radio-button label="dark">🌙 深色</el-radio-button>
                  <el-radio-button label="auto">💻 跟随系统</el-radio-button>
                </el-radio-group>
              </el-form-item>
              <el-form-item label="语言">
                <el-select v-model="settings.language" placeholder="选择语言" style="width: 200px">
                  <el-option label="简体中文" value="zh" />
                  <el-option label="English" value="en" />
                </el-select>
              </el-form-item>
              <el-form-item label="时区">
                <el-input v-model="settings.timezone" placeholder="Asia/Shanghai" style="width: 240px" />
              </el-form-item>
            </el-form>
          </div>

          <div class="section-header" style="margin-top: 24px">
            <h3>容器与文件</h3>
            <p>配置容器运行模式和文件管理范围</p>
          </div>
          <div class="form-card">
            <el-form label-width="120px" label-position="right">
              <el-form-item label="容器模式">
                <el-tag type="info" effect="plain">DockRoot</el-tag>
                <span class="form-hint">轻量级 chroot 容器运行时</span>
              </el-form-item>
              <el-form-item label="文件管理根目录">
                <el-input v-model="settings.file_manager_root" placeholder="/" style="width: 320px" />
                <div class="form-tip">限制文件管理器可访问的根目录路径</div>
              </el-form-item>
              <el-form-item label="负载计算">
                <el-radio-group v-model="settings.load_host_mode">
                  <el-radio label="chroot">仅 Chroot</el-radio>
                  <el-radio label="host">包含宿主机</el-radio>
                </el-radio-group>
                <div class="form-tip">
                  仅 Chroot：只计算 chroot 容器内的进程负载<br/>
                  包含宿主机：同时计算宿主机和 chroot 的进程负载
                </div>
              </el-form-item>
            </el-form>
          </div>

          <div class="action-bar">
            <el-button type="primary" @click="saveSettings">💾 保存设置</el-button>
            <el-button @click="resetSettings">🔄 恢复默认</el-button>
          </div>
        </div>
      </el-tab-pane>

      <!-- 安全设置 -->
      <el-tab-pane label="🔒 安全" name="security">
        <div class="tab-content">
          <div class="section-header">
            <h3>访问控制</h3>
            <p>保护面板入口安全</p>
          </div>
          <div class="form-card">
            <el-form label-width="120px" label-position="right">
              <el-form-item label="安全入口">
                <el-input v-model="settings.SecurityEntrance" placeholder="留空则禁用安全入口" style="width: 320px" />
                <div class="form-tip">
                  设置后需通过 <code>http://面板地址:端口/[安全入口]</code> 访问
                </div>
              </el-form-item>
            </el-form>
          </div>

          <div class="section-header" style="margin-top: 24px">
            <h3>修改密码</h3>
            <p>定期更换密码以保持账户安全</p>
          </div>
          <div class="form-card">
            <el-form :model="pwdForm" label-width="120px" label-position="right">
              <el-form-item label="当前密码">
                <el-input type="password" v-model="pwdForm.old_password" placeholder="输入当前密码" show-password style="width: 320px" />
              </el-form-item>
              <el-form-item label="新密码">
                <el-input type="password" v-model="pwdForm.new_password" placeholder="至少 6 位" show-password style="width: 320px" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" @click="changePassword">修改密码</el-button>
              </el-form-item>
            </el-form>
          </div>

          <div class="action-bar">
            <el-button type="primary" @click="saveSettings">💾 保存设置</el-button>
          </div>
        </div>
      </el-tab-pane>

      <!-- 关于与更新 -->
      <el-tab-pane label="📦 关于" name="about">
        <div class="tab-content">
          <div class="section-header">
            <h3>版本信息</h3>
            <p>当前运行的 Mini Panel 版本详情</p>
          </div>

          <div class="version-hero">
            <div class="version-hero-left">
              <div class="version-logo">M</div>
              <div>
                <div class="version-name">Mini Panel</div>
                <div class="version-tagline">轻量级 Linux 服务器管理面板</div>
              </div>
            </div>
            <div class="version-hero-right">
              <div class="version-number">v{{ versionInfo.version || '—' }}</div>
              <div class="version-meta">
                <span v-if="versionInfo.build_time && versionInfo.build_time !== 'unknown'">
                  构建: {{ versionInfo.build_time }}
                </span>
                <span v-if="versionInfo.git_commit && versionInfo.git_commit !== 'unknown'">
                  Commit: {{ versionInfo.git_commit.substring(0, 8) }}
                </span>
              </div>
            </div>
          </div>

          <div class="section-header" style="margin-top: 24px">
            <h3>检查更新</h3>
            <p>检查是否有新版本，并一键升级</p>
          </div>

          <div class="update-panel" :class="updatePanelClass">
            <div class="update-status">
              <div class="update-status-icon">
                <el-icon v-if="updateChecking" class="is-loading"><Loading /></el-icon>
                <el-icon v-else-if="updateInfo?.has_update" color="#e6a23c"><WarningFilled /></el-icon>
                <el-icon v-else-if="updateInfo && !updateInfo.has_update" color="#67c23a"><SuccessFilled /></el-icon>
                <el-icon v-else color="#909399"><InfoFilled /></el-icon>
              </div>
              <div class="update-status-text">
                <div class="update-status-title">{{ updateStatusTitle }}</div>
                <div class="update-status-desc">{{ updateStatusDesc }}</div>
              </div>
            </div>

            <div class="update-actions">
              <el-button
                v-if="!updateInfo?.has_update"
                type="primary"
                :loading="updateChecking"
                @click="checkUpdate"
              >
                {{ updateChecking ? '检查中...' : '🔍 检查更新' }}
              </el-button>
              <el-button
                v-else
                type="danger"
                :loading="updateApplying"
                @click="applyUpdate"
              >
                ⬇️ 立即更新到 v{{ updateInfo.latest_version }}
              </el-button>
            </div>

            <!-- 更新进度面板 -->
            <div v-if="updateApplying || updateFinished" class="update-progress">
              <div class="progress-header">
                <span>{{ updateProgressTitle }}</span>
                <el-button text size="small" @click="showUpdateLog = !showUpdateLog">
                  {{ showUpdateLog ? '收起日志' : '展开日志' }}
                </el-button>
              </div>
              <el-progress
                v-if="updateApplying"
                :percentage="updateProgress"
                :status="updateProgressStatus"
                :duration="0.5"
                striped
                striped-flow
              />
              <div v-if="showUpdateLog" class="update-log">
                <pre>{{ updateLogContent }}</pre>
              </div>
            </div>

            <!-- 发布说明 -->
            <div v-if="updateInfo?.release_notes && !updateApplying" class="release-notes">
              <div class="release-notes-title">📋 发布说明</div>
              <pre class="release-notes-body">{{ updateInfo.release_notes }}</pre>
            </div>
          </div>

          <!-- 技术栈信息 -->
          <div class="section-header" style="margin-top: 24px">
            <h3>技术栈</h3>
            <p>Mini Panel 使用的主要技术组件</p>
          </div>
          <div class="tech-grid">
            <div class="tech-card">
              <div class="tech-icon">🟢</div>
              <div class="tech-name">后端</div>
              <div class="tech-value">Go 1.23+</div>
            </div>
            <div class="tech-card">
              <div class="tech-icon">💚</div>
              <div class="tech-name">前端</div>
              <div class="tech-value">Vue 3 + Element Plus</div>
            </div>
            <div class="tech-card">
              <div class="tech-icon">⚡</div>
              <div class="tech-name">构建</div>
              <div class="tech-value">Vite</div>
            </div>
            <div class="tech-card">
              <div class="tech-icon">🗃️</div>
              <div class="tech-name">数据库</div>
              <div class="tech-value">SQLite</div>
            </div>
            <div class="tech-card">
              <div class="tech-icon">🐳</div>
              <div class="tech-name">容器</div>
              <div class="tech-value">DockRoot</div>
            </div>
            <div class="tech-card">
              <div class="tech-icon">🔌</div>
              <div class="tech-name">协议</div>
              <div class="tech-value">SSE / WebSocket</div>
            </div>
          </div>
        </div>
      </el-tab-pane>

      <!-- 危险操作 -->
      <el-tab-pane label="⚠️ 危险" name="danger">
        <div class="tab-content">
          <div class="section-header">
            <h3 style="color: var(--el-color-danger)">危险操作</h3>
            <p>这些操作不可恢复，请谨慎执行</p>
          </div>
          <div class="danger-card">
            <div class="danger-item">
              <div class="danger-info">
                <div class="danger-title">清除所有数据</div>
                <div class="danger-desc">
                  删除所有网站、数据库、文件配置和用户数据，恢复到初始状态。操作完成后需要使用默认账号 <code>admin / 123456</code> 重新登录。
                </div>
              </div>
              <el-button type="danger" plain @click="clearData">🗑 清除所有数据</el-button>
            </div>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { settingApi, authApiExt, versionApi, updateApi } from '../api'
import { useAuthStore, useThemeStore } from '../store'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Loading, WarningFilled, SuccessFilled, InfoFilled } from '@element-plus/icons-vue'

const themeStore = useThemeStore()
const settings = ref<any>({})
const pwdForm = ref({ old_password: '', new_password: '' })
const versionInfo = ref({ version: '-', build_time: '-', git_commit: '-' })
const activeTab = ref('general')

// ===== 更新相关状态 =====
const updateChecking = ref(false)
const updateApplying = ref(false)
const updateFinished = ref(false)
const updateInfo = ref<any>(null)
const updateProgress = ref(0)
const updateLogContent = ref('')
const showUpdateLog = ref(false)
let updateLogTimer: ReturnType<typeof setInterval> | null = null
let updateProgressTimer: ReturnType<typeof setInterval> | null = null

const updatePanelClass = computed(() => {
  if (updateApplying.value) return 'update-applying'
  if (updateInfo.value?.has_update) return 'update-available'
  if (updateInfo.value && !updateInfo.value.has_update) return 'update-latest'
  return ''
})

const updateStatusTitle = computed(() => {
  if (updateChecking.value) return '正在检查更新...'
  if (updateApplying.value) return '正在更新...'
  if (updateFinished.value) return '更新完成'
  if (updateInfo.value?.has_update) return `发现新版本 v${updateInfo.value.latest_version}`
  if (updateInfo.value && !updateInfo.value.has_update) return '已是最新版本'
  return '尚未检查更新'
})

const updateStatusDesc = computed(() => {
  if (updateChecking.value) return '正在从 GitHub 获取版本信息...'
  if (updateApplying.value) return '后台正在下载并编译最新版本，服务将在完成后自动重启'
  if (updateFinished.value) return 'Mini Panel 已更新到最新版本，请刷新页面'
  if (updateInfo.value?.has_update) {
    return `当前 v${updateInfo.value.current_version} → 最新 v${updateInfo.value.latest_version}（来源：${updateInfo.value.source}）`
  }
  if (updateInfo.value && !updateInfo.value.has_update) {
    return `当前版本 v${updateInfo.value.current_version} 已是最新`
  }
  return '点击右侧按钮检查 GitHub Releases 是否有新版本'
})

const updateProgressTitle = computed(() => {
  if (updateApplying.value) return '更新进度（下载 → 编译 → 重启）'
  return '更新任务已完成'
})

const updateProgressStatus = computed(() => {
  if (updateFinished.value) return 'success'
  return undefined
})

// ===== 设置加载 =====
async function loadSettings() {
  try {
    const res: any = await settingApi.get()
    settings.value = res.data || {}
    if (settings.value.theme) {
      themeStore.setTheme(settings.value.theme)
    }
  } catch (e) {}
}

async function loadVersion() {
  try { const res: any = await versionApi.get(); versionInfo.value = res.data || {} } catch (e) {}
}

async function saveSettings() {
  try {
    const toSave: any = {}
    for (const key of ['theme', 'language', 'timezone', 'container_mode', 'file_manager_root', 'SecurityEntrance', 'load_host_mode']) {
      if (settings.value[key] !== undefined) {
        toSave[key] = settings.value[key]
      }
    }
    toSave.theme = themeStore.mode
    await settingApi.update(toSave)
    ElMessage.success('保存成功')
  } catch (e) {}
}

const auth = useAuthStore()
const router = useRouter()

async function changePassword() {
  if (!pwdForm.value.old_password || !pwdForm.value.new_password) {
    ElMessage.warning('请填写完整')
    return
  }
  if (pwdForm.value.new_password.length < 6) {
    ElMessage.warning('新密码至少6位')
    return
  }
  try {
    await authApiExt.changePassword(pwdForm.value)
    ElMessage.success('密码修改成功，请重新登录')
    pwdForm.value = { old_password: '', new_password: '' }
    auth.clearAuth()
    router.push('/login')
  } catch (e: any) {
    ElMessage.error(e?.message || '修改失败')
  }
}

async function resetSettings() {
  try {
    await ElMessageBox.confirm('确定要恢复默认设置吗？', '确认')
    await settingApi.reset()
    ElMessage.success('已恢复默认设置')
    loadSettings()
  } catch (e) {}
}

async function clearData() {
  try {
    await ElMessageBox.confirm('确定要清除所有数据吗？此操作不可恢复！', '警告', { confirmButtonClass: 'el-button--danger' })
    await settingApi.clearData()
    ElMessage.success('数据已清除，请使用 admin/123456 重新登录')
    auth.clearAuth()
    router.push('/login')
  } catch (e) {}
}

// ===== 检查更新 =====
async function checkUpdate() {
  updateChecking.value = true
  try {
    const res: any = await updateApi.check()
    if (res.code === 200) {
      updateInfo.value = res.data
      if (res.data.has_update) {
        ElMessage.success(`发现新版本 v${res.data.latest_version}`)
      } else {
        ElMessage.info('当前已是最新版本')
      }
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '检查更新失败')
  } finally {
    updateChecking.value = false
  }
}

// ===== 执行更新 =====
async function applyUpdate() {
  try {
    await ElMessageBox.confirm(
      `确定要更新到 v${updateInfo.value.latest_version} 吗？更新过程中服务会短暂重启。`,
      '更新确认',
      { confirmButtonText: '立即更新', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }

  updateApplying.value = true
  updateFinished.value = false
  updateProgress.value = 5
  showUpdateLog.value = true
  updateLogContent.value = '开始更新...\n'

  try {
    const res: any = await updateApi.apply()
    if (res.code === 200) {
      ElMessage.success('更新已开始，服务将在编译完成后自动重启')
      startUpdateProgressPolling()
    } else {
      ElMessage.error(res.message || '启动更新失败')
      updateApplying.value = false
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '启动更新失败')
    updateApplying.value = false
  }
}

function startUpdateProgressPolling() {
  let elapsed = 0
  const totalEstimate = 180 // 预估 3 分钟

  updateProgressTimer = setInterval(() => {
    elapsed += 2
    // 渐进式进度（前 90% 按时间，最后等待重启）
    if (elapsed < totalEstimate) {
      updateProgress.value = Math.min(90, Math.floor((elapsed / totalEstimate) * 90))
    }
  }, 2000)

  // 同时轮询日志
  updateLogTimer = setInterval(async () => {
    try {
      const res: any = await updateApi.log(50)
      if (res.code === 200 && res.data?.log) {
        updateLogContent.value = res.data.log
        // 滚动到底部
      }
    } catch (e) {
      // 服务可能正在重启，忽略错误
    }

    // 检查更新状态
    updateApi.status().then((res: any) => {
      if (res.code === 200 && res.data) {
        if (!res.data.running && updateApplying.value) {
          // 更新进程已结束
          updateApplying.value = false
          updateFinished.value = true
          updateProgress.value = 100
          ElMessage.success('更新完成，即将刷新页面...')
          setTimeout(() => window.location.reload(), 2000)
        }
      }
    }).catch(() => {
      // 服务正在重启，忽略
    })
  }, 3000)
}

function stopUpdatePolling() {
  if (updateProgressTimer) {
    clearInterval(updateProgressTimer)
    updateProgressTimer = null
  }
  if (updateLogTimer) {
    clearInterval(updateLogTimer)
    updateLogTimer = null
  }
}

onMounted(() => {
  loadSettings()
  loadVersion()
})

onUnmounted(() => {
  stopUpdatePolling()
})
</script>

<style scoped>
.settings-page {
  max-width: 1100px;
  margin: 0 auto;
}

.settings-header {
  margin-bottom: 24px;
}

.settings-header .page-title {
  margin: 0 0 4px 0;
  font-size: 22px;
}

.page-subtitle {
  margin: 0;
  color: var(--dim);
  font-size: 13px;
}

.settings-tabs {
  background: var(--card);
  border-radius: 10px;
  padding: 16px;
  border: 1px solid var(--border);
  min-height: 600px;
}

.settings-tabs :deep(.el-tabs__header) {
  margin-right: 20px;
}

.settings-tabs :deep(.el-tabs__item) {
  height: 44px;
  line-height: 44px;
  font-size: 14px;
  padding: 0 16px;
}

.tab-content {
  padding: 8px 8px 8px 16px;
}

.section-header {
  margin-bottom: 12px;
}

.section-header h3 {
  margin: 0 0 4px 0;
  font-size: 15px;
  font-weight: 600;
}

.section-header p {
  margin: 0;
  color: var(--dim);
  font-size: 12px;
}

.form-card {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 20px 24px;
}

.form-tip {
  font-size: 12px;
  color: var(--dim);
  margin-top: 4px;
  line-height: 1.5;
}

.form-hint {
  margin-left: 8px;
  font-size: 12px;
  color: var(--dim);
}

.form-tip code,
.danger-desc code {
  background: var(--card);
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 11px;
  border: 1px solid var(--border);
}

.action-bar {
  margin-top: 20px;
  padding: 16px 0;
  display: flex;
  gap: 12px;
}

/* 版本 hero 卡片 */
.version-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: linear-gradient(135deg, var(--card) 0%, var(--bg) 100%);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 24px 28px;
}

.version-hero-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.version-logo {
  width: 56px;
  height: 56px;
  border-radius: 14px;
  background: linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 32px;
  font-weight: 700;
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.3);
}

.version-name {
  font-size: 18px;
  font-weight: 600;
  color: var(--txt);
}

.version-tagline {
  font-size: 12px;
  color: var(--dim);
  margin-top: 2px;
}

.version-hero-right {
  text-align: right;
}

.version-number {
  font-size: 24px;
  font-weight: 700;
  color: var(--el-color-primary);
  font-family: 'SF Mono', Consolas, monospace;
}

.version-meta {
  margin-top: 4px;
  font-size: 11px;
  color: var(--dim);
  display: flex;
  gap: 12px;
  justify-content: flex-end;
}

/* 更新面板 */
.update-panel {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 20px;
  background: var(--bg);
  transition: all 0.3s;
}

.update-panel.update-available {
  border-color: #e6a23c;
  background: linear-gradient(135deg, rgba(230, 162, 60, 0.05) 0%, var(--bg) 100%);
}

.update-panel.update-applying {
  border-color: var(--el-color-primary);
}

.update-panel.update-latest {
  border-color: #67c23a;
}

.update-status {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.update-status-icon {
  font-size: 28px;
  display: flex;
  align-items: center;
}

.update-status-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--txt);
}

.update-status-desc {
  font-size: 12px;
  color: var(--dim);
  margin-top: 2px;
}

.update-actions {
  display: flex;
  gap: 8px;
}

.update-progress {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px dashed var(--border);
}

.progress-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  font-size: 13px;
  color: var(--txt);
}

.update-log {
  margin-top: 12px;
  background: #1e1e1e;
  color: #d4d4d4;
  border-radius: 6px;
  padding: 12px;
  max-height: 240px;
  overflow-y: auto;
  font-family: 'SF Mono', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
}

.update-log pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}

.release-notes {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px dashed var(--border);
}

.release-notes-title {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
  color: var(--txt);
}

.release-notes-body {
  background: var(--card);
  border-radius: 6px;
  padding: 12px;
  font-size: 12px;
  color: var(--txt);
  max-height: 240px;
  overflow-y: auto;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
  border: 1px solid var(--border);
}

/* 技术栈网格 */
.tech-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
}

.tech-card {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 16px;
  text-align: center;
  transition: transform 0.2s, box-shadow 0.2s;
}

.tech-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.tech-icon {
  font-size: 24px;
  margin-bottom: 8px;
}

.tech-name {
  font-size: 12px;
  color: var(--dim);
  margin-bottom: 4px;
}

.tech-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--txt);
}

/* 危险操作 */
.danger-card {
  background: var(--bg);
  border: 1px solid var(--el-color-danger-light-5);
  border-radius: 8px;
  padding: 8px 20px;
}

.danger-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 0;
  gap: 24px;
}

.danger-item + .danger-item {
  border-top: 1px dashed var(--border);
}

.danger-info {
  flex: 1;
}

.danger-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--el-color-danger);
  margin-bottom: 4px;
}

.danger-desc {
  font-size: 12px;
  color: var(--dim);
  line-height: 1.6;
}
</style>
