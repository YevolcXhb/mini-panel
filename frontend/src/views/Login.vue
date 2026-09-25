<template>
  <div class="login-page">
    <div class="login-bg">
      <div class="orb orb-1"></div>
      <div class="orb orb-2"></div>
      <div class="orb orb-3"></div>
    </div>
    <div class="login-card">
      <div class="login-logo">🍔</div>
      <h1 class="login-title">MiniPanel</h1>
      <p class="login-subtitle">服务器管理面板</p>
      <form @submit.prevent="handleLogin" autocomplete="off">
        <div class="input-group">
          <label>用户名</label>
          <el-input
            v-model="form.username"
            placeholder="输入用户名"
            :prefix-icon="User"
            size="large"
            name="username"
            autocomplete="off"
            readonly
            @focus="removeReadonly($event, 'username')"
          />
        </div>
        <div class="input-group">
          <label>密码</label>
          <el-input
            v-model="form.password"
            type="password"
            placeholder="输入密码"
            :prefix-icon="Lock"
            size="large"
            name="password"
            autocomplete="new-password"
            readonly
            @focus="removeReadonly($event, 'password')"
            @keyup.enter="handleLogin"
          />
        </div>
        <div v-if="showCaptcha" class="input-group">
          <label>验证码</label>
          <div class="captcha-row">
            <el-input
              v-model="form.captcha"
              placeholder="输入验证码"
              size="large"
              autocomplete="off"
              style="flex:1"
              @keyup.enter="handleLogin"
            />
            <img
              :src="captchaImg"
              class="captcha-img"
              @click="loadCaptcha"
              title="点击刷新验证码"
            />
          </div>
        </div>
        <div v-if="error" class="login-error">{{ error }}</div>
        <button type="submit" class="login-btn" :disabled="loading">
          <span v-if="loading" class="btn-loading"></span>
          <span v-else>登 录</span>
        </button>
      </form>
      <p class="login-footer">Secured by MiniPanel</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { useAuthStore } from '../store'
import { authApi } from '../api'

const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)
const error = ref('')
const showCaptcha = ref(false)
const captchaImg = ref('')
const captchaID = ref('')

const form = reactive({ username: '', password: '', captcha: '' })

function removeReadonly(e: FocusEvent, field: string) {
  const target = e.target as HTMLInputElement
  target.removeAttribute('readonly')
  target.focus()
}

async function loadCaptcha() {
  try {
    const res: any = await authApi.captcha()
    captchaImg.value = 'data:image/png;base64,' + res.data.image
    captchaID.value = res.data.captcha_id
    showCaptcha.value = true
  } catch {
    // 验证码接口不可用时静默处理
  }
}

async function handleLogin() {
  if (!form.username || !form.password) {
    error.value = '请输入用户名和密码'
    return
  }
  if (showCaptcha.value && !form.captcha) {
    error.value = '请输入验证码'
    return
  }
  error.value = ''
  loading.value = true
  try {
    const res: any = await authApi.login({
      username: form.username,
      password: form.password,
      captcha: form.captcha,
      captcha_id: captchaID.value
    })
    auth.setAuth(res.data.token, res.data.username, res.data.role, res.data.permissions || [])
    router.push('/')
  } catch (e: any) {
    const msg = e?.response?.data?.message || e?.message || '登录失败'
    error.value = msg
    if (!showCaptcha.value) {
      loadCaptcha()
    } else {
      loadCaptcha()
    }
    form.captcha = ''
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadCaptcha()
  // 清除浏览器可能记住的密码
  setTimeout(() => {
    form.password = ''
    form.username = ''
  }, 100)
})
</script>

<style scoped>
.login-page {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
  overflow: hidden;
}

/* ── 背景：深色渐变 + 漂浮光球 ── */
.login-bg {
  position: absolute;
  inset: 0;
  background: #0a0a0f;
}
.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.35;
  animation: float 12s ease-in-out infinite alternate;
}
.orb-1 {
  width: 340px; height: 340px;
  background: #4285f4;
  top: -80px; left: 10%;
  animation-duration: 14s;
}
.orb-2 {
  width: 280px; height: 280px;
  background: #9b72cf;
  bottom: -60px; right: 12%;
  animation-delay: -4s;
  animation-duration: 16s;
}
.orb-3 {
  width: 200px; height: 200px;
  background: #d96570;
  top: 40%; left: 55%;
  animation-delay: -8s;
  animation-duration: 18s;
}
@keyframes float {
  0%   { transform: translate(0, 0) scale(1); }
  50%  { transform: translate(30px, -20px) scale(1.08); }
  100% { transform: translate(-20px, 15px) scale(0.95); }
}

/* ── 毛玻璃登录卡 ── */
.login-card {
  position: relative;
  background: rgba(30, 31, 35, 0.75);
  backdrop-filter: blur(24px) saturate(1.6);
  -webkit-backdrop-filter: blur(24px) saturate(1.6);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 24px;
  padding: 48px 44px 36px;
  width: 400px;
  text-align: center;
  box-shadow:
    0 24px 80px rgba(0, 0, 0, 0.6),
    0 0 0 1px rgba(255, 255, 255, 0.04) inset;
  animation: card-in 0.5s cubic-bezier(0.16, 1, 0.3, 1) both;
}
@keyframes card-in {
  from { opacity: 0; transform: translateY(20px) scale(0.97); }
  to   { opacity: 1; transform: translateY(0) scale(1); }
}

.login-logo {
  font-size: 52px;
  margin-bottom: 8px;
  filter: drop-shadow(0 4px 12px rgba(0, 0, 0, 0.3));
}

.login-title {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 4px;
  background: linear-gradient(135deg, #8ab4f8 0%, #c4a7e7 50%, #f2a8b8 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  letter-spacing: 1px;
}

.login-subtitle {
  color: rgba(255, 255, 255, 0.4);
  margin: 0 0 36px;
  font-size: 13px;
  letter-spacing: 2px;
}

/* ── 输入框 ── */
.input-group {
  margin-bottom: 20px;
  text-align: left;
}
.input-group label {
  display: block;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  margin-bottom: 8px;
  font-weight: 500;
  letter-spacing: 0.5px;
}

.input-group :deep(.el-input__wrapper) {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
  box-shadow: none;
  transition: all 0.25s ease;
}
.input-group :deep(.el-input__wrapper:hover) {
  border-color: rgba(138, 180, 248, 0.3);
}
.input-group :deep(.el-input__wrapper.is-focus) {
  border-color: #8ab4f8;
  box-shadow: 0 0 0 3px rgba(138, 180, 248, 0.12);
  background: rgba(255, 255, 255, 0.08);
}
.input-group :deep(.el-input__inner) {
  color: #e8eaed;
}
.input-group :deep(.el-input__inner::placeholder) {
  color: rgba(255, 255, 255, 0.25);
}
.input-group :deep(.el-input__prefix .el-icon) {
  color: rgba(255, 255, 255, 0.3);
}

/* ── 验证码 ── */
.captcha-row {
  display: flex;
  gap: 12px;
  align-items: center;
}
.captcha-img {
  height: 40px;
  border-radius: 10px;
  cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.1);
  transition: all 0.2s;
  background: rgba(255, 255, 255, 0.9);
}
.captcha-img:hover {
  border-color: rgba(138, 180, 248, 0.4);
  transform: scale(1.03);
}

/* ── 错误提示 ── */
.login-error {
  color: #f87171;
  font-size: 13px;
  margin-bottom: 16px;
  padding: 10px 14px;
  background: rgba(248, 113, 113, 0.1);
  border: 1px solid rgba(248, 113, 113, 0.2);
  border-radius: 12px;
  text-align: left;
}

/* ── 渐变登录按钮 ── */
.login-btn {
  width: 100%;
  padding: 14px;
  border: none;
  border-radius: 14px;
  font-size: 15px;
  font-weight: 600;
  color: #fff;
  cursor: pointer;
  letter-spacing: 4px;
  background: linear-gradient(135deg, #4285f4 0%, #6d5ce7 50%, #9b72cf 100%);
  background-size: 200% 200%;
  box-shadow: 0 4px 20px rgba(66, 133, 244, 0.3);
  transition: all 0.3s ease;
  position: relative;
  overflow: hidden;
}
.login-btn:hover {
  background-position: 100% 100%;
  box-shadow: 0 6px 28px rgba(66, 133, 244, 0.45);
  transform: translateY(-1px);
}
.login-btn:active {
  transform: translateY(0);
}
.login-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
  transform: none;
}

.btn-loading {
  display: inline-block;
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}

/* ── 底部文字 ── */
.login-footer {
  margin: 28px 0 0;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.15);
  letter-spacing: 1px;
}
</style>