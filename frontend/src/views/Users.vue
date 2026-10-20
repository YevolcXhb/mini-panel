<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">👤</span> 用户管理
      </h2>
      <el-button type="primary" @click="openDialog()">添加用户</el-button>
    </div>

    <div class="table-wrap">
    <el-table :data="users" style="width: 100%" v-loading="loading">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="username" label="用户名" />
      <el-table-column prop="role" label="角色" width="120">
        <template #default="scope">
          <el-tag v-if="scope.row.role === 'admin'" type="danger" size="small">管理员</el-tag>
          <el-tag v-else type="info" size="small">普通用户</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="已授权功能" min-width="220">
        <template #default="scope">
          <span v-if="scope.row.role === 'admin'" class="perm-admin">全部功能</span>
          <span v-else class="perm-tags">
            <el-tag
              v-for="key in parsePerms(scope.row.permissions)"
              :key="key"
              size="small"
              type="success"
              effect="plain"
              class="perm-tag"
            >{{ featureName(key) }}</el-tag>
            <span v-if="parsePerms(scope.row.permissions).length === 0" class="perm-empty">未配置</span>
          </span>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" width="160">
        <template #default="scope">{{ formatTime(scope.row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" :fixed="isWide ? 'right' : false">
        <template #default="scope">
          <div class="row-actions">
            <el-button size="small" @click="openResetDialog(scope.row)">重置密码</el-button>
            <el-button size="small" @click="openDialog(scope.row)">编辑权限</el-button>
            <el-popconfirm title="确定删除该用户?" @confirm="deleteUser(scope.row.id)">
              <template #reference>
                <el-button type="danger" size="small" :disabled="scope.row.id === currentUserId || (scope.row.role === 'admin' && adminCount <= 1)">删除</el-button>
              </template>
            </el-popconfirm>
          </div>
        </template>
      </el-table-column>
    </el-table>
    </div>

    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑用户 - 角色与权限' : '添加用户'"
      :width="isWide ? '720px' : '94vw'"
      :close-on-click-modal="false"
    >
      <el-form :model="form" :rules="rules" ref="formRef" label-width="90px">
        <el-form-item label="用户名" prop="username" v-if="!isEdit">
          <el-input v-model="form.username" placeholder="username" />
        </el-form-item>
        <el-form-item label="密码" prop="password" v-if="!isEdit">
          <el-input v-model="form.password" type="password" placeholder="password" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-select v-model="form.role" style="width: 100%">
            <el-option label="管理员（拥有全部功能权限）" value="admin" />
            <el-option label="普通用户（按下方勾选开放）" value="user" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="isEdit" label="用户ID">
          <el-tag size="small">#{{ form.id }}</el-tag>
          <span class="form-hint">创建于 {{ formatTime(form.created_at) }}</span>
        </el-form-item>

        <el-form-item label="面板权限">
          <div class="perm-toolbar">
            <el-button size="small" :disabled="form.role === 'admin'" @click="selectAllFeatures">全选</el-button>
            <el-button size="small" :disabled="form.role === 'admin'" @click="selectNoneFeatures">清空</el-button>
            <el-button size="small" :disabled="form.role === 'admin'" @click="selectDefaultFeatures">仅查看类</el-button>
            <span class="perm-tip">
              {{ form.role === 'admin' ? '管理员默认拥有全部权限，无需勾选' : `已选 ${form.permissions.length} / ${featureGroups.length} 组` }}
            </span>
          </div>

          <div v-if="form.role === 'admin'" class="perm-admin-box">
            <el-icon size="20" color="#f56c6c"><WarningFilled /></el-icon>
            <span>管理员自动拥有面板全部功能权限，权限矩阵对管理员不可编辑。</span>
          </div>

          <el-scrollbar v-else max-height="360px" class="perm-scroller">
            <div
              v-for="group in featureGroups"
              :key="group.name"
              class="perm-group"
            >
              <div class="perm-group-header">
                <span class="perm-group-name">{{ group.name }}</span>
                <el-button
                  link
                  size="small"
                  @click="toggleGroup(group.name)"
                >{{ isGroupAllSelected(group.name) ? '取消全组' : '勾选全组' }}</el-button>
              </div>
              <el-checkbox-group v-model="form.permissions" class="perm-checkbox-group">
                <el-checkbox
                  v-for="f in group.features"
                  :key="f.key"
                  :value="f.key"
                  class="perm-checkbox"
                >
                  <div class="perm-item">
                    <span class="perm-item-name">{{ f.name }}</span>
                    <span class="perm-item-key">{{ f.key }}</span>
                  </div>
                </el-checkbox>
              </el-checkbox-group>
            </div>
          </el-scrollbar>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveUser">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="resetDialogVisible" title="重置密码" :width="isWide ? '400px' : '94vw'">
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
import { ref, reactive, computed, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { WarningFilled } from '@element-plus/icons-vue'
import { userApi } from '../api'
import { useAuthStore } from '../store'

const auth = useAuthStore()
const users = ref<any[]>([])
const loading = ref(false)

// 窄屏移除操作列 sticky 固定，避免覆盖用户名列
const WIDE_MQ = '(min-width: 901px)'
const isWide = ref(typeof window !== 'undefined' ? window.matchMedia(WIDE_MQ).matches : true)
const mql = typeof window !== 'undefined' ? window.matchMedia(WIDE_MQ) : null
function onMqlChange(e: MediaQueryListEvent) { isWide.value = e.matches }
const dialogVisible = ref(false)
const resetDialogVisible = ref(false)
const isEdit = ref(false)
const currentUserId = computed(() => auth.userId)
const adminCount = computed(() => users.value.filter((u: any) => u.role === 'admin').length)
const currentResetId = ref(0)
const formRef = ref<any>(null)
const resetFormRef = ref<any>(null)

interface Feature { key: string; name: string; group: string }
const allFeatures = ref<Feature[]>([])

const form = reactive({
  id: 0,
  username: '',
  password: '',
  role: 'user',
  permissions: [] as string[],
  created_at: ''
})
const resetForm = reactive({ password: '' })

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }]
}

const resetRules = {
  password: [{ required: true, message: '请输入新密码', trigger: 'blur' }]
}

const DEFAULT_USER_PERMS = ['/dashboard', '/monitor', '/logs']

const featureGroups = computed(() => {
  const map = new Map<string, Feature[]>()
  for (const f of allFeatures.value) {
    if (!map.has(f.group)) map.set(f.group, [])
    map.get(f.group)!.push(f)
  }
  return Array.from(map.entries()).map(([name, features]) => ({ name, features }))
})

function formatTime(ts: string | number) {
  if (!ts) return '-'
  const d = new Date(ts as any)
  return d.toLocaleString()
}

function parsePerms(raw: any): string[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  if (typeof raw === 'string') {
    try {
      const arr = JSON.parse(raw)
      return Array.isArray(arr) ? arr : []
    } catch {
      return []
    }
  }
  return []
}

function featureName(key: string): string {
  const f = allFeatures.value.find(x => x.key === key)
  return f ? f.name : key
}

function isGroupAllSelected(groupName: string): boolean {
  const group = featureGroups.value.find(g => g.name === groupName)
  if (!group) return false
  return group.features.every(f => form.permissions.includes(f.key))
}

function toggleGroup(groupName: string) {
  if (form.role === 'admin') return
  const group = featureGroups.value.find(g => g.name === groupName)
  if (!group) return
  const allSelected = isGroupAllSelected(groupName)
  const groupKeys = group.features.map(f => f.key)
  if (allSelected) {
    form.permissions = form.permissions.filter(k => !groupKeys.includes(k))
  } else {
    const set = new Set(form.permissions)
    for (const k of groupKeys) set.add(k)
    form.permissions = Array.from(set)
  }
}

function selectAllFeatures() {
  if (form.role === 'admin') return
  form.permissions = allFeatures.value.map(f => f.key)
}
function selectNoneFeatures() {
  form.permissions = []
}
function selectDefaultFeatures() {
  form.permissions = [...DEFAULT_USER_PERMS]
}

async function loadFeatures() {
  try {
    const res: any = await userApi.listFeatures()
    allFeatures.value = res.data || []
  } catch (e: any) {
    ElMessage.error('加载功能列表失败')
  }
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
    Object.assign(form, {
      id: row.id,
      username: row.username,
      password: '',
      role: row.role,
      permissions: row.role === 'admin' ? [] : parsePerms(row.permissions),
      created_at: row.created_at
    })
  } else {
    Object.assign(form, {
      id: 0,
      username: '',
      password: '',
      role: 'user',
      permissions: [...DEFAULT_USER_PERMS],
      created_at: ''
    })
  }
  dialogVisible.value = true
}

function openResetDialog(row: any) {
  currentResetId.value = row.id
  resetForm.password = ''
  resetDialogVisible.value = true
}

watch(() => form.role, (val) => {
  if (val === 'admin') {
    form.permissions = []
  }
})

async function saveUser() {
  await formRef.value?.validate()
  try {
    if (isEdit.value) {
      const payload: any = { role: form.role }
      if (form.role !== 'admin') {
        payload.permissions = form.permissions
      }
      await userApi.update(form.id, payload)
      ElMessage.success('更新成功')
    } else {
      const payload: any = {
        username: form.username,
        password: form.password,
        role: form.role
      }
      if (form.role === 'admin') {
        payload.permissions = allFeatures.value.map(f => f.key)
      } else {
        payload.permissions = form.permissions
      }
      await userApi.create(payload)
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
  loadFeatures()
  loadUsers()
  if (mql) mql.addEventListener('change', onMqlChange)
})

onUnmounted(() => {
  if (mql) mql.removeEventListener('change', onMqlChange)
})
</script>

<style scoped>
.page-container {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}
.page-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}
.page-title {
  white-space: nowrap;
  flex-shrink: 0;
}
.table-wrap {
  width: 100%;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}
.row-actions {
  display: flex;
  flex-wrap: nowrap;
  gap: 6px;
  align-items: center;
}
.row-actions .el-button {
  margin-left: 0 !important;
}
.perm-toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
}
.perm-tip {
  margin-left: auto;
  color: var(--dim);
  font-size: 12px;
}
.perm-admin-box {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 16px 20px;
  background: rgba(245, 108, 108, 0.08);
  border: 1px dashed var(--red);
  border-radius: 8px;
  color: var(--red);
  font-size: 13px;
}
.perm-scroller {
  border: 1px solid var(--bdr);
  border-radius: 8px;
  padding: 12px;
  background: var(--bg);
}
.perm-group {
  margin-bottom: 12px;
}
.perm-group:last-child { margin-bottom: 0; }
.perm-group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 8px;
  background: var(--card);
  border-radius: 6px;
  margin-bottom: 8px;
}
.perm-group-name {
  font-weight: 600;
  font-size: 13px;
  color: var(--txt);
}
.perm-checkbox-group {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 4px 12px;
  padding: 4px 8px;
}
.perm-checkbox {
  margin-right: 0 !important;
}
.perm-item {
  display: flex;
  flex-direction: column;
  line-height: 1.3;
}
.perm-item-name {
  font-size: 13px;
  color: var(--txt);
}
.perm-item-key {
  font-size: 11px;
  color: var(--dim);
  font-family: monospace;
}
.perm-tags {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
}
.perm-tag {
  margin: 0;
}
.perm-admin {
  color: var(--red);
  font-weight: 600;
  font-size: 12px;
}
.perm-empty {
  color: var(--dim);
  font-size: 12px;
  font-style: italic;
}
.form-hint {
  margin-left: 12px;
  color: var(--dim);
  font-size: 12px;
}

/* ── 移动端 ── */
@media (max-width: 900px) {
  .page-container {
    padding: 0;
  }
  .page-header {
    flex-direction: row;
    flex-wrap: wrap;
    row-gap: 10px;
    margin-bottom: 12px;
  }
  .page-title {
    font-size: 17px;
    white-space: nowrap;
    flex-shrink: 0;
  }
  .table-wrap {
    width: 100%;
    max-width: 100%;
    overflow-x: auto;
  }
  .table-wrap :deep(.el-table) {
    min-width: 820px;
  }
  .row-actions {
    flex-wrap: nowrap;
    white-space: nowrap;
  }
  .perm-toolbar {
    flex-wrap: wrap;
    row-gap: 8px;
  }
  .perm-tip {
    margin-left: 0;
    flex: 1 1 100%;
  }
  .perm-checkbox-group {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 480px) {
  .page-title {
    font-size: 16px;
  }
  .perm-toolbar .el-button {
    padding: 6px 10px !important;
  }
}
</style>
