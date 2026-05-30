<template>
  <div>
    <h2 class="page-title">📦 容器管理</h2>

    <div style="margin-bottom: 16px">
      <el-button type="primary" @click="showCreate = true">+ 创建容器</el-button>
      <el-button @click="loadContainers">🔄 刷新</el-button>
    </div>

    <div class="ct-grid" v-if="containers.length">
      <div class="ct-card" v-for="c in containers" :key="c.name">
        <div class="ct-info">
          <div class="ct-name">
            📦 {{ c.name }}
            <span :class="c.status === 'running' ? 'tag-on' : 'tag-off'">{{ c.status === 'running' ? '运行中' : '已停止' }}</span>
          </div>
          <div class="ct-meta">{{ c.image }} · PID: {{ c.pids?.join(', ') || '-' }}</div>
        </div>
        <div class="ct-acts">
          <el-button size="small" v-if="c.status !== 'running'" type="primary" @click="startContainer(c.name)">▶ 启动</el-button>
          <el-button size="small" v-if="c.status === 'running'" @click="stopContainer(c.name)">⏹ 停止</el-button>
          <el-button size="small" @click="showLogs(c.name)">📋 日志</el-button>
          <el-button size="small" type="danger" @click="removeContainer(c.name)">🗑 删除</el-button>
        </div>
      </div>
    </div>
    <div v-else class="empty-state">暂无容器，点击上方按钮创建</div>

    <el-dialog v-model="showCreate" title="创建容器" width="520px">
      <el-form :model="createForm" label-width="90px">
        <el-form-item label="名称">
          <el-input v-model="createForm.name" placeholder="容器名称" />
        </el-form-item>
        <el-form-item label="镜像">
          <el-input v-model="createForm.image" placeholder="如: docker.io/library/alpine:latest" />
        </el-form-item>
        <el-form-item label="环境变量">
          <el-input v-model="createForm.envStr" type="textarea" :rows="3" placeholder="每行一个 KEY=VALUE" />
        </el-form-item>
        <el-form-item label="挂载卷">
          <el-input v-model="createForm.volumeStr" type="textarea" :rows="3" placeholder="每行一个 /host:/container" />
        </el-form-item>
        <el-form-item label="后台运行">
          <el-switch v-model="createForm.detach" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="createContainer">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showLogDialog" title="容器日志" width="700px">
      <div class="mono-block">{{ logs || '无日志' }}</div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { containerApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const containers = ref<any[]>([])
const loading = ref(false)
const showCreate = ref(false)
const showLogDialog = ref(false)
const logs = ref('')

const createForm = ref({ name: '', image: '', envStr: '', volumeStr: '', detach: true })

async function loadContainers() {
  loading.value = true
  try {
    const res: any = await containerApi.list()
    containers.value = res.data || []
  } catch (e) {} finally { loading.value = false }
}

async function createContainer() {
  const env = createForm.value.envStr.split('\n').filter(Boolean)
  const volumes = createForm.value.volumeStr.split('\n').filter(Boolean)
  try {
    await containerApi.create({ name: createForm.value.name, image: createForm.value.image, env, volumes, detach: createForm.value.detach })
    ElMessage.success('创建成功')
    showCreate.value = false
    loadContainers()
  } catch (e) {}
}

async function startContainer(name: string) {
  try { await containerApi.start(name); ElMessage.success('启动成功'); loadContainers() } catch (e) {}
}
async function stopContainer(name: string) {
  try { await containerApi.stop(name); ElMessage.success('停止成功'); loadContainers() } catch (e) {}
}
async function removeContainer(name: string) {
  try {
    await ElMessageBox.confirm(`确定要删除容器 ${name} 吗？`, '确认删除', { confirmButtonClass: 'el-button--danger' })
    await containerApi.remove(name); ElMessage.success('删除成功'); loadContainers()
  } catch (e) {}
}
async function showLogs(name: string) {
  try {
    const res: any = await containerApi.logs(name, 100)
    logs.value = res.data || '无日志'
    showLogDialog.value = true
  } catch (e) {}
}

onMounted(loadContainers)
</script>
