<template>
  <div>
    <h2 class="page-title">📁 文件管理</h2>

    <div class="file-toolbar">
      <div class="breadcrumb">
        <a @click="currentPath = '/'; loadFiles()">/</a>
        <span v-for="(part, i) in pathParts" :key="i">
          <span>/</span><a @click="navigateTo(i)">{{ part }}</a>
        </span>
      </div>
      <div class="file-actions">
        <el-button size="small" type="primary" @click="showCreate = true">+ 新建文件</el-button>
        <el-button size="small" @click="showCreateDir = true">新建目录</el-button>
        <el-upload :show-file-list="false" :http-request="handleUpload" style="display:inline-block;margin:0 6px">
          <el-button size="small">上传文件</el-button>
        </el-upload>
        <el-button size="small" @click="goBack">返回上级</el-button>
      </div>
    </div>

    <div class="table-wrap">
      <el-table :data="files" v-loading="loading" @row-click="handleRowClick" size="small">
        <el-table-column label="名称" min-width="200">
          <template #default="{ row }">
            <el-icon v-if="row.is_dir" size="16" color="#fbbf24"><Folder /></el-icon>
            <el-icon v-else size="16" color="#888"><Document /></el-icon>
            <span style="margin-left:8px">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120">
          <template #default="{ row }">{{ row.is_dir ? '-' : formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column prop="mode" label="权限" width="100" />
        <el-table-column label="修改时间" width="170">
          <template #default="{ row }">{{ new Date(row.mod_time * 1000).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" v-if="!row.is_dir" @click.stop="editFile(row)">编辑</el-button>
            <el-button size="small" type="danger" @click.stop="deleteFile(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="showEditor" title="文件编辑" width="800px" class="editor-dialog">
      <el-input v-model="fileContent" type="textarea" :rows="22" class="editor-textarea" />
      <template #footer>
        <el-button @click="showEditor = false">取消</el-button>
        <el-button type="primary" @click="saveFile">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showCreate" title="新建文件" width="400px">
      <el-input v-model="newFileName" placeholder="文件名" />
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createFile(false)">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showCreateDir" title="新建目录" width="400px">
      <el-input v-model="newDirName" placeholder="目录名" />
      <template #footer>
        <el-button @click="showCreateDir = false">取消</el-button>
        <el-button type="primary" @click="createFile(true)">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { fileApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const currentPath = ref('/')
const files = ref<any[]>([])
const loading = ref(false)
const showEditor = ref(false)
const showCreate = ref(false)
const showCreateDir = ref(false)
const fileContent = ref('')
const editingFile = ref('')
const newFileName = ref('')
const newDirName = ref('')

const pathParts = computed(() => currentPath.value.split('/').filter(Boolean))

function formatSize(size: number) {
  if (size < 1024) return size + ' B'
  if (size < 1024 * 1024) return (size / 1024).toFixed(1) + ' KB'
  if (size < 1024 * 1024 * 1024) return (size / 1024 / 1024).toFixed(1) + ' MB'
  return (size / 1024 / 1024 / 1024).toFixed(1) + ' GB'
}

async function loadFiles() {
  loading.value = true
  try {
    const res: any = await fileApi.list(currentPath.value)
    files.value = res.data || []
  } catch (e) {} finally { loading.value = false }
}

function handleRowClick(row: any) {
  if (row.is_dir) { currentPath.value = row.path; loadFiles() }
}
function goBack() {
  const parts = currentPath.value.split('/').filter(Boolean)
  parts.pop()
  currentPath.value = '/' + parts.join('/')
  loadFiles()
}
function navigateTo(index: number) {
  const parts = pathParts.value.slice(0, index + 1)
  currentPath.value = '/' + parts.join('/')
  loadFiles()
}

async function editFile(row: any) {
  editingFile.value = row.path
  try {
    const res: any = await fileApi.getContent(row.path)
    fileContent.value = res.data
    showEditor.value = true
  } catch (e) {}
}

async function saveFile() {
  try {
    await fileApi.update({ path: editingFile.value, content: fileContent.value })
    ElMessage.success('保存成功')
    showEditor.value = false
  } catch (e) {}
}

async function createFile(isDir: boolean) {
  const name = isDir ? newDirName.value : newFileName.value
  const path = currentPath.value === '/' ? '/' + name : currentPath.value + '/' + name
  try {
    await fileApi.create({ path, is_dir: isDir })
    ElMessage.success('创建成功')
    showCreate.value = false; showCreateDir.value = false
    newFileName.value = ''; newDirName.value = ''
    loadFiles()
  } catch (e) {}
}

async function deleteFile(row: any) {
  try {
    await ElMessageBox.confirm(`确定删除 ${row.name} 吗？`, '确认删除', { confirmButtonClass: 'el-button--danger' })
    await fileApi.delete(row.path)
    ElMessage.success('删除成功')
    loadFiles()
  } catch (e) {}
}

async function handleUpload(options: any) {
  const form = new FormData()
  form.append('file', options.file)
  form.append('path', currentPath.value)
  try {
    await fileApi.upload(form)
    ElMessage.success('上传成功')
    loadFiles()
  } catch (e) {}
}

onMounted(loadFiles)
</script>

<style scoped>
.file-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
  flex-wrap: wrap;
  gap: 8px;
}
.file-actions { display: flex; gap: 6px; flex-wrap: wrap; }
.breadcrumb {
  display: flex;
  gap: 2px;
  font-size: 13px;
  flex-wrap: wrap;
  align-items: center;
  padding: 6px 0;
}
.breadcrumb a {
  color: var(--acc);
  text-decoration: none;
  padding: 2px 6px;
  border-radius: 4px;
  cursor: pointer;
}
.breadcrumb a:hover { background: rgba(79, 140, 255, 0.1); }
.breadcrumb span { color: var(--dim); }
</style>
