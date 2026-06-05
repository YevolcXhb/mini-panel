<template>
  <div>
    <h2 class="page-title">⏰ 计划任务</h2>

    <div style="margin-bottom:12px">
      <el-button type="primary" size="small" @click="showCreate = true">+ 新建任务</el-button>
    </div>

    <div class="table-wrap">
      <el-table :data="cronjobs" v-loading="loading" size="small">
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="spec" label="定时规则" width="150" />
        <el-table-column prop="command" label="命令" show-overflow-tooltip min-width="200" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-switch v-model="row.status" active-value="enabled" inactive-value="disabled" @change="(v: any) => toggleStatus(row, v)" />
          </template>
        </el-table-column>
        <el-table-column label="上次执行" width="170">
          <template #default="{ row }">{{ row.last_run ? new Date(row.last_run * 1000).toLocaleString() : '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="runNow(row)">执行</el-button>
            <el-button size="small" @click="viewLog(row)">日志</el-button>
            <el-button size="small" @click="editCronjob(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteCronjob(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="showLog" title="执行日志" width="700px">
      <div style="background:#0d1117;border-radius:8px;padding:12px;max-height:400px;overflow:auto;font-family:monospace;font-size:13px;line-height:1.6;color:#c9d1d9;white-space:pre-wrap">{{ logContent || '暂无日志' }}</div>
    </el-dialog>

    <el-dialog v-model="showCreate" :title="editing ? '编辑任务' : '新建任务'" width="600px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="任务名称" /></el-form-item>
        <el-form-item label="定时规则">
          <el-input v-model="form.spec" placeholder="如: 0 0 * * *" />
          <div style="margin-top:6px;display:flex;gap:8px;flex-wrap:wrap">
            <el-link type="primary" @click="form.spec = '*/5 * * * *'">每5分钟</el-link>
            <el-link type="primary" @click="form.spec = '0 * * * *'">每小时</el-link>
            <el-link type="primary" @click="form.spec = '0 0 * * *'">每天</el-link>
          </div>
        </el-form-item>
        <el-form-item label="命令"><el-input v-model="form.command" placeholder="要执行的命令" /></el-form-item>
        <el-form-item label="脚本"><el-input v-model="form.script" type="textarea" :rows="5" placeholder="完整脚本内容（可选）" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="saveCronjob">保存</el-button>
      </template>
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
const logContent = ref('')

const form = ref({ id: 0, name: '', spec: '', command: '', script: '' })

function viewLog(row: any) {
  logContent.value = row.last_log || ''
  showLog.value = true
}

async function loadCronjobs() {
  loading.value = true
  try { const res: any = await cronjobApi.list(); cronjobs.value = res.data || [] } catch (e) {} finally { loading.value = false }
}

function editCronjob(row: any) {
  editing.value = true
  form.value = { ...row }
  showCreate.value = true
}

function resetForm() {
  form.value = { id: 0, name: '', spec: '', command: '', script: '' }
  editing.value = false
}

async function saveCronjob() {
  try {
    if (editing.value) await cronjobApi.update(form.value.id, form.value)
    else await cronjobApi.create(form.value)
    ElMessage.success('保存成功')
    showCreate.value = false
    resetForm()
    loadCronjobs()
  } catch (e) {}
}

async function toggleStatus(row: any, status: string) {
  try { await cronjobApi.update(row.id, { ...row, status }); ElMessage.success('状态已更新') } catch (e) {}
}

async function runNow(row: any) {
  try { await cronjobApi.run(row.id); ElMessage.success('执行成功') } catch (e) {}
}

async function deleteCronjob(row: any) {
  try {
    await ElMessageBox.confirm(`确定删除 ${row.name} 吗？`, '确认', { confirmButtonClass: 'el-button--danger' })
    await cronjobApi.delete(row.id)
    ElMessage.success('删除成功')
    loadCronjobs()
  } catch (e) {}
}

onMounted(loadCronjobs)
</script>
