<template>
  <div>
    <el-tabs v-model="activeTab">
      <el-tab-pane label="应用列表" name="apps">
        <el-row :gutter="20">
          <el-col :span="6" v-for="app in apps" :key="app.id" style="margin-bottom: 20px">
            <el-card :body-style="{ padding: '20px' }">
              <div class="app-card">
                <div class="app-icon">
                  <el-icon size="48" color="#409eff"><Box /></el-icon>
                </div>
                <div class="app-info">
                  <h3>{{ app.name }}</h3>
                  <p class="app-desc">{{ app.description }}</p>
                  <p class="app-meta">
                    <el-tag size="small">{{ app.category }}</el-tag>
                    <span class="app-version">{{ app.version }}</span>
                  </p>
                </div>
                <el-button type="primary" size="small" @click="openInstall(app)">安装</el-button>
              </div>
            </el-card>
          </el-col>
        </el-row>
      </el-tab-pane>

      <el-tab-pane label="已安装" name="installed">
        <el-table :data="installed" size="small">
          <el-table-column prop="name" label="名称" />
          <el-table-column prop="image" label="镜像" show-overflow-tooltip />
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'running' ? 'success' : row.status === 'failed' ? 'danger' : 'info'">
                {{ row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="port" label="端口" width="100" />
          <el-table-column label="操作" width="150">
            <template #default="{ row }">
              <el-button size="small" type="danger" @click="uninstall(row)">卸载</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <el-dialog v-model="showInstall" title="安装应用" width="400px">
      <el-form :model="installForm" label-width="80px">
        <el-form-item label="应用">
          <span>{{ selectedApp?.name }}</span>
        </el-form-item>
        <el-form-item label="名称">
          <el-input v-model="installForm.name" placeholder="实例名称" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="installForm.port" :min="1" :max="65535" />
        </el-form-item>
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

const installForm = ref({
  name: '',
  port: 8080
})

async function loadApps() {
  try {
    const res: any = await appApi.list()
    apps.value = res.data || []
  } catch (e) {}
}

async function loadInstalled() {
  try {
    const res: any = await appApi.installed()
    installed.value = res.data || []
  } catch (e) {}
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
    await appApi.install({
      app_id: selectedApp.value.id,
      name: installForm.value.name,
      port: installForm.value.port
    })
    ElMessage.success('安装成功')
    showInstall.value = false
    activeTab.value = 'installed'
    loadInstalled()
  } catch (e) {
  } finally {
    installing.value = false
  }
}

async function uninstall(row: any) {
  try {
    await ElMessageBox.confirm(`确定要卸载 ${row.name} 吗？`, '确认卸载')
    await appApi.uninstall(row.id)
    ElMessage.success('卸载成功')
    loadInstalled()
  } catch (e) {}
}

onMounted(() => {
  loadApps()
  loadInstalled()
})
</script>

<style scoped>
.app-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}
.app-icon {
  margin-bottom: 15px;
}
.app-info {
  flex: 1;
  margin-bottom: 15px;
}
.app-info h3 {
  margin: 0 0 8px;
}
.app-desc {
  color: #888;
  font-size: 13px;
  margin: 0 0 10px;
  min-height: 36px;
}
.app-meta {
  display: flex;
  gap: 10px;
  justify-content: center;
  align-items: center;
}
.app-version {
  color: #888;
  font-size: 12px;
}
</style>
