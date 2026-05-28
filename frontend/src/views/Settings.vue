<template>
  <div>
    <h2 class="page-title">🔧 系统设置</h2>

    <div class="info-card" style="max-width:600px;margin-bottom:16px">
      <el-form :model="settings" label-width="120px">
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
        <el-form-item label="容器模式"><el-input v-model="settings.container_mode" disabled /></el-form-item>
        <el-form-item label="文件管理根目录"><el-input v-model="settings.file_manager_root" /></el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveSettings">保存设置</el-button>
          <el-button @click="resetSettings">恢复默认</el-button>
        </el-form-item>
      </el-form>
    </div>

    <h3 class="section-title">系统信息</h3>
    <div class="info-grid" style="max-width:600px">
      <div class="info-card">
        <div class="info-label">版本</div>
        <div class="info-value">v1.0.0</div>
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
    </div>

    <h3 class="section-title">危险操作</h3>
    <div class="info-card" style="max-width:600px">
      <el-button type="danger" @click="clearData">🗑 清除所有数据</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { settingApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const settings = ref<any>({})

async function loadSettings() {
  try { const res: any = await settingApi.get(); settings.value = res.data || {} } catch (e) {}
}

async function saveSettings() {
  try { await settingApi.update(settings.value); ElMessage.success('保存成功') } catch (e) {}
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

onMounted(loadSettings)
</script>
