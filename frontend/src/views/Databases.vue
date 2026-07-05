<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">🗄️</span> 数据库管理
      </h2>
    </div>

    <el-alert v-if="!serviceStatus.installed" type="warning" show-icon style="margin-bottom: 16px">
      <template #title>
        MySQL/MariaDB 未检测到，请先安装数据库服务
      </template>
      <template #default>
        <div style="margin-top: 8px">
          <el-button type="primary" :loading="installing" @click="installService">安装 MySQL/MariaDB</el-button>
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
            <span v-if="serviceStatus.version">{{ serviceStatus.name === 'ufw' ? '' : 'MySQL' }} v{{ serviceStatus.version }}</span>
          </div>
          <div style="display: flex; gap: 8px">
            <el-button size="small" type="primary" @click="startService" v-if="!serviceStatus.running" :loading="actionLoading">启动</el-button>
            <el-button size="small" type="warning" @click="stopService" v-if="serviceStatus.running" :loading="actionLoading">停止</el-button>
            <el-button size="small" @click="restartService" :loading="actionLoading">重启</el-button>
            <el-button size="small" @click="checkStatus">刷新状态</el-button>
          </div>
        </div>
      </el-card>

      <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center">
        <el-button type="primary" @click="openDialog()">添加数据库实例</el-button>
      </div>

      <el-table :data="databases" style="width: 100%" v-loading="loading">
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="getTypeTag(row.type)">{{ row.type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="host" label="主机" min-width="120" />
        <el-table-column prop="port" label="端口" width="80" />
        <el-table-column prop="username" label="用户名" width="100" />
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="testConnection(row)">测试连接</el-button>
            <el-button size="small" @click="openDbDialog(row)">数据库</el-button>
            <el-button size="small" @click="openDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row.id)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </template>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑数据库实例' : '添加数据库实例'" width="500px">
      <el-form :model="form" label-width="100px" ref="formRef" :rules="rules">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="如: 本地 MySQL" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="form.type" placeholder="选择数据库类型" style="width: 100%">
            <el-option label="MySQL" value="mysql" />
          </el-select>
        </el-form-item>
        <el-form-item label="主机" prop="host">
          <el-input v-model="form.host" placeholder="如: 127.0.0.1" />
        </el-form-item>
        <el-form-item label="端口" prop="port">
          <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 100%" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="form.username" placeholder="用户名" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.password" type="password" placeholder="密码" show-password />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="2" placeholder="备注信息" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveDatabase">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="dbDialogVisible" title="数据库管理" width="700px">
      <div style="margin-bottom: 16px; display: flex; gap: 8px">
        <el-button type="primary" size="small" @click="openCreateDbDialog">创建数据库</el-button>
        <el-button size="small" @click="loadDbs">刷新</el-button>
      </div>
      <el-table :data="dbList" v-loading="dbLoading" size="small">
        <el-table-column prop="name" label="数据库名" />
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="dropDb(row.name)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog v-model="createDbDialogVisible" title="创建数据库" width="400px">
      <el-form label-width="80px">
        <el-form-item label="数据库名">
          <el-input v-model="newDbName" placeholder="输入数据库名" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDbDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="createDb">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { databaseApi, systemApi } from '../api'

const databases = ref<any[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const dbDialogVisible = ref(false)
const createDbDialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<any>(null)
const installing = ref(false)
const actionLoading = ref(false)
const serviceStatus = ref<any>({ installed: false, running: false, version: '', name: 'mysql' })
const currentDbInstance = ref<any>(null)
const dbList = ref<any[]>([])
const dbLoading = ref(false)
const newDbName = ref('')

const form = reactive({
  id: 0,
  name: '',
  type: 'mysql',
  host: '127.0.0.1',
  port: 3306,
  username: 'root',
  password: '',
  database: '',
  ssl: false,
  note: ''
})

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  host: [{ required: true, message: '请输入主机地址', trigger: 'blur' }],
  port: [{ required: true, message: '请输入端口', trigger: 'blur' }]
}

function getTypeTag(type: string) {
  const map: Record<string, string> = {
    mysql: 'primary',
    postgresql: 'success',
    redis: 'warning',
    mongodb: 'danger'
  }
  return map[type] || 'info'
}

function resetForm() {
  form.id = 0
  form.name = ''
  form.type = 'mysql'
  form.host = '127.0.0.1'
  form.port = 3306
  form.username = 'root'
  form.password = ''
  form.database = ''
  form.ssl = false
  form.note = ''
}

function openDialog(row?: any) {
  if (row) {
    isEdit.value = true
    Object.assign(form, row)
  } else {
    isEdit.value = false
    resetForm()
  }
  dialogVisible.value = true
}

async function checkStatus() {
  try {
    const res: any = await systemApi.checkServices()
    serviceStatus.value = res.data?.mysql || { installed: false, running: false }
  } catch (e: any) {
    ElMessage.error(e?.message || '检查服务状态失败')
  }
}

async function installService() {
  installing.value = true
  try {
    await systemApi.installService('mysql')
    ElMessage.success('MySQL/MariaDB 安装成功')
    await checkStatus()
    if (serviceStatus.value.installed) {
      loadDatabases()
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
    await systemApi.startService('mysql')
    ElMessage.success('MySQL 启动成功')
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
    await systemApi.stopService('mysql')
    ElMessage.success('MySQL 停止成功')
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
    await systemApi.restartService('mysql')
    ElMessage.success('MySQL 重启成功')
    await checkStatus()
  } catch (e: any) {
    ElMessage.error(e?.message || '重启失败')
  } finally {
    actionLoading.value = false
  }
}

async function loadDatabases() {
  loading.value = true
  try {
    const res: any = await databaseApi.list()
    databases.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function saveDatabase() {
  await formRef.value?.validate()
  try {
    if (isEdit.value) {
      await databaseApi.update(form.id, { ...form })
      ElMessage.success('更新成功')
    } else {
      await databaseApi.create({ ...form })
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadDatabases()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

async function handleDelete(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除这个数据库实例吗？', '提示', { type: 'warning' })
    await databaseApi.delete(id)
    ElMessage.success('删除成功')
    loadDatabases()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e?.message || '删除失败')
    }
  }
}

async function testConnection(row: any) {
  try {
    const res: any = await databaseApi.test({
      type: row.type || 'mysql',
      host: row.host,
      port: row.port,
      username: row.username,
      password: row.password
    })
    ElMessage.success(res.message || '连接成功')
  } catch (e: any) {
    ElMessage.error(e?.message || '连接失败')
  }
}

function openDbDialog(row: any) {
  currentDbInstance.value = row
  dbDialogVisible.value = true
  loadDbs()
}

async function loadDbs() {
  if (!currentDbInstance.value) return
  dbLoading.value = true
  try {
    const res: any = await databaseApi.listDatabases(currentDbInstance.value.id)
    dbList.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载数据库列表失败')
  } finally {
    dbLoading.value = false
  }
}

function openCreateDbDialog() {
  newDbName.value = ''
  createDbDialogVisible.value = true
}

async function createDb() {
  if (!newDbName.value.trim()) {
    ElMessage.warning('请输入数据库名')
    return
  }
  try {
    await databaseApi.createDatabase(currentDbInstance.value.id, newDbName.value.trim())
    ElMessage.success('数据库创建成功')
    createDbDialogVisible.value = false
    loadDbs()
  } catch (e: any) {
    ElMessage.error(e?.message || '创建失败')
  }
}

async function dropDb(dbName: string) {
  try {
    await ElMessageBox.confirm(`确定要删除数据库 ${dbName} 吗？此操作不可恢复！`, '危险操作', { type: 'error' })
    ElMessage.info('暂不支持删除数据库操作')
  } catch (e) {}
}

onMounted(async () => {
  await checkStatus()
  if (serviceStatus.value.installed) {
    loadDatabases()
  }
})
</script>

<style scoped>
.page-container {
  padding: 20px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
.page-title {
  font-size: 1.5rem;
  font-weight: 600;
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}
.icon {
  font-size: 1.3rem;
}
</style>
