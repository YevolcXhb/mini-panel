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
        <el-upload :show-file-list="false" :http-request="handleUpload" :multiple="true" style="display:inline-block">
          <el-button size="small">上传</el-button>
        </el-upload>
        <el-button size="small" type="primary" @click="showCreate = true">+ 新建文件</el-button>
        <el-button size="small" @click="showCreateDir = true">新建目录</el-button>
        <el-button size="small" @click="goBack">返回上级</el-button>
        <el-button size="small" @click="showRecycleBin = true; loadRecycleBin()">🗑 回收站</el-button>
        <el-input v-model="searchQuery" placeholder="搜索文件名..." size="small" style="width:180px;margin-left:8px" clearable @input="onSearch" @clear="loadFiles" />
      </div>
    </div>

    <div v-if="selectedFiles.length > 0" class="batch-bar">
      <span>已选 {{ selectedFiles.length }} 项</span>
      <el-button size="small" @click="batchCopy">复制</el-button>
      <el-button size="small" @click="batchCut">剪切</el-button>
      <el-button size="small" @click="openCompressDialog">压缩</el-button>
      <el-button size="small" type="danger" @click="batchDelete">删除</el-button>
      <el-button size="small" @click="clearSelection">取消选择</el-button>
    </div>

    <div v-if="clipboard.length > 0" class="batch-bar" style="background:#fff3e0">
      <span>{{ clipboardMode === 'copy' ? '已复制' : '已剪切' }} {{ clipboard.length }} 项</span>
      <el-button size="small" type="primary" @click="pasteFiles">粘贴到当前目录</el-button>
      <el-button size="small" @click="clipboard = []; clipboardMode = 'copy'">取消</el-button>
    </div>

    <div class="table-wrap">
      <el-table :data="files" v-loading="loading" size="small" @selection-change="onSelectionChange" ref="tableRef">
        <el-table-column type="selection" width="40" />
        <el-table-column label="名称" min-width="200">
          <template #default="{ row }">
            <el-icon v-if="row.is_dir" size="16" color="#fbbf24"><Folder /></el-icon>
            <el-icon v-else size="16" color="#888"><Document /></el-icon>
            <span style="margin-left:8px;cursor:pointer" @click="handleRowClick(row)">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="size" label="大小" width="120">
          <template #default="{ row }">{{ row.is_dir ? '-' : formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column prop="mode" label="权限" width="110" />
        <el-table-column label="修改时间" width="170">
          <template #default="{ row }">{{ new Date(row.mod_time * 1000).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column label="操作" width="380" fixed="right">
          <template #default="{ row }">
            <el-button size="small" v-if="!row.is_dir" @click.stop="previewFile(row)">预览</el-button>
            <el-button size="small" v-if="!row.is_dir" @click.stop="editFile(row)">编辑</el-button>
            <el-button size="small" v-if="!row.is_dir" @click.stop="downloadFile(row)">下载</el-button>
            <el-button size="small" v-if="row.is_dir" @click.stop="downloadZip(row)">下载ZIP</el-button>
            <el-button size="small" v-if="isArchive(row.name)" @click.stop="extractArchive(row)">解压</el-button>
            <el-button size="small" @click.stop="renameFile(row)">重命名</el-button>
            <el-button size="small" @click.stop="openChmod(row)">权限</el-button>
            <el-button size="small" type="danger" @click.stop="deleteFile(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 文件编辑 -->
    <el-dialog v-model="showEditor" title="文件编辑" width="800px" class="editor-dialog">
      <el-input v-model="fileContent" type="textarea" :rows="22" class="editor-textarea" />
      <template #footer>
        <el-button @click="showEditor = false">取消</el-button>
        <el-button type="primary" @click="saveFile">保存</el-button>
      </template>
    </el-dialog>

    <!-- 新建文件 -->
    <el-dialog v-model="showCreate" title="新建文件" width="400px">
      <el-input v-model="newFileName" placeholder="文件名" />
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createFile(false)">创建</el-button>
      </template>
    </el-dialog>

    <!-- 新建目录 -->
    <el-dialog v-model="showCreateDir" title="新建目录" width="400px">
      <el-input v-model="newDirName" placeholder="目录名" />
      <template #footer>
        <el-button @click="showCreateDir = false">取消</el-button>
        <el-button type="primary" @click="createFile(true)">创建</el-button>
      </template>
    </el-dialog>

    <!-- 文件预览 -->
    <el-dialog v-model="showPreview" :title="'预览 - ' + previewFileName" width="800px">
      <pre style="max-height: 500px; overflow: auto; background: #1a1a2e; color: #e0e0e0; padding: 16px; border-radius: 6px; font-size: 13px; white-space: pre-wrap; word-break: break-all">{{ previewContent }}</pre>
    </el-dialog>

    <!-- 重命名 -->
    <el-dialog v-model="showRename" title="重命名" width="400px">
      <el-input v-model="renameNewName" :placeholder="'原名称: ' + renameOldName" />
      <template #footer>
        <el-button @click="showRename = false">取消</el-button>
        <el-button type="primary" @click="doRename">确定</el-button>
      </template>
    </el-dialog>

    <!-- 权限修改 -->
    <el-dialog v-model="showChmod" title="修改权限" width="400px">
      <div style="margin-bottom: 12px; font-size: 13px; color: #888">文件: {{ chmodFileName }}</div>
      <el-input v-model="chmodMode" placeholder="644 (文件) / 755 (目录)" style="margin-bottom: 12px" />
      <el-checkbox v-model="chmodRecursive">应用到子目录</el-checkbox>
      <template #footer>
        <el-button @click="showChmod = false">取消</el-button>
        <el-button type="primary" @click="doChmod">确定</el-button>
      </template>
    </el-dialog>

    <!-- 压缩 -->
    <el-dialog v-model="showCompress" title="压缩文件" width="400px">
      <div style="margin-bottom: 8px; font-size: 13px; color: #888">已选 {{ compressPaths.length }} 项</div>
      <el-input v-model="compressOutput" placeholder="压缩包名称" style="margin-bottom: 12px" />
      <el-radio-group v-model="compressFormat" style="margin-bottom: 12px">
        <el-radio value="zip">ZIP</el-radio>
        <el-radio value="tar.gz">TAR.GZ</el-radio>
      </el-radio-group>
      <template #footer>
        <el-button @click="showCompress = false">取消</el-button>
        <el-button type="primary" @click="doCompress">压缩</el-button>
      </template>
    </el-dialog>

    <!-- 回收站 -->
    <el-dialog v-model="showRecycleBin" title="回收站" width="700px">
      <div style="margin-bottom: 12px; display: flex; gap: 8px">
        <el-button size="small" type="danger" @click="clearRecycleBin">清空回收站</el-button>
        <el-button size="small" @click="loadRecycleBin">刷新</el-button>
      </div>
      <el-table :data="recycleItems" v-loading="recycleLoading" size="small">
        <el-table-column prop="name" label="名称" min-width="200" />
        <el-table-column label="大小" width="100">
          <template #default="{ row }">{{ row.is_dir ? '-' : formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column label="删除时间" width="170">
          <template #default="{ row }">{{ new Date(row.mod_time * 1000).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column label="操作" width="160">
          <template #default="{ row }">
            <el-button size="small" @click="restoreRecycle(row)">恢复</el-button>
            <el-button size="small" type="danger" @click="permanentDelete(row)">彻底删除</el-button>
          </template>
        </el-table-column>
      </el-table>
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
const showPreview = ref(false)
const showRename = ref(false)
const showChmod = ref(false)
const showCompress = ref(false)
const showRecycleBin = ref(false)
const fileContent = ref('')
const editingFile = ref('')
const newFileName = ref('')
const newDirName = ref('')
const previewFileName = ref('')
const previewContent = ref('')
const searchQuery = ref('')
const tableRef = ref<any>(null)

// 多选
const selectedFiles = ref<any[]>([])

// 重命名
const renameOldPath = ref('')
const renameOldName = ref('')
const renameNewName = ref('')

// 权限
const chmodPath = ref('')
const chmodFileName = ref('')
const chmodMode = ref('')
const chmodRecursive = ref(false)

// 压缩
const compressPaths = ref<string[]>([])
const compressOutput = ref('')
const compressFormat = ref('zip')

// 剪贴板
const clipboard = ref<any[]>([])
const clipboardMode = ref<'copy' | 'cut'>('copy')

// 回收站
const recycleItems = ref<any[]>([])
const recycleLoading = ref(false)

const pathParts = computed(() => currentPath.value.split('/').filter(Boolean))

function formatSize(size: number) {
  if (size < 1024) return size + ' B'
  if (size < 1024 * 1024) return (size / 1024).toFixed(1) + ' KB'
  if (size < 1024 * 1024 * 1024) return (size / 1024 / 1024).toFixed(1) + ' MB'
  return (size / 1024 / 1024 / 1024).toFixed(1) + ' GB'
}

function isArchive(name: string) {
  return /\.(zip|tar\.gz|tgz|tar\.bz2|tar\.xz)$/i.test(name)
}

async function loadFiles() {
  loading.value = true
  try {
    const res: any = searchQuery.value
      ? await fileApi.search(currentPath.value, searchQuery.value)
      : await fileApi.list(currentPath.value)
    files.value = res.data || []
  } catch (e) {} finally { loading.value = false }
}

function onSearch() {
  loadFiles()
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

function onSelectionChange(rows: any[]) {
  selectedFiles.value = rows
}
function clearSelection() {
  tableRef.value?.clearSelection()
}

// 编辑文件
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

// 删除 (移入回收站)
async function deleteFile(row: any) {
  try {
    await ElMessageBox.confirm(`确定删除 ${row.name} 吗？将移入回收站`, '确认删除', { confirmButtonClass: 'el-button--danger' })
    await fileApi.delete(row.path)
    ElMessage.success('已移入回收站')
    loadFiles()
  } catch (e) {}
}
async function batchDelete() {
  try {
    await ElMessageBox.confirm(`确定删除 ${selectedFiles.value.length} 个文件吗？将移入回收站`, '批量删除', { confirmButtonClass: 'el-button--danger' })
    for (const f of selectedFiles.value) {
      await fileApi.delete(f.path)
    }
    ElMessage.success('已移入回收站')
    clearSelection()
    loadFiles()
  } catch (e) {}
}

// 下载
async function downloadFile(row: any) {
  try {
    const res = await fileApi.download(row.path)
    const blob = new Blob([res.data])
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = row.name
    document.body.appendChild(a); a.click(); document.body.removeChild(a)
    window.URL.revokeObjectURL(url)
  } catch (e) { ElMessage.error('下载失败') }
}
async function downloadZip(row: any) {
  try {
    const res = await fileApi.downloadZip(row.path)
    const blob = new Blob([res.data])
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = row.name + '.zip'
    document.body.appendChild(a); a.click(); document.body.removeChild(a)
    window.URL.revokeObjectURL(url)
    ElMessage.success('下载开始')
  } catch (e: any) { ElMessage.error(e?.message || '下载失败') }
}

// 上传
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

// 预览
async function previewFile(row: any) {
  try {
    const res: any = await fileApi.getContent(row.path)
    previewFileName.value = row.name
    previewContent.value = res.data || ''
    showPreview.value = true
  } catch (e: any) { ElMessage.error(e?.message || '预览失败') }
}

// 重命名
function renameFile(row: any) {
  renameOldPath.value = row.path
  renameOldName.value = row.name
  renameNewName.value = row.name
  showRename.value = true
}
async function doRename() {
  if (!renameNewName.value.trim()) {
    ElMessage.warning('请输入新名称')
    return
  }
  try {
    await fileApi.rename(renameOldPath.value, renameNewName.value.trim())
    ElMessage.success('重命名成功')
    showRename.value = false
    loadFiles()
  } catch (e: any) { ElMessage.error(e?.message || '重命名失败') }
}

// 权限修改
function openChmod(row: any) {
  chmodPath.value = row.path
  chmodFileName.value = row.name
  chmodMode.value = row.is_dir ? '755' : '644'
  chmodRecursive.value = false
  showChmod.value = true
}
async function doChmod() {
  if (!/^\d{3}$/.test(chmodMode.value)) {
    ElMessage.warning('请输入 3 位数字权限 (如 644, 755)')
    return
  }
  try {
    await fileApi.chmod(chmodPath.value, chmodMode.value, chmodRecursive.value)
    ElMessage.success('权限修改成功')
    showChmod.value = false
    loadFiles()
  } catch (e: any) { ElMessage.error(e?.message || '修改失败') }
}

// 压缩
function openCompressDialog() {
  compressPaths.value = selectedFiles.value.map(f => f.path)
  compressOutput.value = selectedFiles.value[0]?.name + '.zip' || 'archive.zip'
  compressFormat.value = 'zip'
  showCompress.value = true
}
async function doCompress() {
  if (!compressOutput.value.trim()) {
    ElMessage.warning('请输入压缩包名称')
    return
  }
  const output = currentPath.value === '/' ? '/' + compressOutput.value : currentPath.value + '/' + compressOutput.value
  try {
    await fileApi.compress(compressPaths.value, output, compressFormat.value)
    ElMessage.success('压缩完成')
    showCompress.value = false
    clearSelection()
    loadFiles()
  } catch (e: any) { ElMessage.error(e?.message || '压缩失败') }
}

// 解压
async function extractArchive(row: any) {
  try {
    const destDir = currentPath.value
    await fileApi.extract(row.path, destDir)
    ElMessage.success('解压完成')
    loadFiles()
  } catch (e: any) { ElMessage.error(e?.message || '解压失败') }
}

// 复制/剪切/粘贴
function batchCopy() {
  clipboard.value = [...selectedFiles.value]
  clipboardMode.value = 'copy'
  ElMessage.success(`已复制 ${clipboard.value.length} 项`)
}
function batchCut() {
  clipboard.value = [...selectedFiles.value]
  clipboardMode.value = 'cut'
  ElMessage.success(`已剪切 ${clipboard.value.length} 项`)
}
async function pasteFiles() {
  let success = 0
  for (const f of clipboard.value) {
    const destPath = currentPath.value === '/' ? '/' + f.name : currentPath.value + '/' + f.name
    try {
      if (clipboardMode.value === 'copy') {
        await fileApi.copy(f.path, destPath)
      } else {
        await fileApi.move(f.path, destPath)
      }
      success++
    } catch (e) {}
  }
  ElMessage.success(`成功 ${clipboardMode.value === 'copy' ? '复制' : '移动'} ${success}/${clipboard.value.length} 项`)
  if (clipboardMode.value === 'cut') clipboard.value = []
  loadFiles()
}

// 回收站
async function loadRecycleBin() {
  recycleLoading.value = true
  try {
    const res: any = await fileApi.listRecycleBin()
    recycleItems.value = res.data || []
  } catch (e) {} finally { recycleLoading.value = false }
}
async function restoreRecycle(row: any) {
  try {
    await fileApi.restoreRecycle(row.path)
    ElMessage.success('已恢复')
    loadRecycleBin()
  } catch (e: any) { ElMessage.error(e?.message || '恢复失败') }
}
async function permanentDelete(row: any) {
  try {
    await ElMessageBox.confirm('确定彻底删除吗？此操作不可恢复！', '彻底删除', { confirmButtonClass: 'el-button--danger' })
	    await fileApi.forceDelete(row.path)
    ElMessage.success('已彻底删除')
    loadRecycleBin()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '删除失败')
  }
}
async function clearRecycleBin() {
  try {
    await ElMessageBox.confirm('确定清空回收站吗？所有文件将被彻底删除！', '清空回收站', { confirmButtonClass: 'el-button--danger' })
    await fileApi.clearRecycleBin()
    ElMessage.success('回收站已清空')
    loadRecycleBin()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '清空失败')
  }
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
.file-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
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
.batch-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 12px;
  background: rgba(79, 140, 255, 0.08);
  border-radius: 6px;
  font-size: 13px;
}
</style>
