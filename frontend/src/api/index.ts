import axios from 'axios'
import { useAuthStore } from '../store'
import { ElMessage } from 'element-plus'

const api = axios.create({
  baseURL: '/api/v1'
})

api.interceptors.request.use((config) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  return config
})

api.interceptors.response.use(
  (res) => {
    if (res.data.code !== 200) {
      ElMessage.error(res.data.message || '请求失败')
      return Promise.reject(new Error(res.data.message))
    }
    return res.data
  },
  (err) => {
    if (err.response?.status === 401) {
      const auth = useAuthStore()
      auth.clearAuth()
      window.location.href = '/login'
    }
    ElMessage.error(err.message || '网络错误')
    return Promise.reject(err)
  }
)

export default api

export const authApi = {
  login: (data: any) => api.post('/login', data),
  logout: () => api.post('/logout')
}

export const dashboardApi = {
  getInfo: () => api.get('/dashboard'),
  getMonitor: () => api.get('/dashboard/monitor')
}

export const fileApi = {
  list: (path: string) => api.get('/files', { params: { path } }),
  getContent: (path: string) => api.get('/files/content', { params: { path } }),
  create: (data: any) => api.post('/files', data),
  update: (data: any) => api.put('/files', data),
  delete: (path: string) => api.delete('/files', { data: { path } }),
  upload: (path: string, file: File) => {
    const form = new FormData()
    form.append('path', path)
    form.append('file', file)
    return api.post('/files/upload', form)
  },
  download: (path: string) => api.get('/files/download', { params: { path }, responseType: 'blob' })
}

export const processApi = {
  list: () => api.get('/processes'),
  kill: (pid: string, force = false) => api.post('/processes/kill', { pid, force })
}

export const containerApi = {
  list: () => api.get('/containers'),
  inspect: (name: string) => api.get(`/containers/${name}`),
  create: (data: any) => api.post('/containers', data),
  start: (name: string) => api.post(`/containers/${name}/start`),
  stop: (name: string) => api.post(`/containers/${name}/stop`),
  remove: (name: string) => api.delete(`/containers/${name}`),
  logs: (name: string, tail = 100) => api.get(`/containers/${name}/logs`, { params: { tail } }),
  pull: (image: string, name?: string) => api.post('/containers/pull', { image, name })
}

export const appApi = {
  list: (category?: string) => api.get('/apps', { params: { category } }),
  search: (q: string) => api.get('/apps/search', { params: { q } }),
  icon: (key: string) => `${api.defaults.baseURL}/apps/icon/${key}`,
  detail: (id: number) => api.get(`/apps/${id}`),
  installed: () => api.get('/apps/installed'),
  install: (data: any) => api.post('/apps/install', data),
  uninstall: (id: number) => api.post(`/apps/${id}/uninstall`),
  sync: (source_id: number) => api.post('/apps/sync', { source_id }),
  sources: () => api.get('/apps/sources'),
  addSource: (data: any) => api.post('/apps/sources', data),
  removeSource: (id: number) => api.delete(`/apps/sources/${id}`)
}

export const auditApi = {
  list: (limit?: number) => api.get('/audit-logs', { params: { limit } })
}

export const authApiExt = {
  changePassword: (data: any) => api.post('/auth/change-password', data)
}

export const cronjobApi = {
  list: () => api.get('/cronjobs'),
  create: (data: any) => api.post('/cronjobs', data),
  update: (id: number, data: any) => api.put(`/cronjobs/${id}`, data),
  delete: (id: number) => api.delete(`/cronjobs/${id}`),
  run: (id: number) => api.post(`/cronjobs/${id}/run`)
}

export const settingApi = {
  get: () => api.get('/settings'),
  update: (data: any) => api.put('/settings', data),
  reset: () => api.post('/settings/reset'),
  clearData: () => api.post('/settings/clear-data')
}

export const logApi = {
  list: (levels?: string[], lines?: number) => api.get('/logs', {
    params: { levels: levels?.join(','), lines }
  })
}

export const versionApi = {
  get: () => api.get('/version')
}

export const monitorApi = {
  history: (limit?: number) => api.get('/monitor/history', { params: { limit } })
}

export const websiteApi = {
  list: () => api.get('/websites'),
  create: (data: any) => api.post('/websites', data),
  update: (id: number, data: any) => api.put(`/websites/${id}`, data),
  delete: (id: number) => api.delete(`/websites/${id}`),
  toggle: (id: number, enabled: boolean) => api.put(`/websites/${id}/toggle`, { enabled }),
  reloadNginx: () => api.post('/websites/reload-nginx')
}
