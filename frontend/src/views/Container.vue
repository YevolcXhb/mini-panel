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

    <el-dialog v-model="showCreate" title="创建容器" :width="isWide ? '520px' : '94vw'">
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

    <el-dialog v-model="showLogDialog" title="容器日志" :width="isWide ? '700px' : '94vw'">
      <div class="mono-block">{{ logs || '无日志' }}</div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { containerApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

// 窄屏用 94vw 弹窗，避免固定像素宽度溢出屏幕
const WIDE_MQ = '(min-width: 901px)'
const isWide = ref(typeof window !== 'undefined' ? window.matchMedia(WIDE_MQ).matches : true)
const mql = typeof window !== 'undefined' ? window.matchMedia(WIDE_MQ) : null
function onMqlChange(e: MediaQueryListEvent) { isWide.value = e.matches }

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

onMounted(() => {
  loadContainers()
  if (mql) mql.addEventListener('change', onMqlChange)
})
onUnmounted(() => {
  if (mql) mql.removeEventListener('change', onMqlChange)
})
</script>

<style scoped>
/* 容器日志块：dark-theme.css 未定义，需页面自带 */
.mono-block {
  background: #0d1117;
  color: #c9d1d9;
  border-radius: 8px;
  padding: 12px;
  max-height: 460px;
  overflow: auto;
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12.5px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

/* ── 移动端：卡片竖排，操作按钮铺成 2 列网格 ──
   窄屏 901px 起生效，覆盖 dark-theme.css 只到 768px 的规则 */
@media (max-width: 900px) {
  .page-title {
    font-size: 17px;
    white-space: nowrap;
  }
  .ct-grid {
    gap: 10px;
  }
  .ct-card {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
    padding: 12px;
  }
  .ct-info {
    min-width: 0;
    width: 100%;
  }
  .ct-name {
    font-size: 14px;
    word-break: break-all;
  }
  .ct-meta {
    font-size: 11.5px;
    word-break: break-all;
    line-height: 1.5;
  }
  .ct-acts {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    width: 100%;
  }
  .ct-acts .el-button {
    width: 100%;
    margin-left: 0 !important;
  }
  .empty-state {
    padding: 40px 16px;
    font-size: 13px;
  }
  .mono-block {
    max-height: 55dvh;
    font-size: 11.5px;
    padding: 10px;
  }
}

@media (max-width: 480px) {
  .page-title {
    font-size: 16px;
  }
  .ct-acts .el-button {
    padding: 7px 6px !important;
    font-size: 12px;
  }
}
</style>
