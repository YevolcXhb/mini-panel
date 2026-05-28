<template>
  <div>
    <el-card>
      <template #header>
        <div class="file-header">
          <el-breadcrumb separator="/">
            <el-breadcrumb-item v-for="(part, i) in pathParts" :key="i" @click="navigateTo(i)">
              <a>{{ part || '/' }}</a>
            </el-breadcrumb-item>
          </el-breadcrumb>
          <div class="file-actions">
            <el-button size="small" @click="showCreate = true" type="primary">新建文件</el-button>
            <el-button size="small" @click="showCreateDir = true">新建目录</el-button>
            <el-upload :show-file-list="false" :http-request="handleUpload" style="display: inline-block; margin: 0 10px">
              <el-button size="small" type="success">上传文件</el-button>
            </el-upload>
            <el-button size="small" @click="goBack">返回上级</el-button>
          </div>
        </div>
      </template>

      <el-table :data="files" v-loading="loading" @row-click="handleRowClick">
        <el-table-column label="名称">
          <template #default="{ row }">
            <el-icon v-if="row.is_dir" size="16"><Folder /></el-icon>
            <el-icon v-else size="16"><Document /></el-icon>
            <span style="margin-left: 8px">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120">
          <template #default="{ row }">
            {{ row.is_dir ? '-' : formatSize(row.size) }}
          </template>
        </el-table-column>
        <el-table-column prop="mode" label="权限" width="100" />
        <el-table-column label="修改时间" width="180">
          <template #default="{ row }">
            {{ new Date(row.mod_time * 1000).toLocaleString() }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" v-if="!row.is_dir" @click.stop="editFile(row)">编辑</el-button>
            <el-button size="small" type="danger" @click.stop="deleteFile(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="showEditor" title="文件编辑" width="800px">
      <el-input v-model="fileContent" type="textarea" :rows="20" />
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

const pathParts = computed(() => {
  return currentPath.value.split('/').filter(Boolean)
})

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
  } catch (e) {
  } finally {
    loading.value = false
  }
}

function handleRowClick(row: any) {
  if (row.is_dir) {
    currentPath.value = row.path
    loadFiles()
  }
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

async function deleteFile(row: any) {
  try {
    await ElMessageBox.confirm(`确定要删除 ${row.name} 吗？`, '确认删除')
    await fileApi.delete({ path: row.path })
    ElMessage.success('删除成功')
    loadFiles()
  } catch (e) {}
}

async function createFile(isDir: boolean) {
  const name = isDir ? newDirName.value : newFileName.value
  if (!name) return
  const path = currentPath.value === '/' ? '/' + name : currentPath.value + '/' + name
  try {
    await fileApi.create({ path, is_dir: isDir, content: '' })
    ElMessage.success('创建成功')
    showCreate.value = false
    showCreateDir.value = false
    newFileName.value = ''
    newDirName.value = ''
    loadFiles()
  } catch (e) {}
}

async function handleUpload(options: any) {
  try {
    await fileApi.upload(currentPath.value, options.file)
    ElMessage.success('上传成功')
    loadFiles()
  } catch (e) {}
}

onMounted(loadFiles)
</script>

<style scoped>
.file-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.file-actions {
  display: flex;
  gap: 5px;
}
</style>
