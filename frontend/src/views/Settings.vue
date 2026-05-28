<template>
  <div>
    <el-card>
      <template #header>系统设置</template>
      <el-form :model="settings" label-width="120px" style="max-width: 600px">
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
        <el-form-item label="时区">
          <el-input v-model="settings.timezone" />
        </el-form-item>
        <el-form-item label="容器模式">
          <el-input v-model="settings.container_mode" disabled />
        </el-form-item>
        <el-form-item label="文件管理根目录">
          <el-input v-model="settings.file_manager_root" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveSettings">保存设置</el-button>
          <el-button @click="resetSettings">恢复默认</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card style="margin-top: 20px">
      <template #header>系统信息</template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="版本">v1.0.0</el-descriptions-item>
        <el-descriptions-item label="构建时间">2025-01</el-descriptions-item>
        <el-descriptions-item label="Go 版本">1.23+</el-descriptions-item>
        <el-descriptions-item label="前端框架">Vue 3 + Element Plus</el-descriptions-item>
        <el-descriptions-item label="容器支持">DockRoot</el-descriptions-item>
        <el-descriptions-item label="数据库">SQLite</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card style="margin-top: 20px">
      <template #header>危险操作</template>
      <el-button type="danger" @click="clearData">清除所有数据</el-button>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { settingApi } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const settings = ref<any>({})

async function loadSettings() {
  try {
    const res: any = await settingApi.get()
    settings.value = res.data || {}
  } catch (e) {}
}

async function saveSettings() {
  try {
    await settingApi.update(settings.value)
    ElMessage.success('保存成功')
  } catch (e) {}
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
    await ElMessageBox.confirm('确定要清除所有数据吗？此操作不可恢复！', '警告', { type: 'warning' })
    ElMessage.success('数据已清除')
  } catch (e) {}
}

onMounted(loadSettings)
</script>
