<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">👤</span> 用户管理
      </h2>
      <el-button type="primary" @click="openDialog()">添加用户</el-button>
    </div>

    <el-table :data="users" style="width: 100%" v-loading="loading">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="username" label="用户名" />
      <el-table-column prop="role" label="角色" width="120">
        <template #default="scope">
          <el-tag v-if="scope.row.role === 'admin'" type="danger" size="small">管理员</el-tag>
          <el-tag v-else type="info" size="small">普通用户</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="160">
        <template #default="scope">{{ formatTime(scope.row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="scope">
          <el-button size="small" @click="openResetDialog(scope.row)">重置密码</el-button>
          <el-button size="small" @click="openDialog(scope.row)">编辑角色</el-button>
          <el-popconfirm title="确定删除该用户?" @confirm="deleteUser(scope.row.id)">
            <template #reference>
              <el-button type="danger" size="small" :disabled="scope.row.id === currentUserId">删除</el-button>
            </template>
          </el-popconfirm>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑用户' : '添加用户'" width="450px">
      <el-form :model="form" :rules="rules" ref="formRef" label-width="80px">
        <el-form-item label="用户名" prop="username" v-if="!isEdit">
          <el-input v-model="form.username" placeholder="username" />
        </el-form-item>
        <el-form-item label="密码" prop="password" v-if="!isEdit">
          <el-input v-model="form.password" type="password" placeholder="password" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-select v-model="form.role" style="width: 100%">
            <el-option label="管理员" value="admin" />
            <el-option label="普通用户" value="user" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveUser">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="resetDialogVisible" title="重置密码" width="400px">
      <el-form :model="resetForm" :rules="resetRules" ref="resetFormRef" label-width="80px">
        <el-form-item label="新密码" prop="password">
          <el-input v-model="resetForm.password" type="password" placeholder="新密码" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveReset">确认重置</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { userApi } from '../api'

const users = ref<any[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const resetDialogVisible = ref(false)
const isEdit = ref(false)
const currentUserId = ref(0)
const currentResetId = ref(0)
const formRef = ref<any>(null)
const resetFormRef = ref<any>(null)

const form = reactive({ id: 0, username: '', password: '', role: 'user' })
const resetForm = reactive({ password: '' })

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }]
}

const resetRules = {
  password: [{ required: true, message: '请输入新密码', trigger: 'blur' }]
}

function formatTime(ts: string | number) {
  if (!ts) return '-'
  const d = new Date(ts as any)
  return d.toLocaleString()
}

async function loadUsers() {
  loading.value = true
  try {
    const res: any = await userApi.list()
    users.value = res.data || []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

function openDialog(row?: any) {
  isEdit.value = !!row
  if (row) {
    Object.assign(form, { id: row.id, username: row.username, password: '', role: row.role })
  } else {
    Object.assign(form, { id: 0, username: '', password: '', role: 'user' })
  }
  dialogVisible.value = true
}

function openResetDialog(row: any) {
  currentResetId.value = row.id
  resetForm.password = ''
  resetDialogVisible.value = true
}

async function saveUser() {
  await formRef.value?.validate()
  try {
    if (isEdit.value) {
      await userApi.update(form.id, { role: form.role })
      ElMessage.success('更新成功')
    } else {
      await userApi.create({ username: form.username, password: form.password, role: form.role })
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadUsers()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}

async function saveReset() {
  await resetFormRef.value?.validate()
  try {
    await userApi.resetPassword(currentResetId.value, { password: resetForm.password })
    ElMessage.success('密码重置成功')
    resetDialogVisible.value = false
  } catch (e: any) {
    ElMessage.error(e?.message || '重置失败')
  }
}

async function deleteUser(id: number) {
  try {
    await userApi.deleteUser(id)
    ElMessage.success('删除成功')
    loadUsers()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

onMounted(() => {
  loadUsers()
})
</script>
