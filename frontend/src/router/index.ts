import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../store'

// 需要进行功能权限检查的路由（登录页和首页除外）
const permissionRoutes: Record<string, boolean> = {
  '/dashboard': true,
  '/monitor': true,
  '/backups': true,
  '/containers': true,
  '/apps': true,
  '/websites': true,
  '/databases': true,
  '/firewall': true,
  '/files': true,
  '/processes': true,
  '/cronjobs': true,
  '/ssh': true,
  '/agent': true,
  '/logs': true,
  '/settings': true
}

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/Login.vue')
  },
  {
    path: '/',
    name: 'Layout',
    component: () => import('../views/Layout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'dashboard', name: 'Dashboard', component: () => import('../views/Dashboard.vue') },
      { path: 'monitor', name: 'Monitor', component: () => import('../views/Monitor.vue') },
      { path: 'backups', name: 'Backups', component: () => import('../views/Backup.vue') },
      { path: 'users', name: 'Users', component: () => import('../views/Users.vue') },
      { path: 'containers', name: 'Containers', component: () => import('../views/Container.vue') },
      { path: 'apps', name: 'Apps', component: () => import('../views/AppStore.vue') },
      { path: 'websites', name: 'Websites', component: () => import('../views/Websites.vue') },
      { path: 'databases', name: 'Databases', component: () => import('../views/Databases.vue') },
      { path: 'firewall', name: 'Firewall', component: () => import('../views/Firewall.vue') },
      { path: 'files', name: 'Files', component: () => import('../views/FileManager.vue') },
      { path: 'processes', name: 'Processes', component: () => import('../views/Process.vue') },
      { path: 'cronjobs', name: 'Cronjobs', component: () => import('../views/Cronjob.vue') },
      { path: 'ssh', name: 'SSH', component: () => import('../views/SSH.vue') },
      { path: 'agent', name: 'Agent', component: () => import('../views/MiniAgent.vue') },
      { path: 'logs', name: 'Logs', component: () => import('../views/PanelLogs.vue') },
      { path: 'settings', name: 'Settings', component: () => import('../views/Settings.vue') }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, _from, next) => {
  const auth = useAuthStore()
  if (to.path !== '/login' && !auth.token) {
    next('/login')
    return
  }
  if (to.path === '/users' && auth.role !== 'admin') {
    next('/dashboard')
    return
  }
  // 普通用户访问无权限页面时跳回 dashboard
  if (to.path !== '/login' && auth.token && auth.role !== 'admin' && permissionRoutes[to.path] && !auth.hasFeature(to.path)) {
    next('/dashboard')
    return
  }
  next()
})

export default router
