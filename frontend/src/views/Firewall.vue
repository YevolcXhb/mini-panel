<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">🛡️</span> 防火墙
      </h2>
    </div>

    <el-alert v-if="!serviceStatus.installed" type="warning" show-icon :closable="false" style="margin-bottom: 16px">
      <template #title>
        <div style="display: flex; align-items: center; justify-content: space-between; width: 100%">
          <span>{{ serviceStatus.message || '防火墙功能不可用' }}</span>
          <el-button size="small" type="primary" @click="runDiagnose" :loading="diagnosing">一键诊断</el-button>
        </div>
      </template>
      <template #default>
        <div style="margin-top: 8px">
          <pre v-if="serviceStatus.diagnosis" style="margin: 0; white-space: pre-wrap; color: #666; font-size: 13px; font-family: monospace">{{ serviceStatus.diagnosis }}</pre>
          <p v-else style="margin: 0; color: #666; font-size: 13px">点击"一键诊断"获取详细原因</p>
        </div>
      </template>
    </el-alert>

    <el-alert v-else-if="serviceStatus.kernel_warning" type="warning" show-icon :closable="false" style="margin-bottom: 16px">
      <template #title>内核模块警告</template>
      <template #default>
        <div style="margin-top: 8px; color: #666; font-size: 13px">{{ serviceStatus.kernel_warning }}</div>
      </template>
    </el-alert>

    <template v-else>
      <el-card style="margin-bottom: 16px">
        <div style="display: flex; justify-content: space-between; align-items: center">
          <div style="display: flex; align-items: center; gap: 12px">
            <el-tag :type="serviceStatus.running ? 'success' : 'danger'" size="large">
              {{ serviceStatus.running ? '运行中' : '已停止' }}
            </el-tag>
            <span>{{ firewallType === 'ufw' ? 'UFW' : 'firewalld' }} {{ serviceStatus.version ? 'v' + serviceStatus.version : '' }}</span>
          </div>
          <div style="display: flex; gap: 8px">
            <el-button size="small" type="primary" @click="startFirewall" v-if="!serviceStatus.running" :loading="actionLoading">启动</el-button>
            <el-button size="small" type="warning" @click="stopFirewall" v-if="serviceStatus.running" :loading="actionLoading">停止</el-button>
            <el-button size="small" @click="checkStatus">刷新状态</el-button>
            <el-button type="success" size="small" @click="applyRules" :loading="applying">应用规则</el-button>
            <el-button type="primary" size="small" @click="openDialog()">添加规则</el-button>
          </div>
        </div>
      </el-card>

      <el-table :data="rules" style="width: 100%" v-loading="loading">
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="type" label="类型" width="80">
          <template #default="{ row }">
            <el-tag :type="row.type === 'port' ? 'primary' : 'warning'">{{ row.type === 'port' ? '端口' : 'IP' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="action" label="动作" width="80">
          <template #default="{ row }">
            <el-tag :type="row.action === 'allow' ? 'success' : 'danger'">{{ row.action === 'allow' ? '允许' : '拒绝' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="protocol" label="协议" width="80" />
        <el-table-column prop="port" label="端口" width="100" />
        <el-table-column prop="ip" label="IP" width="120" />
        <el-table-column prop="direction" label="方向" width="80">
          <template #default="{ row }">
            {{ row.direction === 'in' ? '入站' : '出站' }}
          </template>
        </el-table-column>
        <el-table-column prop="enabled" label="状态" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.enabled" type="success">启用</el-tag>
            <el-tag v-else type="info">停用</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row.id)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </template>

    <el-dialog v-model="diagnoseDialogVisible" title="防火墙环境诊断报告" width="700px">
      <div v-if="diagnoseReport">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="诊断时间">{{ diagnoseReport.timestamp }}</el-descriptions-item>
          <el-descriptions-item label="操作系统">{{ diagnoseReport.platform }} {{ diagnoseReport.platform_supported ? '(支持)' : '(不支持)' }}</el-descriptions-item>
          <el-descriptions-item label="运行权限">
            <el-tag :type="diagnoseReport.is_root ? 'success' : 'danger'">{{ diagnoseReport.is_root ? 'root (有权限)' : '非 root (权限不足)' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="容器环境">
            <el-tag :type="diagnoseReport.in_container ? 'warning' : 'success'">{{ diagnoseReport.in_container ? `在容器中 (${diagnoseReport.container_type || 'unknown'})` : '物理机/虚拟机' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="可用后端">{{ (diagnoseReport.available_backends || []).join(', ') || 'none' }}</el-descriptions-item>
          <el-descriptions-item label="工具安装情况">
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <el-tag v-for="(installed, tool) in (diagnoseReport.tools_installed || {})" :key="tool" :type="installed ? 'success' : 'info'">{{ tool }}: {{ installed ? '已装' : '未装' }}</el-tag>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="内核模块">
            <div v-if="diagnoseReport.kernel_modules" style="display: flex; gap: 8px; flex-wrap: wrap">
              <el-tag v-for="(status, mod) in diagnoseReport.kernel_modules" :key="mod" :type="status === 'ok' ? 'success' : 'danger'">{{ mod }}: {{ status }}</el-tag>
            </div>
            <span v-else style="color: #c0c4cc">无</span>
          </el-descriptions-item>
          <el-descriptions-item label="总结">
            <span style="color: #f56c6c; font-weight: bold">{{ diagnoseReport.summary }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="建议">
            <pre style="margin: 0; white-space: pre-wrap; color: #666; font-size: 13px">{{ diagnoseReport.recommendation }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>
      <div v-else style="text-align: center; padding: 20px">诊断中...</div>
    </el-dialog>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑规则' : '添加规则'" width="500px">
      <el-form :model="form" label-width="100px" ref="formRef" :rules="formRules">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="如: 允许 SSH" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="form.type" placeholder="选择类型" style="width: 100%">
            <el-option label="端口规则" value="port" />
            <el-option label="IP 规则" value="ip" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作" prop="action">
          <el-select v-model="form.action" placeholder="选择动作" style="width: 100%">
            <el-option label="允许" value="allow" />
            <el-option label="拒绝" value="deny" />
          </el-select>
        </el-form-item>
        <el-form-item label="协议" v-if="form.type === 'port'">
          <el-select v-model="form.protocol" placeholder="选择协议" style="width: 100%">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
            <el-option label="全部" value="all" />
          </el-select>
        </el-form-item>
        <el-form-item label="端口" v-if="form.type === 'port'">
          <el-input v-model="form.port" placeholder="如: 22, 80, 3306-3308" />
        </el-form-item>
        <el-form-item label="IP 地址" v-if="form.type === 'ip'">
          <el-input v-model="form.ip" placeholder="如: 192.168.1.100 或 10.0.0.0/24" />
        </el-form-item>
        <el-form-item label="方向">
          <el-select v-model="form.direction" placeholder="选择方向" style="width: 100%">
            <el-option label="入站" value="in" />
            <el-option label="出站" value="out" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="2" placeholder="备注信息" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { firewallApi, systemApi } from '../api'

const rules = ref<any[]>([])
const loading = ref(false)
const applying = ref(false)
const actionLoading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<any>(null)
const serviceStatus = ref<any>({ installed: false, running: false, version: '', name: 'firewalld' })
const diagnosing = ref(false)
const diagnoseDialogVisible = ref(false)
const diagnoseReport = ref<any>(null)

const firewallType = computed(() => {
  const name = serviceStatus.value.backend || serviceStatus.value.name || 'firewalld'
  if (name === 'ufw') return 'UFW'
  if (name === 'nftables') return 'nftables'
  if (name === 'iptables') return 'iptables'
  return 'firewalld'
})

const form = reactive({
  id: 0,
  name: '',
  type: 'port',
  action: 'allow',
  protocol: 'tcp',
  port: '',
  ip: '',
  direction: 'in',
  enabled: true,
  note: ''
})

const formRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  action: [{ required: true, message: '请选择动作', trigger: 'change' }]
}

function resetForm() {
  form.id = 0
  form.name = ''
  form.type = 'port'
  form.action = 'allow'
  form.protocol = 'tcp'
  form.port = ''
  form.ip = ''
  form.direction = 'in'
  form.enabled = true
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
    const res: any = await firewallApi.status()
    serviceStatus.value = res.data || { installed: false, running: false, name: 'firewalld' }
  } catch (e: any) {
    ElMessage.error(e?.message || '检查服务状态失败')
  }
}

async function runDiagnose() {
  diagnosing.value = true
  try {
    const res: any = await firewallApi.diagnose()
    diagnoseReport.value = res.data
    diagnoseDialogVisible.value = true
  } catch (e: any) {
    ElMessage.error(e?.message || '诊断失败')
  } finally {
    diagnosing.value = false
  }
}

async function startFirewall() {
  actionLoading.value = true
  try {
    await firewallApi.start()
    ElMessage.success('防火墙启动成功')
    await checkStatus()
  } catch (e: any) {
    ElMessage.error(e?.message || '启动失败')
  } finally {
    actionLoading.value = false
  }
}

async function stopFirewall() {
  actionLoading.value = true
  try {
    await firewallApi.stop()
    ElMessage.success('防火墙停止成功')
    await checkStatus()
  } catch (e: any) {
    ElMessage.error(e?.message || '停止失败')
  } finally {
    actionLoading.value = false
  }
}

async function loadRules() {
  loading.value = true
  try {
    const res: any = await firewallApi.list()
    rules.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function saveRule() {
  await formRef.value?.validate()
  try {
    if (isEdit.value) {
      await firewallApi.update(form.id, { ...form })
      ElMessage.success('更新成功')
    } else {
      await firewallApi.create({ ...form })
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadRules()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

async function handleDelete(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除这条规则吗？', '提示', { type: 'warning' })
    await firewallApi.delete(id)
    ElMessage.success('删除成功')
    loadRules()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e?.message || '删除失败')
    }
  }
}

async function applyRules() {
  applying.value = true
  try {
    const res: any = await firewallApi.apply()
    ElMessage.success(res.message || '规则已应用')
  } catch (e: any) {
    ElMessage.error(e?.message || '应用失败')
  } finally {
    applying.value = false
  }
}

onMounted(async () => {
  await checkStatus()
  if (serviceStatus.value.installed) {
    loadRules()
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
