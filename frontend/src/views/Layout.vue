<template>
  <el-container class="layout">
    <el-aside width="200px" class="sidebar">
      <div class="logo">
        <el-icon size="24"><Monitor /></el-icon>
        <span>Mini Panel</span>
      </div>
      <el-menu :default-active="$route.path" router class="menu" background-color="#1a1a2e" text-color="#b0b0b0" active-text-color="#409eff">
        <el-menu-item index="/dashboard">
          <el-icon><Odometer /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>
        <el-menu-item index="/files">
          <el-icon><Folder /></el-icon>
          <span>文件管理</span>
        </el-menu-item>
        <el-menu-item index="/terminal">
          <el-icon><Terminal /></el-icon>
          <span>终端</span>
        </el-menu-item>
        <el-menu-item index="/processes">
          <el-icon><Cpu /></el-icon>
          <span>进程管理</span>
        </el-menu-item>
        <el-menu-item index="/containers">
          <el-icon><Box /></el-icon>
          <span>容器管理</span>
        </el-menu-item>
        <el-menu-item index="/apps">
          <el-icon><Shop /></el-icon>
          <span>应用商店</span>
        </el-menu-item>
        <el-menu-item index="/cronjobs">
          <el-icon><Timer /></el-icon>
          <span>计划任务</span>
        </el-menu-item>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <span>设置</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <div class="header-right">
          <el-dropdown @command="handleCommand">
            <span class="user-info">
              <el-icon><User /></el-icon>
              {{ auth.username }}
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="logout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '../store'
import { authApi } from '../api'

const router = useRouter()
const auth = useAuthStore()

function handleCommand(cmd: string) {
  if (cmd === 'logout') {
    authApi.logout()
    auth.clearAuth()
    router.push('/login')
  }
}
</script>

<style scoped>
.layout {
  height: 100vh;
}
.sidebar {
  background: #1a1a2e;
}
.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #fff;
  font-size: 18px;
  font-weight: bold;
  border-bottom: 1px solid #2a2a3e;
}
.menu {
  border-right: none;
}
.header {
  background: #fff;
  border-bottom: 1px solid #eee;
  display: flex;
  align-items: center;
  justify-content: flex-end;
}
.header-right {
  display: flex;
  align-items: center;
}
.user-info {
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 5px;
}
.main {
  background: #f5f7fa;
  padding: 20px;
  overflow-y: auto;
}
</style>
