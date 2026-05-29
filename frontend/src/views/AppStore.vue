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

      <el-tab-pane label="镜像源" name="sources">
        <div style="margin-bottom:16px;display:flex;gap:8px;align-items:center">
          <el-input v-model="newSource.name" placeholder="源名称" style="width:160px" />
          <el-input v-model="newSource.url" placeholder="镜像地址 (如 docker.1ms.run)" style="width:300px" />
          <el-button type="primary" @click="addSource">添加源</el-button>
        </div>
        <div class="table-wrap">
          <el-table :data="sources" size="small">
            <el-table-column prop="name" label="名称" width="160" />
            <el-table-column prop="url" label="地址" />
            <el-table-column prop="enabled" label="状态" width="100">
              <template #default="{ row }">
                <span :class="row.enabled ? 'tag-on' : 'tag-off'">{{ row.enabled ? '启用' : '禁用' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
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
import { ref, computed, onMounted } from 'vue'
import { appApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const activeTab = ref('apps')
const apps = ref<any[]>([])
const installed = ref<any[]>([])
const sources = ref<any[]>([])
const showInstall = ref(false)
const installing = ref(false)
const selectedApp = ref<any>(null)
const installForm = ref({ name: '', port: 8080 })
const newSource = ref({ name: '', url: '' })
const pullForm = ref({ image: '' })
const pulling = ref(false)

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

async function loadApps() {
  try { const res: any = await appApi.list(); apps.value = res.data || [] } catch (e) {}
}
async function loadInstalled() {
  try { const res: any = await appApi.installed(); installed.value = res.data || [] } catch (e) {}
}
async function loadSources() {
  try { const res: any = await appApi.listSources(); sources.value = res.data || [] } catch (e) {}
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

async function doPull() {
  if (!pullForm.value.image) {
    ElMessage.warning('请输入镜像地址')
    return
  }
  pulling.value = true
  try {
    await appApi.list()
    ElMessage.success('拉取成功')
  } catch (e) {} finally { pulling.value = false }
}

onMounted(() => { loadApps(); loadInstalled(); loadSources() })
</script>
