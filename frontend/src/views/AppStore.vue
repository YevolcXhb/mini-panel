<template>
  <div>
    <h2 class="page-title">🛒 应用商店</h2>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="应用列表" name="apps">
        <div class="info-grid" v-if="apps.length">
          <div class="info-card" v-for="app in apps" :key="app.id" style="display:flex;flex-direction:column;gap:10px">
            <div style="display:flex;align-items:center;gap:10px">
              <el-icon size="32" color="#4f8cff"><Box /></el-icon>
              <div style="flex:1">
                <div style="font-weight:600;font-size:15px">{{ app.name }}</div>
                <div style="font-size:12px;color:var(--dim)">{{ app.description }}</div>
              </div>
            </div>
            <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap">
              <span class="tag-on">{{ app.category }}</span>
              <span style="font-size:12px;color:var(--dim)">v{{ app.version }}</span>
            </div>
            <el-button type="primary" size="small" @click="openInstall(app)">安装</el-button>
          </div>
        </div>
        <div v-else class="empty-state">暂无应用</div>
      </el-tab-pane>

      <el-tab-pane label="已安装" name="installed">
        <div class="table-wrap">
          <el-table :data="installed" size="small">
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="image" label="镜像" show-overflow-tooltip />
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
    </el-tabs>

    <el-dialog v-model="showInstall" title="安装应用" width="420px">
      <el-form :model="installForm" label-width="80px">
        <el-form-item label="应用"><span>{{ selectedApp?.name }}</span></el-form-item>
        <el-form-item label="名称"><el-input v-model="installForm.name" placeholder="实例名称" /></el-form-item>
        <el-form-item label="端口"><el-input-number v-model="installForm.port" :min="1" :max="65535" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showInstall = false">取消</el-button>
        <el-button type="primary" :loading="installing" @click="doInstall">安装</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { appApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const activeTab = ref('apps')
const apps = ref<any[]>([])
const installed = ref<any[]>([])
const showInstall = ref(false)
const installing = ref(false)
const selectedApp = ref<any>(null)
const installForm = ref({ name: '', port: 8080 })

async function loadApps() {
  try { const res: any = await appApi.list(); apps.value = res.data || [] } catch (e) {}
}
async function loadInstalled() {
  try { const res: any = await appApi.installed(); installed.value = res.data || [] } catch (e) {}
}

function openInstall(app: any) {
  selectedApp.value = app
  installForm.value.name = app.name.toLowerCase() + '001'
  installForm.value.port = 8080
  showInstall.value = true
}

async function doInstall() {
  if (!selectedApp.value) return
  installing.value = true
  try {
    await appApi.install({ app_id: selectedApp.value.id, name: installForm.value.name, port: installForm.value.port })
    ElMessage.success('安装成功')
    showInstall.value = false
    activeTab.value = 'installed'
    loadInstalled()
  } catch (e) {} finally { installing.value = false }
}

async function uninstall(row: any) {
  try {
    await ElMessageBox.confirm(`确定卸载 ${row.name} 吗？`, '确认', { confirmButtonClass: 'el-button--danger' })
    await appApi.uninstall(row.id)
    ElMessage.success('卸载成功')
    loadInstalled()
  } catch (e) {}
}

onMounted(() => { loadApps(); loadInstalled() })
</script>
