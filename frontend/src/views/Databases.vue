<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">🗄️</span> 数据库管理
      </h2>
      <el-button type="primary" @click="openDialog()">添加数据库</el-button>
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
      <el-table-column prop="database" label="数据库名" width="120" />
      <el-table-column prop="ssl" label="SSL" width="70">
        <template #default="{ row }">
          <el-tag v-if="row.ssl" type="success">是</el-tag>
          <el-tag v-else type="info">否</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="enabled" label="状态" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.enabled" type="success">启用</el-tag>
          <el-tag v-else type="danger">停用</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="testConnection(row)">测试</el-button>
          <el-button size="small" @click="openDialog(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="handleDelete(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑数据库' : '添加数据库'" width="500px">
      <el-form :model="form" label-width="100px" ref="formRef" :rules="rules">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="如: 生产 MySQL" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="form.type" placeholder="选择数据库类型" style="width: 100%">
            <el-option label="MySQL" value="mysql" />
            <el-option label="PostgreSQL" value="postgresql" />
            <el-option label="Redis" value="redis" />
            <el-option label="MongoDB" value="mongodb" />
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
        <el-form-item label="数据库名">
          <el-input v-model="form.database" placeholder="默认数据库名" />
        </el-form-item>
        <el-form-item label="SSL">
          <el-switch v-model="form.ssl" />
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
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { databaseApi } from '../api'

const databases = ref<any[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<any>(null)

const form = reactive({
  id: 0,
  name: '',
  type: 'mysql',
  host: '127.0.0.1',
  port: 3306,
  username: '',
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

const defaultPort: Record<string, number> = {
  mysql: 3306,
  postgresql: 5432,
  redis: 6379,
  mongodb: 27017
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
  form.username = ''
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
      host: row.host,
      port: row.port
    })
    ElMessage.success(res.message || '连接成功')
  } catch (e: any) {
    ElMessage.error(e?.message || '连接失败')
  }
}

onMounted(loadDatabases)
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
