<template>
  <div class="login-page">
    <div class="login-bg"></div>
    <div class="login-card">
      <div class="login-logo">🍔</div>
      <h1 class="login-title">MiniPanel</h1>
      <p class="login-subtitle">服务器管理面板</p>
      <form @submit.prevent="handleLogin">
        <div class="input-group">
          <label>用户名</label>
          <el-input v-model="form.username" placeholder="输入用户名" :prefix-icon="User" size="large" />
        </div>
        <div class="input-group">
          <label>密码</label>
          <el-input v-model="form.password" type="password" placeholder="输入密码" :prefix-icon="Lock" size="large" @keyup.enter="handleLogin" />
        </div>
        <div v-if="error" class="login-error">{{ error }}</div>
        <el-button type="primary" :loading="loading" @click="handleLogin" class="btn-block" size="large">登 录</el-button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { useAuthStore } from '../store'
import { authApi } from '../api'

const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)
const error = ref('')

const form = reactive({ username: 'admin', password: 'admin123' })

async function handleLogin() {
  if (!form.username || !form.password) {
    error.value = '请输入用户名和密码'
    return
  }
  error.value = ''
  loading.value = true
  try {
    const res: any = await authApi.login(form)
    auth.setAuth(res.data.token, res.data.username)
    router.push('/')
  } catch (e: any) {
    error.value = e?.response?.data?.message || '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}
.login-bg {
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, #0f1117 0%, #1a1d2e 50%, #0f1117 100%);
}
.login-card {
  position: relative;
  background: var(--card);
  border: 1px solid var(--bdr);
  border-radius: 16px;
  padding: 48px 40px;
  width: 380px;
  text-align: center;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}
.login-logo {
  font-size: 48px;
  margin-bottom: 8px;
}
.login-title {
  font-size: 24px;
  font-weight: 700;
  margin-bottom: 4px;
  color: var(--txt);
}
.login-subtitle {
  color: var(--dim);
  margin-bottom: 32px;
  font-size: 14px;
}
.input-group {
  margin-bottom: 16px;
  text-align: left;
}
.input-group label {
  display: block;
  font-size: 12px;
  color: var(--dim);
  margin-bottom: 6px;
  font-weight: 500;
}
.login-error {
  color: var(--red);
  font-size: 13px;
  margin-bottom: 12px;
  padding: 8px;
  background: rgba(248, 113, 113, 0.1);
  border-radius: var(--r);
}
.btn-block {
  width: 100%;
}
</style>
