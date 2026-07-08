<template>
  <div class="main-app" :class="{ collapsed: sidebarCollapsed }">
    <aside class="sidebar">
      <div class="sidebar-header">
        <span class="sidebar-logo" style="margin-left:-2px">🍔</span>
        <span class="sidebar-title">MiniPanel</span>
      </div>
      <nav class="sidebar-nav">
        <router-link
          v-for="item in menus"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ active: $route.path === item.path }"
        >
          <el-icon class="nav-icon" size="18"><component :is="item.icon" /></el-icon>
          <span class="nav-text">{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="sidebar-footer">
        <div class="sidebar-footer-row">
          <el-button class="btn-theme" @click="themeStore.toggleTheme()" circle size="small">
            <el-icon size="16">
              <Sunny v-if="themeStore.isDark" />
              <Moon v-else />
            </el-icon>
          </el-button>
          <el-button class="btn-ghost" @click="logout" :icon="SwitchButton" size="small" style="flex:1">
            退出
          </el-button>
        </div>
      </div>
    </aside>
    <main class="content">
      <router-view v-slot="{ Component }">
        <keep-alive>
          <component :is="Component" />
        </keep-alive>
      </router-view>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import {
  Odometer, Folder, Monitor, Cpu, Box, Shop, Timer, Document, Setting,
  SwitchButton, Sunny, Moon, User
} from '@element-plus/icons-vue'
import { useAuthStore, useThemeStore } from '../store'
import { authApi } from '../api'

const router = useRouter()
const auth = useAuthStore()
const themeStore = useThemeStore()
const sidebarCollapsed = ref(false)

const allMenus = [
  { path: '/dashboard', label: '仪表盘', icon: Odometer },
  { path: '/monitor', label: '监控中心', icon: Monitor },
  { path: '/backups', label: '备份恢复', icon: Folder },
  { path: '/users', label: '用户管理', icon: User, adminOnly: true },
  { path: '/containers', label: '容器管理', icon: Box },
  { path: '/apps', label: '应用商店', icon: Shop },
  { path: '/websites', label: '网站管理', icon: Monitor },
  { path: '/databases', label: '数据库', icon: Folder },
  { path: '/firewall', label: '防火墙', icon: Setting },
  { path: '/files', label: '文件管理', icon: Folder },
  { path: '/processes', label: '进程管理', icon: Cpu },
  { path: '/cronjobs', label: '计划任务', icon: Timer },
  { path: '/ssh', label: 'SSH管理', icon: Monitor },
  { path: '/agent', label: 'Mini Agent', icon: Monitor },
  { path: '/logs', label: '系统日志', icon: Document },
  { path: '/settings', label: '设置', icon: Setting },
]

const menus = computed(() => {
  // 管理员拥有全部菜单
  if (auth.isAdmin) return allMenus
  // 普通用户：按权限过滤 + 隐藏 adminOnly
  const filtered = allMenus.filter(m => {
    if (m.adminOnly) return false
    return auth.hasFeature(m.path)
  })
  // 兜底：过滤后为空时至少显示默认菜单（防止老用户 permissions 为空导致左侧空白）
  if (filtered.length === 0) {
    return allMenus.filter(m => !m.adminOnly && ['/dashboard', '/monitor', '/logs'].includes(m.path))
  }
  return filtered
})

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
  width: var(--sb-width);
  min-width: var(--sb-width);
  background: var(--sb);
  border-right: 1px solid var(--bdr);
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  flex-shrink: 0;
  transition: width 0.2s, min-width 0.2s;
}
.sidebar-header {
  padding: 18px 16px;
  display: flex;
  align-items: center;
  gap: 10px;
  border-bottom: 1px solid var(--bdr);
}
.sidebar-logo {
  font-size: 26px;
  margin-left: -2px;
}
.sidebar-title {
  font-size: 17px;
  font-weight: 700;
  color: var(--txt);
  letter-spacing: -0.3px;
}
.sidebar-nav {
  flex: 1;
  padding: 8px 0;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 16px;
  margin: 1px 8px;
  color: var(--dim);
  text-decoration: none;
  font-size: 13px;
  border-radius: var(--r);
  transition: all 0.12s;
  cursor: pointer;
  font-weight: 500;
}
.nav-item:hover {
  color: var(--txt);
  background: var(--sb-hover);
}
.nav-item.active {
  color: var(--acc);
  background: var(--acc-bg);
  font-weight: 600;
}
.nav-icon {
  width: 22px;
  text-align: center;
  flex-shrink: 0;
}
.sidebar-footer {
  padding: 10px 12px;
  border-top: 1px solid var(--bdr);
}
.sidebar-footer-row {
  display: flex;
  gap: 6px;
  align-items: center;
}
.btn-theme {
  background: var(--card) !important;
  border: 1px solid var(--bdr) !important;
  color: var(--txt) !important;
  flex-shrink: 0;
}
.btn-theme:hover {
  border-color: var(--acc) !important;
  color: var(--acc) !important;
}
.btn-ghost {
  background: transparent !important;
  border: 1px solid var(--bdr) !important;
  color: var(--dim) !important;
  justify-content: center;
}
.btn-ghost:hover {
  color: var(--red) !important;
  border-color: var(--red) !important;
}
.content {
  flex: 1;
  padding: 24px 28px;
  overflow-y: auto;
  overflow-x: hidden;
  background: var(--bg);
}
@media (max-width: 768px) {
  .sidebar { width: 56px; min-width: 56px; }
  .sidebar-title, .nav-text { display: none; }
  .sidebar-header { justify-content: center; padding: 14px 8px; }
  .sidebar-logo { font-size: 22px; }
  .nav-item { justify-content: center; padding: 10px 0; margin: 1px 4px; }
  .nav-icon { width: auto; }
  .content { padding: 16px; }
}
</style>
