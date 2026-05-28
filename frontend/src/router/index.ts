import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../store'

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
      { path: 'files', name: 'Files', component: () => import('../views/FileManager.vue') },
      { path: 'terminal', name: 'Terminal', component: () => import('../views/Terminal.vue') },
      { path: 'processes', name: 'Processes', component: () => import('../views/Process.vue') },
      { path: 'containers', name: 'Containers', component: () => import('../views/Container.vue') },
      { path: 'apps', name: 'Apps', component: () => import('../views/AppStore.vue') },
      { path: 'cronjobs', name: 'Cronjobs', component: () => import('../views/Cronjob.vue') },
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
  } else {
    next()
  }
})

export default router
