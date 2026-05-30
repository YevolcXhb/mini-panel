<template>
  <div>
    <h2 class="page-title">🛒 应用商店</h2>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="应用列表" name="apps">
        <div style="margin-bottom:16px;display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <el-input v-model="searchQuery" placeholder="搜索应用" style="width:240px" clearable @keyup.enter="doSearch">
            <template #append><el-button @click="doSearch"><el-icon><Search /></el-icon></el-button></template>
          </el-input>
          <el-select v-model="selectedCategory" placeholder="分类" style="width:140px" clearable @change="loadApps">
            <el-option label="全部" value="all" />
            <el-option label="Web" value="web" />
            <el-option label="数据库" value="database" />
            <el-option label="工具" value="tool" />
            <el-option label="其他" value="other" />
          </el-select>
          <el-button type="primary" @click="showSync = true">🔄 同步应用</el-button>
        </div>
        <div class="info-grid" v-if="apps.length">
          <div class="info-card" v-for="app in apps" :key="app.id" style="display:flex;flex-direction:column;gap:10px">
            <div style="display:flex;align-items:center;gap:10px">
              <el-icon size="32" color="#4f8cff"><Box /></el-icon>
              <div style="flex:1">
                <div style="font-weight:600;font-size:15px">{{ app.name }}</div>
                <div style="font-size:12px;color:var(--dim)">{{ app.short_desc || app.description }}</div>
              </div>
            </div>
            <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
              <span class="tag-on">{{ app.category || 'other' }}</span>
              <span class="tag-off">{{ app.type || 'container' }}</span>
            </div>
            <div style="display:flex;gap:8px">
              <el-button type="primary" size="small" @click="openInstall(app)">安装</el-button>
              <el-button size="small" @click="openDetail(app)">详情</el-button>
            </div>
          </div>
        </div>
        <div v-else class="empty-state">暂无应用，请先同步应用源</div>
      </el-tab-pane>

      <el-tab-pane label="已安装" name="installed">
        <div class="table-wrap">
          <el-table :data="installed" size="small">
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="image" label="镜像" show-overflow-tooltip />
            <el-table-column prop="version" label="版本" width="120" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <span :class="row.status === 'running' ? 'tag-on' : row.status === 'failed' ? 'tag-off' : ''">{{ row.status }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="port" label="端口" width="100" />
            <el-table-column label="操作" width="120">
              <template #default="{ row }">
                <el-button size="small" type="danger" @click="uninstall(row)">卸载</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="应用源" name="sources">
        <div style="margin-bottom:16px;display:flex;gap:8px;align-items:center">
          <el-input v-model="newSource.name" placeholder="源名称" style="width:160px" />
          <el-input v-model="newSource.url" placeholder="应用列表URL (JSON)" style="width:400px" />
          <el-button type="primary" @click="addSource">添加源</el-button>
        </div>
        <div class="table-wrap">
          <el-table :data="sources" size="small">
            <el-table-column prop="name" label="名称" width="160" />
            <el-table-column prop="url" label="地址" show-overflow-tooltip />
            <el-table-column prop="enabled" label="状态" width="100">
              <template #default="{ row }">
                <span :class="row.enabled ? 'tag-on' : 'tag-off'">{{ row.enabled ? '启用' : '禁用' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="180">
              <template #default="{ row }">
                <el-button size="small" type="primary" @click="syncSource(row)">同步</el-button>
                <el-button size="small" type="danger" @click="removeSource(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-tab-pane>

      <el-tab-pane label="拉取镜像" name="pull">
        <div class="info-card" style="max-width:600px">
          <el-form label-width="100px">
            <el-form-item label="镜像地址">
              <el-input v-model="pullForm.image" placeholder="如 docker.1ms.run/openlistteam/openlist:latest-lite-aio" />
            </el-form-item>
            <el-form-item label="解析结果" v-if="parsedImage.registry">
              <div style="font-size:13px;color:var(--dim)">
                <div>镜像源: <span style="color:var(--acc)">{{ parsedImage.registry }}</span></div>
                <div>仓库: <span style="color:var(--acc)">{{ parsedImage.repo }}</span></div>
                <div>标签: <span style="color:var(--acc)">{{ parsedImage.tag }}</span></div>
              </div>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="pulling" @click="doPull">拉取镜像</el-button>
            </el-form-item>
          </el-form>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="showInstall" title="安装应用" width="480px">
      <el-form :model="installForm" label-width="100px">
        <el-form-item label="应用"><span>{{ selectedApp?.name }}</span></el-form-item>
        <el-form-item label="版本">
          <el-select v-model="installForm.app_detail_id" placeholder="选择版本" style="width:100%">
            <el-option v-for="d in appDetails" :key="d.id" :label="d.version" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="实例名称"><el-input v-model="installForm.name" placeholder="如 nginx001" /></el-form-item>
        <el-form-item label="端口"><el-input-number v-model="installForm.port" :min="1" :max="65535" style="width:100%" /></el-form-item>
        <el-form-item label="环境变量" v-if="envKeys.length">
          <div v-for="(k, idx) in envKeys" :key="idx" style="display:flex;gap:8px;margin-bottom:8px">
            <el-input :model-value="k" disabled style="width:120px" />
            <el-input v-model="installForm.env[k]" placeholder="值" />
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showInstall = false">取消</el-button>
        <el-button type="primary" :loading="installing" @click="doInstall">安装</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showDetail" title="应用详情" width="520px">
      <div v-if="detailApp">
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px">
          <el-icon size="40" color="#4f8cff"><Box /></el-icon>
          <div>
            <div style="font-weight:700;font-size:18px">{{ detailApp.name }}</div>
            <div style="font-size:13px;color:var(--dim)">{{ detailApp.key }}</div>
          </div>
        </div>
        <p style="font-size:14px;line-height:1.6">{{ detailApp.description }}</p>
        <div style="margin:12px 0;display:flex;gap:8px;flex-wrap:wrap">
          <span class="tag-on">{{ detailApp.category }}</span>
          <span class="tag-off">{{ detailApp.type }}</span>
        </div>
        <div v-if="detailApp.website" style="font-size:13px;margin-top:8px">
          官网: <a :href="detailApp.website" target="_blank" style="color:var(--acc)">{{ detailApp.website }}</a>
        </div>
        <div v-if="detailApp.document" style="font-size:13px;margin-top:4px">
          文档: <a :href="detailApp.document" target="_blank" style="color:var(--acc)">{{ detailApp.document }}</a>
        </div>
        <el-divider />
        <h4 style="margin:8px 0">可用版本</h4>
        <el-table :data="appDetails" size="small">
          <el-table-column prop="version" label="版本" />
          <el-table-column prop="image" label="镜像" show-overflow-tooltip />
        </el-table>
      </div>
    </el-dialog>

    <el-dialog v-model="showSync" title="同步应用" width="400px">
      <p style="margin-bottom:12px">选择要同步的应用源：</p>
      <el-select v-model="syncSourceId" placeholder="选择源" style="width:100%">
        <el-option v-for="s in sources" :key="s.id" :label="s.name" :value="s.id" />
      </el-select>
      <template #footer>
        <el-button @click="showSync = false">取消</el-button>
        <el-button type="primary" :loading="syncing" @click="doSync">同步</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showInstallProgress" title="正在安装" width="420px" :close-on-click-modal="false" :close-on-press-escape="false" :show-close="false">
      <div class="install-progress-box">
        <el-progress :percentage="100" :indeterminate="true" :duration="2" :stroke-width="12" :color="['#409eff', '#67c23a']" style="margin-bottom:20px" />
        <p class="install-progress-text">{{ installProgressText }}</p>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { appApi, containerApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const activeTab = ref('apps')
const apps = ref<any[]>([])
const installed = ref<any[]>([])
const sources = ref<any[]>([])
const searchQuery = ref('')
const selectedCategory = ref('all')
const showInstall = ref(false)
const showInstallProgress = ref(false)
const installProgressText = ref('正在安装...')
const showDetail = ref(false)
const showSync = ref(false)
const installing = ref(false)
const syncing = ref(false)
const selectedApp = ref<any>(null)
const detailApp = ref<any>(null)
const appDetails = ref<any[]>([])
const installForm = ref<any>({ name: '', port: 8080, app_detail_id: undefined, env: {} })
const newSource = ref({ name: '', url: '' })
const pullForm = ref({ image: '' })
const pulling = ref(false)
const syncSourceId = ref<number | undefined>(undefined)

const imageRegex = /^(?:([^\/]+)\/)?([^:\/]+(?:\/[^:\/]+)*)(?::([^:\/]+))?$/

const parsedImage = computed(() => {
  const match = pullForm.value.image.match(imageRegex)
  if (!match) return { registry: '', repo: '', tag: '' }
  return {
    registry: match[1] || 'docker.io',
    repo: match[2] || '',
    tag: match[3] || 'latest'
  }
})

const envKeys = computed(() => Object.keys(installForm.value.env || {}))

async function loadApps() {
  try {
    const res: any = await appApi.list(selectedCategory.value === 'all' ? undefined : selectedCategory.value)
    apps.value = res.data || []
  } catch (e) {}
}

async function doSearch() {
  if (!searchQuery.value) {
    loadApps()
    return
  }
  try {
    const res: any = await appApi.search(searchQuery.value)
    apps.value = res.data || []
  } catch (e) {}
}

async function loadInstalled() {
  try { const res: any = await appApi.installed(); installed.value = res.data || [] } catch (e) {}
}

async function loadSources() {
  try { const res: any = await appApi.sources(); sources.value = res.data || [] } catch (e) {}
}

async function openInstall(app: any) {
  selectedApp.value = app
  installForm.value = { name: app.key + '001', port: 8080, app_detail_id: undefined, env: {} }
  try {
    const res: any = await appApi.detail(app.id)
    appDetails.value = res.data?.details || []
    if (appDetails.value.length > 0) {
      installForm.value.app_detail_id = appDetails.value[0].id
    }
  } catch (e) { appDetails.value = [] }
  showInstall.value = true
}

async function openDetail(app: any) {
  detailApp.value = app
  try {
    const res: any = await appApi.detail(app.id)
    appDetails.value = res.data?.details || []
  } catch (e) { appDetails.value = [] }
  showDetail.value = true
}

async function doInstall() {
  if (!selectedApp.value) return
  showInstall.value = false
  showInstallProgress.value = true
  installProgressText.value = '正在提交安装任务，请耐心等待（下载镜像可能需要几分钟）...'
  try {
    await appApi.install({
      app_id: selectedApp.value.id,
      app_detail_id: installForm.value.app_detail_id,
      name: installForm.value.name,
      port: installForm.value.port,
      env: installForm.value.env
    })
    ElMessage.success('安装成功')
    activeTab.value = 'installed'
    loadInstalled()
  } catch (e: any) {
    ElMessage.error(e?.message || '安装失败')
  } finally { showInstallProgress.value = false }
}

async function uninstall(row: any) {
  try {
    await ElMessageBox.confirm(`确定卸载 ${row.name} 吗？`, '确认', { confirmButtonClass: 'el-button--danger' })
    await appApi.uninstall(row.id)
    ElMessage.success('卸载成功')
    loadInstalled()
  } catch (e) {}
}

async function addSource() {
  if (!newSource.value.name || !newSource.value.url) {
    ElMessage.warning('请填写完整信息')
    return
  }
  try {
    await appApi.addSource(newSource.value)
    ElMessage.success('添加成功')
    newSource.value = { name: '', url: '' }
    loadSources()
  } catch (e) {}
}

async function removeSource(row: any) {
  try {
    await ElMessageBox.confirm(`确定删除源 ${row.name} 吗？`, '确认', { confirmButtonClass: 'el-button--danger' })
    await appApi.removeSource(row.id)
    ElMessage.success('删除成功')
    loadSources()
  } catch (e) {}
}

async function syncSource(row: any) {
  syncing.value = true
  try {
    await appApi.sync(row.id)
    ElMessage.success('同步成功')
    loadApps()
  } catch (e: any) {
    ElMessage.error(e?.message || '同步失败')
  } finally { syncing.value = false }
}

async function doSync() {
  if (!syncSourceId.value) {
    ElMessage.warning('请选择应用源')
    return
  }
  syncing.value = true
  try {
    await appApi.sync(syncSourceId.value)
    ElMessage.success('同步成功')
    showSync.value = false
    loadApps()
  } catch (e: any) {
    ElMessage.error(e?.message || '同步失败')
  } finally { syncing.value = false }
}

async function doPull() {
  if (!pullForm.value.image) {
    ElMessage.warning('请输入镜像地址')
    return
  }
  pulling.value = true
  try {
    await containerApi.pull(pullForm.value.image)
    ElMessage.success('拉取成功')
  } catch (e: any) {
    ElMessage.error(e?.message || '拉取失败')
  } finally { pulling.value = false }
}

onMounted(() => { loadApps(); loadInstalled(); loadSources() })
</script>

<style scoped>
.install-progress-box {
  text-align: center;
  padding: 24px 16px;
}
.install-progress-text {
  font-size: 14px;
  color: var(--el-text-color-regular);
  margin-top: 8px;
}
</style>
