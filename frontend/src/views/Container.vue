<template>
  <div>
    <el-card>
      <template #header>
        <div class="container-header">
          <span>容器管理</span>
          <el-button type="primary" size="small" @click="showCreate = true">创建容器</el-button>
        </div>
      </template>
      <el-table :data="containers" v-loading="loading" size="small">
        <el-table-column prop="name" label="名称" sortable />
        <el-table-column prop="image" label="镜像" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'running' ? 'success' : 'info'">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="PID" width="100">
          <template #default="{ row }">
            {{ row.pids?.join(', ') || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="250">
          <template #default="{ row }">
            <el-button size="small" v-if="row.status !== 'running'" @click="startContainer(row.name)">启动</el-button>
            <el-button size="small" v-if="row.status === 'running'" @click="stopContainer(row.name)">停止</el-button>
            <el-button size="small" @click="showLogs(row.name)">日志</el-button>
            <el-button size="small" type="danger" @click="removeContainer(row.name)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="showCreate" title="创建容器" width="500px">
      <el-form :model="createForm" label-width="100px">
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

    <el-dialog v-model="showLogDialog" title="容器日志" width="800px">
      <el-input v-model="logs" type="textarea" :rows="20" readonly />
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

const createForm = ref({
  name: '',
  image: '',
  envStr: '',
  volumeStr: '',
  detach: true
})

async function loadContainers() {
  loading.value = true
  try {
    const res: any = await containerApi.list()
    containers.value = res.data || []
  } catch (e) {
  } finally {
    loading.value = false
  }
}

async function createContainer() {
  const env = createForm.value.envStr.split('\n').filter(Boolean)
  const volumes = createForm.value.volumeStr.split('\n').filter(Boolean)
  try {
    await containerApi.create({
      name: createForm.value.name,
      image: createForm.value.image,
      env,
      volumes,
      detach: createForm.value.detach
    })
    ElMessage.success('创建成功')
    showCreate.value = false
    loadContainers()
  } catch (e) {}
}

async function startContainer(name: string) {
  try {
    await containerApi.start(name)
    ElMessage.success('启动成功')
    loadContainers()
  } catch (e) {}
}

async function stopContainer(name: string) {
  try {
    await containerApi.stop(name)
    ElMessage.success('停止成功')
    loadContainers()
  } catch (e) {}
}

async function removeContainer(name: string) {
  try {
    await ElMessageBox.confirm(`确定要删除容器 ${name} 吗？`, '确认删除')
    await containerApi.remove(name)
    ElMessage.success('删除成功')
    loadContainers()
  } catch (e) {}
}

async function showLogs(name: string) {
  try {
    const res: any = await containerApi.logs(name, 100)
    logs.value = res.data
    showLogDialog.value = true
  } catch (e) {}
}

onMounted(loadContainers)
</script>

<style scoped>
.container-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
