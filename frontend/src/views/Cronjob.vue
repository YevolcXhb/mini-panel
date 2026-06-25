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
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="(v: any) => toggleStatus(row, v)" />
          </template>
        </el-table-column>
        <el-table-column label="上次执行" width="170">
          <template #default="{ row }">
            <span v-if="row.last_run_at">{{ new Date(row.last_run_at).toLocaleString() }}</span>
            <el-tag v-else size="small" type="info">未执行</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="上次状态" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.last_status === 'success'" size="small" type="success">成功</el-tag>
            <el-tag v-else-if="row.last_status === 'failed'" size="small" type="danger">失败</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="runNow(row)">立即执行</el-button>
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
      <el-form :model="form" label-width="90px" :rules="formRules" ref="formRef">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="任务名称" />
        </el-form-item>
        <el-form-item label="定时规则" prop="spec">
          <el-input v-model="form.spec" placeholder="如: 0 0 * * * (分 时 日 月 周)" />
          <div style="margin-top:6px;display:flex;gap:8px;flex-wrap:wrap">
            <el-link type="primary" @click="form.spec = '*/5 * * * *'">每5分钟</el-link>
            <el-link type="primary" @click="form.spec = '0 * * * *'">每小时整点</el-link>
            <el-link type="primary" @click="form.spec = '0 0 * * *'">每天零点</el-link>
            <el-link type="primary" @click="form.spec = '0 0 * * 0'">每周日零点</el-link>
          </div>
        </el-form-item>
        <el-form-item label="执行方式" prop="type">
          <el-radio-group v-model="form.type">
            <el-radio label="command">命令</el-radio>
            <el-radio label="script">脚本</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="命令" prop="command" v-if="form.type === 'command' || !form.type">
          <el-input v-model="form.command" placeholder="要执行的命令，如: echo hello" />
        </el-form-item>
        <el-form-item label="脚本" prop="script" v-if="form.type === 'script'">
          <el-input v-model="form.script" type="textarea" :rows="5" placeholder="完整脚本内容" />
        </el-form-item>
        <el-form-item label="超时(秒)">
          <el-input-number v-model="form.timeout" :min="1" :max="86400" :default-value="300" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="2" placeholder="备注信息" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="saveCronjob" :loading="saving">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { cronjobApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const cronjobs = ref<any[]>([])
const loading = ref(false)
const saving = ref(false)
const showCreate = ref(false)
const showLog = ref(false)
const editing = ref(false)
const logContent = ref('')
const formRef = ref<any>(null)

const form = reactive({ id: 0, name: '', spec: '', type: 'command', command: '', script: '', timeout: 300, note: '', enabled: true })

const formRules = {
  name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
  spec: [{ required: true, message: '请输入定时规则', trigger: 'blur' }],
  command: [{ required: true, message: '请输入要执行的命令', trigger: 'blur' }]
}

function viewLog(row: any) {
  logContent.value = row.last_log || '暂无执行日志'
  showLog.value = true
}

async function loadCronjobs() {
  loading.value = true
  try {
    const res: any = await cronjobApi.list()
    cronjobs.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

function editCronjob(row: any) {
  editing.value = true
  Object.assign(form, { id: 0, name: '', spec: '', type: 'command', command: '', script: '', timeout: 300, note: '', enabled: true }, row)
  showCreate.value = true
}

function resetForm() {
  Object.assign(form, { id: 0, name: '', spec: '', type: 'command', command: '', script: '', timeout: 300, note: '', enabled: true })
  editing.value = false
}

async function saveCronjob() {
  await formRef.value?.validate()
  saving.value = true
  try {
    const data = { ...form }
    if (data.type === 'script') {
      data.command = data.script
    }
    if (editing.value) {
      await cronjobApi.update(form.id, data)
    } else {
      await cronjobApi.create(data)
    }
    ElMessage.success('保存成功')
    showCreate.value = false
    resetForm()
    loadCronjobs()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleStatus(row: any, enabled: boolean) {
  try {
    await cronjobApi.update(row.id, { enabled })
    ElMessage.success('状态已更新')
    loadCronjobs()
  } catch (e: any) {
    ElMessage.error(e?.message || '更新失败')
    row.enabled = !enabled
  }
}

async function runNow(row: any) {
  try {
    await cronjobApi.run(row.id)
    ElMessage.success('任务已提交执行')
    setTimeout(loadCronjobs, 1000)
  } catch (e: any) {
    ElMessage.error(e?.message || '执行失败')
  }
}

async function deleteCronjob(row: any) {
  try {
    await ElMessageBox.confirm(`确定删除 ${row.name} 吗？`, '确认删除', { confirmButtonClass: 'el-button--danger' })
    await cronjobApi.delete(row.id)
    ElMessage.success('删除成功')
    loadCronjobs()
  } catch (e) {}
}

onMounted(loadCronjobs)
</script>
