<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">💾</span> 备份恢复
      </h2>
      <el-button type="primary" @click="openTaskDialog()">添加备份任务</el-button>
    </div>

    <el-card class="section-card" shadow="never">
      <template #header><span>备份任务</span></template>
      <el-table :data="tasks" style="width: 100%" v-loading="loadingTasks">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="type" label="类型" width="100">
          <template #default="scope">
            <el-tag size="small" :type="scope.row.type === 'website' ? 'primary' : scope.row.type === 'database' ? 'success' : 'warning'">
              {{ typeLabel(scope.row.type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="schedule" label="定时策略" width="180">
          <template #default="scope">
            <span>{{ scope.row.schedule || '手动执行' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="keep_count" label="保留份数" width="90" />
        <el-table-column prop="enabled" label="状态" width="80">
          <template #default="scope">
            <el-switch v-model="scope.row.enabled" @change="toggleEnabled(scope.row)" />
          </template>
        </el-table-column>
        <el-table-column label="上次执行结果" width="100">
          <template #default="scope">
            <el-tag v-if="scope.row.last_status === 'success'" type="success" size="small">成功</el-tag>
            <el-tag v-else-if="scope.row.last_status === 'failed'" type="danger" size="small">失败</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="上次执行时间" width="160">
          <template #default="scope">{{ formatTime(scope.row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="scope">
            <el-button type="primary" size="small" :loading="runningId === scope.row.id" @click="runTask(scope.row.id)">立即执行</el-button>
            <el-button size="small" @click="openTaskDialog(scope.row)">编辑</el-button>
            <el-popconfirm title="确定删除?" @confirm="deleteTask(scope.row.id)">
              <template #reference>
                <el-button type="danger" size="small">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card" shadow="never" style="margin-top: 16px">
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <span>备份记录</span>
          <div style="display:flex;gap:8px">
            <el-select v-model="filterTaskId" placeholder="全部任务" size="small" clearable @change="loadRecords" style="width: 200px">
              <el-option label="全部任务" :value="0" />
              <el-option v-for="t in tasks" :key="t.id" :label="t.name" :value="t.id" />
            </el-select>
            <el-button size="small" @click="loadRecords">刷新</el-button>
          </div>
        </div>
      </template>
      <el-table :data="records" style="width: 100%" v-loading="loadingRecords">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column label="任务名称" width="120">
          <template #default="scope">{{ getTaskName(scope.row.task_id) }}</template>
        </el-table-column>
        <el-table-column prop="file_name" label="文件名" show-overflow-tooltip />
        <el-table-column prop="file_size" label="大小" width="100">
          <template #default="scope">{{ formatSize(scope.row.file_size) }}</template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90">
          <template #default="scope">
            <el-tag v-if="scope.row.status === 'success'" type="success" size="small">成功</el-tag>
            <el-tag v-else-if="scope.row.status === 'failed'" type="danger" size="small">失败</el-tag>
            <el-tag v-else type="warning" size="small">执行中</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="160">
          <template #default="scope">{{ formatTime(scope.row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="scope">
            <el-button type="success" size="small" :disabled="scope.row.status !== 'success'" @click="restoreRecord(scope.row.id)">恢复</el-button>
            <el-popconfirm title="确定删除记录和备份文件?" @confirm="deleteRecord(scope.row.id)">
              <template #reference>
                <el-button type="danger" size="small">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑备份任务' : '添加备份任务'" width="550px">
      <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
        <el-form-item label="任务名称" prop="name">
          <el-input v-model="form.name" placeholder="例如：网站每日备份" />
        </el-form-item>
        <el-form-item label="备份类型" prop="type">
          <el-select v-model="form.type" style="width: 100%" @change="onTypeChange">
            <el-option label="网站目录备份" value="website" />
            <el-option label="数据库备份" value="database" />
            <el-option label="文件/目录备份" value="files" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.type === 'website'" label="选择网站" prop="source_id">
          <el-select v-model="form.source_id" style="width: 100%" placeholder="选择要备份的网站">
            <el-option v-for="w in websites" :key="w.id" :label="w.name + ' (' + w.domain + ')'" :value="w.id" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.type === 'database'" label="选择数据库" prop="source_id">
          <el-select v-model="form.source_id" style="width: 100%" placeholder="选择要备份的数据库实例">
            <el-option v-for="d in databases" :key="d.id" :label="d.name + ' (' + d.host + ':' + d.port + ')'" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.type === 'files'" label="源路径" prop="source_path">
          <el-input v-model="form.source_path" placeholder="/var/www/html 或 /data/www" />
        </el-form-item>
        <el-form-item label="备份目录" prop="target_dir">
          <el-input v-model="form.target_dir" placeholder="/data/backups" />
        </el-form-item>
        <el-form-item label="定时策略">
          <el-input v-model="form.schedule" placeholder="Cron 表达式，留空为手动执行" />
          <div style="margin-top:6px;display:flex;gap:8px;flex-wrap:wrap">
            <el-link type="primary" @click="form.schedule = '0 2 * * *'">每天凌晨2点</el-link>
            <el-link type="primary" @click="form.schedule = '0 2 * * 0'">每周日凌晨2点</el-link>
            <el-link type="primary" @click="form.schedule = '0 2 1 * *'">每月1号凌晨2点</el-link>
            <el-link type="primary" @click="form.schedule = '0 */6 * * *'">每6小时</el-link>
          </div>
        </el-form-item>
        <el-form-item label="保留份数">
          <el-input-number v-model="form.keep_count" :min="1" :max="100" />
          <span style="margin-left:8px;color:#909399;font-size:12px">自动删除旧备份</span>
        </el-form-item>
        <el-form-item label="启用任务">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="2" placeholder="备份任务说明" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveTask" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { backupApi, websiteApi, databaseApi } from '../api'

const tasks = ref<any[]>([])
const records = ref<any[]>([])
const websites = ref<any[]>([])
const databases = ref<any[]>([])
const loadingTasks = ref(false)
const loadingRecords = ref(false)
const saving = ref(false)
const runningId = ref<number | null>(null)
const dialogVisible = ref(false)
const isEdit = ref(false)
const filterTaskId = ref(0)
const formRef = ref<any>(null)

const form = reactive({
  id: 0,
  name: '',
  type: 'website',
  source_id: null as number | null,
  source_path: '',
  target_dir: '',
  schedule: '',
  keep_count: 7,
  enabled: true,
  note: ''
})

const rules = {
  name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择备份类型', trigger: 'change' }],
  source_id: [{ required: true, message: '请选择备份源', trigger: 'change' }],
  source_path: [{ required: true, message: '请输入源路径', trigger: 'blur' }]
}

function typeLabel(t: string) {
  const map: Record<string, string> = { website: '网站', database: '数据库', files: '文件' }
  return map[t] || t
}

function formatSize(bytes: number) {
  if (!bytes || bytes === 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let size = bytes
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++ }
  return `${size.toFixed(1)} ${units[i]}`
}

function formatTime(timeStr: string) {
  if (!timeStr) return '-'
  try {
    return new Date(timeStr).toLocaleString()
  } catch {
    return timeStr
  }
}

function getTaskName(taskId: number) {
  const task = tasks.value.find(t => t.id === taskId)
  return task ? task.name : `任务#${taskId}`
}

async function loadTasks() {
  loadingTasks.value = true
  try {
    const res: any = await backupApi.listTasks()
    tasks.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载备份任务失败')
  } finally {
    loadingTasks.value = false
  }
}

async function loadRecords() {
  loadingRecords.value = true
  try {
    const res: any = await backupApi.listRecords(filterTaskId.value || undefined)
    records.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载备份记录失败')
  } finally {
    loadingRecords.value = false
  }
}

async function loadRefs() {
  try {
    const [wRes, dRes]: any = await Promise.all([
      websiteApi.list().catch(() => ({ data: [] })),
      databaseApi.list().catch(() => ({ data: [] }))
    ])
    websites.value = wRes.data || []
    databases.value = dRes.data || []
  } catch (e) {
    console.error('Load refs failed', e)
  }
}

function openTaskDialog(row?: any) {
  isEdit.value = !!row
  if (row) {
    Object.assign(form, {
      id: 0, name: '', type: 'website', source_id: null,
      source_path: '', target_dir: '/data/backups',
      schedule: '', keep_count: 7, enabled: true, note: '',
      ...row
    })
  } else {
    Object.assign(form, {
      id: 0, name: '', type: 'website', source_id: null,
      source_path: '', target_dir: '/data/backups',
      schedule: '', keep_count: 7, enabled: true, note: ''
    })
  }
  dialogVisible.value = true
}

function onTypeChange() {
  form.source_id = null
  form.source_path = ''
}

async function saveTask() {
  await formRef.value?.validate()
  saving.value = true
  try {
    const data = { ...form }
    if (data.type === 'website' || data.type === 'database') {
      data.source_path = ''
    } else {
      data.source_id = null
    }
    if (isEdit.value) {
      await backupApi.updateTask(form.id, data)
      ElMessage.success('备份任务更新成功')
    } else {
      await backupApi.createTask(data)
      ElMessage.success('备份任务创建成功')
    }
    dialogVisible.value = false
    loadTasks()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(row: any) {
  try {
    await backupApi.updateTask(row.id, { enabled: row.enabled })
    ElMessage.success('任务状态更新成功')
  } catch (e: any) {
    ElMessage.error(e?.message || '更新失败')
    row.enabled = !row.enabled
  }
}

async function runTask(id: number) {
  runningId.value = id
  try {
    const res: any = await backupApi.runTask(id)
    ElMessage.success(res.message || '备份任务已开始执行')
    setTimeout(() => { loadTasks(); loadRecords() }, 2000)
  } catch (e: any) {
    ElMessage.error(e?.message || '执行失败')
  } finally {
    runningId.value = null
  }
}

async function deleteTask(id: number) {
  try {
    await backupApi.deleteTask(id)
    ElMessage.success('备份任务已删除')
    loadTasks()
    loadRecords()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

async function restoreRecord(id: number) {
  try {
    await ElMessageBox.confirm(
      '恢复操作会覆盖现有数据，请确保已了解风险！确定继续恢复吗？',
      '危险操作',
      { type: 'warning', confirmButtonText: '确定恢复', cancelButtonText: '取消' }
    )
    const res: any = await backupApi.restoreRecord(id)
    ElMessage.success(res.message || '备份恢复成功')
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '恢复失败')
  }
}

async function deleteRecord(id: number) {
  try {
    await backupApi.deleteRecord(id)
    ElMessage.success('备份记录已删除')
    loadRecords()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

onMounted(() => {
  loadTasks()
  loadRecords()
  loadRefs()
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
.section-card {
  border-radius: 8px;
}
</style>
