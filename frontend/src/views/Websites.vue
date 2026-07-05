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
          <el-table-column label="操作" width="240" fixed="right">
            <template #default="{ row }">
              <el-button size="small" @click="editWebsite(row)">编辑</el-button>
              <el-button size="small" @click="toggleWebsite(row)">{{ row.enabled ? '停用' : '启用' }}</el-button>
              <el-button size="small" type="danger" @click="deleteWebsite(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <el-dialog v-model="showForm" :title="editing ? '编辑网站' : '添加网站'" width="600px">
        <el-form :model="form" label-width="100px">
          <el-form-item label="名称" required>
            <el-input v-model="form.name" placeholder="如：主站" />
          </el-form-item>
          <el-form-item label="域名" required>
            <el-input v-model="form.domain" placeholder="如：example.com" />
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
          <el-form-item label="代理目标" v-if="form.type === 'proxy'">
            <el-input v-model="form.proxy_target" placeholder="如：http://localhost:8080" />
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
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { websiteApi, systemApi } from '../api'

const websites = ref<any[]>([])
const loading = ref(false)
const showForm = ref(false)
const editing = ref(false)
const installing = ref(false)
const actionLoading = ref(false)
const serviceStatus = ref<any>({ installed: false, running: false, version: '' })

const form = ref<any>({ name: '', domain: '', port: 80, root: '', type: 'static', proxy_target: '', ssl: false, ssl_cert: '', ssl_key: '', remark: '' })

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
  form.value = { name: '', domain: '', port: 80, root: '', type: 'static', proxy_target: '', ssl: false, ssl_cert: '', ssl_key: '', remark: '' }
  showForm.value = true
}

function editWebsite(row: any) {
  editing.value = true
  form.value = { ...row }
  showForm.value = true
}

async function saveWebsite() {
  try {
    if (editing.value) {
      await websiteApi.update(form.value.id, form.value)
      ElMessage.success('更新成功')
    } else {
      await websiteApi.create(form.value)
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
    await websiteApi.toggle(row.id, !row.enabled)
    ElMessage.success('状态已更新')
    loadWebsites()
  } catch (e: any) {
    ElMessage.error(e?.message || '操作失败')
  }
}

async function deleteWebsite(row: any) {
  try {
    await ElMessageBox.confirm(`确定删除 ${row.name} (${row.domain}) 吗？`, '确认删除', { confirmButtonClass: 'el-button--danger' })
    await websiteApi.delete(row.id)
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

onMounted(async () => {
  await checkStatus()
  if (serviceStatus.value.installed) {
    loadWebsites()
  }
})
</script>
