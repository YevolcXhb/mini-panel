<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">🛡️</span> 防火墙
      </h2>
      <div>
        <el-button type="success" @click="applyRules" :loading="applying">应用规则</el-button>
        <el-button type="primary" @click="openDialog()">添加规则</el-button>
      </div>
    </div>

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
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { firewallApi } from '../api'

const rules = ref<any[]>([])
const loading = ref(false)
const applying = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<any>(null)

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

onMounted(loadRules)
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
