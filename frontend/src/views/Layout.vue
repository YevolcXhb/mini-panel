<template>
  <div class="main-app">
    <aside class="sidebar">
      <div class="sidebar-header">
        <span class="sidebar-logo">🍔</span>
        <span class="sidebar-title">MiniPanel</span>
      </div>
      <nav class="sidebar-nav">
        <router-link v-for="item in menus" :key="item.path" :to="item.path"
          class="nav-item" :class="{ active: $route.path === item.path }">
          <el-icon class="nav-icon" size="18"><component :is="item.icon" /></el-icon>
          <span class="nav-text">{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="sidebar-footer">
        <el-button class="btn-ghost" @click="logout" :icon="SwitchButton">退出登录</el-button>
      </div>
    </aside>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import {
  Odometer, Folder, Monitor, Cpu, Box, Shop, Timer, Document, Setting, SwitchButton
} from '@element-plus/icons-vue'
import { useAuthStore } from '../store'
import { authApi } from '../api'

const router = useRouter()
const auth = useAuthStore()

const menus = [
  { path: '/dashboard', label: '仪表盘', icon: Odometer },
  { path: '/files', label: '文件管理', icon: Folder },
  { path: '/terminal', label: '终端', icon: Monitor },
  { path: '/processes', label: '进程管理', icon: Cpu },
  { path: '/containers', label: '容器管理', icon: Box },
  { path: '/apps', label: '应用商店', icon: Shop },
  { path: '/cronjobs', label: '计划任务', icon: Timer },
  { path: '/logs', label: '系统日志', icon: Document },
  { path: '/settings', label: '设置', icon: Setting },
]

function logout() {
  authApi.logout()
  auth.clearAuth()
  router.push('/login')
}
</script>

<style scoped>
.main-app {
  display: flex;
  height: 100vh;
  width: 100vw;
}
.sidebar {
  width: 220px;
  min-width: 220px;
  background: var(--sb);
  border-right: 1px solid var(--bdr);
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  flex-shrink: 0;
}
.sidebar-header {
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 10px;
  border-bottom: 1px solid var(--bdr);
}
.sidebar-logo {
  font-size: 28px;
}
.sidebar-title {
  font-size: 18px;
  font-weight: 700;
  color: var(--txt);
}
.sidebar-nav {
  flex: 1;
  padding: 12px 0;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 20px;
  color: var(--dim);
  text-decoration: none;
  font-size: 14px;
  border-left: 3px solid transparent;
  transition: all 0.15s;
  cursor: pointer;
}
.nav-item:hover {
  color: var(--txt);
  background: rgba(255, 255, 255, 0.03);
}
.nav-item.active {
  color: var(--acc);
  background: rgba(79, 140, 255, 0.08);
  border-left-color: var(--acc);
}
.nav-icon {
  width: 24px;
  text-align: center;
}
.sidebar-footer {
  padding: 12px 16px;
  border-top: 1px solid var(--bdr);
}
.sidebar-footer .el-button {
  width: 100%;
  text-align: left;
  justify-content: flex-start;
}
.content {
  flex: 1;
  padding: 24px 32px;
  overflow-y: auto;
  overflow-x: hidden;
}
@media (max-width: 768px) {
  .sidebar { width: 60px; min-width: 60px; }
  .sidebar-title, .nav-text { display: none; }
  .sidebar-header { justify-content: center; padding: 16px 8px; }
  .sidebar-logo { font-size: 24px; }
  .nav-item { justify-content: center; padding: 12px 0; border-left: none; }
  .nav-icon { width: auto; }
  .content { padding: 16px; }
}
</style>
