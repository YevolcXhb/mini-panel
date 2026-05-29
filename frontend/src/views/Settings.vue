<template>
  <div>
    <h2 class="page-title">🔧 系统设置</h2>

    <div class="info-grid" style="margin-bottom:24px">
      <div class="info-card">
        <h3 style="margin:0 0 16px 0;font-size:15px">外观设置</h3>
        <el-form :model="settings" label-width="100px">
          <el-form-item label="主题">
            <el-radio-group v-model="settings.theme">
              <el-radio label="dark">深色</el-radio>
              <el-radio label="light">浅色</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="语言">
            <el-select v-model="settings.language">
              <el-option label="简体中文" value="zh" />
              <el-option label="English" value="en" />
            </el-select>
          </el-form-item>
          <el-form-item label="时区"><el-input v-model="settings.timezone" /></el-form-item>
        </el-form>
      </div>

      <div class="info-card">
        <h3 style="margin:0 0 16px 0;font-size:15px">安全设置</h3>
        <el-form :model="settings" label-width="100px">
          <el-form-item label="安全入口">
            <el-input v-model="settings.SecurityEntrance" placeholder="留空则禁用安全入口" />
            <div style="font-size:12px;color:var(--dim);margin-top:4px">
              设置后需通过 http://面板地址:端口/[安全入口] 访问
            </div>
          </el-form-item>
        </el-form>
      </div>

      <div class="info-card">
        <h3 style="margin:0 0 16px 0;font-size:15px">修改密码</h3>
        <el-form :model="pwdForm" label-width="100px">
          <el-form-item label="当前密码">
            <el-input type="password" v-model="pwdForm.old_password" placeholder="当前密码" />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input type="password" v-model="pwdForm.new_password" placeholder="至少6位" />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="changePassword">修改密码</el-button>
          </el-form-item>
        </el-form>
      </div>

      <div class="info-card">
        <h3 style="margin:0 0 16px 0;font-size:15px">容器设置</h3>
        <el-form :model="settings" label-width="100px">
          <el-form-item label="容器模式">
            <el-tag type="info">DockRoot</el-tag>
          </el-form-item>
          <el-form-item label="文件管理根目录">
            <el-input v-model="settings.file_manager_root" placeholder="/" />
          </el-form-item>
        </el-form>
      </div>

      <div class="info-card">
        <h3 style="margin:0 0 16px 0;font-size:15px">负载计算</h3>
        <el-form :model="settings" label-width="100px">
          <el-form-item label="计算模式">
            <el-radio-group v-model="settings.load_host_mode">
              <el-radio label="chroot">仅 Chroot</el-radio>
              <el-radio label="host">包含宿主机</el-radio>
            </el-radio-group>
            <div style="font-size:12px;color:var(--dim);margin-top:4px">
              仅 Chroot：只计算 chroot 容器内的进程负载<br/>
              包含宿主机：同时计算宿主机和 chroot 的进程负载
            </div>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <div style="display:flex;gap:12px;margin-bottom:24px">
      <el-button type="primary" @click="saveSettings">💾 保存设置</el-button>
      <el-button @click="resetSettings">🔄 恢复默认</el-button>
    </div>

    <h3 class="section-title">系统信息</h3>
    <div class="info-grid" style="margin-bottom:24px">
      <div class="info-card">
        <div class="info-label">版本</div>
        <div class="info-value">{{ versionInfo.version || '-' }}</div>
      </div>
      <div class="info-card">
        <div class="info-label">前端框架</div>
        <div class="info-value">Vue 3 + Element Plus</div>
      </div>
      <div class="info-card">
        <div class="info-label">容器支持</div>
        <div class="info-value">DockRoot</div>
      </div>
      <div class="info-card">
        <div class="info-label">数据库</div>
        <div class="info-value">SQLite</div>
      </div>
      <div class="info-card">
        <div class="info-label">后端语言</div>
        <div class="info-value">Go 1.23+</div>
      </div>
      <div class="info-card">
        <div class="info-label">构建工具</div>
        <div class="info-value">Vite</div>
      </div>
    </div>

    <h3 class="section-title">危险操作</h3>
    <div class="info-card" style="max-width:600px">
      <el-button type="danger" @click="clearData">🗑 清除所有数据</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { settingApi, authApiExt, versionApi } from '../api'
import { useAuthStore } from '../store'
import { ElMessage, ElMessageBox } from 'element-plus'

const settings = ref<any>({})
const pwdForm = ref({ old_password: '', new_password: '' })
const versionInfo = ref({ version: '-', build_time: '-', git_commit: '-' })

async function loadSettings() {
  try { const res: any = await settingApi.get(); settings.value = res.data || {} } catch (e) {}
}

async function loadVersion() {
  try { const res: any = await versionApi.get(); versionInfo.value = res.data || {} } catch (e) {}
}

async function saveSettings() {
  try {
    const toSave: any = {}
    for (const key of ['theme', 'language', 'timezone', 'container_mode', 'file_manager_root', 'SecurityEntrance', 'load_host_mode']) {
      if (settings.value[key] !== undefined) {
        toSave[key] = settings.value[key]
      }
    }
    await settingApi.update(toSave)
    ElMessage.success('保存成功')
  } catch (e) {}
}

const auth = useAuthStore()
const router = useRouter()

async function changePassword() {
  if (!pwdForm.value.old_password || !pwdForm.value.new_password) {
    ElMessage.warning('请填写完整')
    return
  }
  if (pwdForm.value.new_password.length < 6) {
    ElMessage.warning('新密码至少6位')
    return
  }
  try {
    await authApiExt.changePassword(pwdForm.value)
    ElMessage.success('密码修改成功，请重新登录')
    pwdForm.value = { old_password: '', new_password: '' }
    auth.clearAuth()
    router.push('/login')
  } catch (e: any) {
    ElMessage.error(e?.message || '修改失败')
  }
}

async function resetSettings() {
  try {
    await ElMessageBox.confirm('确定要恢复默认设置吗？', '确认')
    await settingApi.reset()
    ElMessage.success('已恢复默认设置')
    loadSettings()
  } catch (e) {}
}

async function clearData() {
  try {
    await ElMessageBox.confirm('确定要清除所有数据吗？此操作不可恢复！', '警告', { confirmButtonClass: 'el-button--danger' })
    ElMessage.success('数据已清除')
  } catch (e) {}
}

onMounted(() => {
  loadSettings()
  loadVersion()
})
</script>
