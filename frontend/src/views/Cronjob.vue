<template>
  <div>
    <el-card>
      <template #header>
        <div class="cron-header">
          <span>计划任务</span>
          <el-button type="primary" size="small" @click="showCreate = true">新建任务</el-button>
        </div>
      </template>
      <el-table :data="cronjobs" v-loading="loading" size="small">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="spec" label="定时规则" width="150" />
        <el-table-column prop="command" label="命令" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-switch v-model="row.status" active-value="enabled" inactive-value="disabled" @change="(v: any) => toggleStatus(row, v)" />
          </template>
        </el-table-column>
        <el-table-column label="上次执行" width="180">
          <template #default="{ row }">
            {{ row.last_run ? new Date(row.last_run * 1000).toLocaleString() : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="runNow(row)">执行</el-button>
            <el-button size="small" @click="editCronjob(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteCronjob(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="showCreate" :title="editing ? '编辑任务' : '新建任务'" width="600px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="任务名称" />
        </el-form-item>
        <el-form-item label="定时规则">
          <el-input v-model="form.spec" placeholder="如: 0 0 * * *" />
          <div class="cron-help">
            <span>格式: 分 时 日 月 周</span>
            <el-link type="primary" @click="form.spec = '*/5 * * * *'">每5分钟</el-link>
            <el-link type="primary" @click="form.spec = '0 * * * *'">每小时</el-link>
            <el-link type="primary" @click="form.spec = '0 0 * * *'">每天</el-link>
          </div>
        </el-form-item>
        <el-form-item label="命令">
          <el-input v-model="form.command" placeholder="要执行的命令" />
        </el-form-item>
        <el-form-item label="脚本">
          <el-input v-model="form.script" type="textarea" :rows="5" placeholder="完整脚本内容（可选）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="saveCronjob">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showLog" title="执行日志" width="600px">
      <pre class="log-content">{{ currentLog }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { cronjobApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const cronjobs = ref<any[]>([])
const loading = ref(false)
const showCreate = ref(false)
const showLog = ref(false)
const editing = ref(false)
const currentLog = ref('')

const form = ref({
  id: 0,
  name: '',
  spec: '',
  command: '',
  script: ''
})

async function loadCronjobs() {
  loading.value = true
  try {
    const res: any = await cronjobApi.list()
    cronjobs.value = res.data || []
  } catch (e) {
  } finally {
    loading.value = false
  }
}

function editCronjob(row: any) {
  editing.value = true
  form.value = { ...row }
  showCreate.value = true
}

async function saveCronjob() {
  try {
    if (editing.value) {
      await cronjobApi.update(form.value.id, form.value)
    } else {
      await cronjobApi.create(form.value)
    }
    ElMessage.success('保存成功')
    showCreate.value = false
    editing.value = false
    resetForm()
    loadCronjobs()
  } catch (e) {}
}

async function toggleStatus(row: any, status: string) {
  try {
    await cronjobApi.update(row.id, { status })
    ElMessage.success('状态已更新')
  } catch (e) {
    row.status = status === 'enabled' ? 'disabled' : 'enabled'
  }
}

async function runNow(row: any) {
  try {
    await cronjobApi.run(row.id)
    ElMessage.success('执行成功')
    loadCronjobs()
    currentLog.value = row.last_log || ''
    showLog.value = true
  } catch (e) {}
}

async function deleteCronjob(row: any) {
  try {
    await ElMessageBox.confirm(`确定要删除任务 ${row.name} 吗？`, '确认删除')
    await cronjobApi.delete(row.id)
    ElMessage.success('删除成功')
    loadCronjobs()
  } catch (e) {}
}

function resetForm() {
  form.value = { id: 0, name: '', spec: '', command: '', script: '' }
}

onMounted(loadCronjobs)
</script>

<style scoped>
.cron-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.cron-help {
  margin-top: 5px;
  font-size: 12px;
  color: #888;
}
.cron-help .el-link {
  margin-left: 10px;
}
.log-content {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 15px;
  border-radius: 4px;
  max-height: 400px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
