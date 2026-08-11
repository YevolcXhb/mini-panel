import axios from 'axios'
import { useAuthStore } from '../store'
import { ElMessage } from 'element-plus'

function getCookie(name: string): string {
  const value = `; ${document.cookie}`
  const parts = value.split(`; ${name}=`)
  if (parts.length === 2) return parts.pop()?.split(';').shift() || ''
  return ''
}

const api = axios.create({
  baseURL: '/api/v1'
})

api.interceptors.request.use((config) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  const entrance = getCookie('SecurityEntrance')
  if (entrance) {
    config.headers.EntranceCode = entrance
  }
  return config
})

api.interceptors.response.use(
  (res) => {
    if (res.config.responseType === 'blob') {
      return res
    }
    if (res.data.code !== 200) {
      ElMessage.error(res.data.message || '请求失败')
      return Promise.reject(new Error(res.data.message || '请求失败'))
    }
    return res.data
  },
  (err) => {
    if (err.response?.status === 401) {
      const auth = useAuthStore()
      auth.clearAuth()
      window.location.href = '/login'
      return Promise.reject(err)
    }
    const errorMsg = err.response?.data?.message || err.message || '网络错误'
    ElMessage.error(errorMsg)
    return Promise.reject(err)
  }
)

export default api

export const authApi = {
  login: (data: any) => api.post('/login', data),
  logout: () => api.post('/logout'),
  captcha: () => api.get('/captcha')
}

export const dashboardApi = {
  getInfo: () => api.get('/dashboard'),
  getMonitor: (mode?: string) => api.get('/dashboard/monitor', { params: mode ? { mode } : {} })
}

export const fileApi = {
  list: (path: string) => api.get('/files', { params: { path } }),
  getContent: (path: string) => api.get('/files/content', { params: { path } }),
  create: (data: any) => api.post('/files', data),
  update: (data: any) => api.put('/files', data),
  delete: (path: string) => api.delete('/files', { data: { path } }),
	  forceDelete: (path: string) => api.delete('/files/force', { data: { path } }),
  upload: (data: FormData | string, file?: File) => {
    if (data instanceof FormData) {
      return api.post('/files/upload', data)
    }
    const form = new FormData()
    form.append('path', data)
    form.append('file', file!)
    return api.post('/files/upload', form)
  },
  download: (path: string) => api.get('/files/download', { params: { path }, responseType: 'blob' }),
	  uploadMultiple: (path: string, files: File[]) => {
	    const form = new FormData()
	    form.append('path', path)
	    files.forEach(f => form.append('files', f))
	    return api.post('/files/upload-multiple', form)
	  },
	  downloadZip: (path: string) => api.get('/files/download-zip', { params: { path } }),
	  rename: (path: string, newName: string) => api.post('/files/rename', { path, new_name: newName }),
	  chmod: (path: string, mode: string, recursive: boolean) => api.post('/files/chmod', { path, mode, recursive }),
	  compress: (paths: string[], output: string, format: string) => api.post('/files/compress', { paths, output, format }),
	  extract: (path: string, destDir: string) => api.post('/files/extract', { path, dest_dir: destDir }),
	  copy: (srcPath: string, destPath: string) => api.post('/files/copy', { src_path: srcPath, dest_path: destPath }),
	  move: (srcPath: string, destPath: string) => api.post('/files/move', { src_path: srcPath, dest_path: destPath }),
	  search: (path: string, search: string) => api.get('/files/search', { params: { path, search } }),
	  listRecycleBin: () => api.get('/files/recycle-bin'),
	  restoreRecycle: (path: string) => api.post('/files/recycle-bin/restore', { path }),
	  clearRecycleBin: () => api.post('/files/recycle-bin/clear'),
}

export const phpApi = {
  getVersions: () => api.get('/php/versions'),
  installVersion: (version: string) => api.post('/php/versions/install', { version }, { timeout: 15 * 60 * 1000 }),
  removeVersion: (version: string) => api.delete(`/php/versions/${version}`),
  startFpm: (version: string) => api.post(`/php/versions/${version}/start`),
  stopFpm: (version: string) => api.post(`/php/versions/${version}/stop`),
  restartFpm: (version: string) => api.post(`/php/versions/${version}/restart`),
  getExtensions: (version: string) => api.get(`/php/versions/${version}/extensions`),
  installExtension: (version: string, name: string) => api.post(`/php/versions/${version}/extensions`, { name }),
  removeExtension: (version: string, name: string) => api.delete(`/php/versions/${version}/extensions/${name}`),
  getConfig: (version: string) => api.get(`/php/versions/${version}/config`),
  updateConfig: (version: string, items: any[]) => api.put(`/php/versions/${version}/config`, items),
  getSocket: (version: string) => api.get(`/php/versions/${version}/socket`),
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
  installStatus: (name: string) => api.get(`/apps/install/${name}/status`),
  uninstall: (id: number) => api.post(`/apps/${id}/uninstall`),
  clearHistory: () => api.delete('/apps/history'),
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

export const updateApi = {
  check: () => api.get('/update/check'),
  apply: () => api.post('/update/apply'),
  status: () => api.get('/update/status'),
  log: (tail = 100) => api.get('/update/log', { params: { tail } })
}

export const monitorApi = {
	  history: (limit?: number) => api.get('/monitor/history', { params: { limit } }),
	  realtime: () => api.get('/monitor/realtime')
	}

export const websiteApi = {
  list: () => api.get('/websites'),
  create: (data: any) => api.post('/websites', data),
  update: (id: number, data: any) => api.put(`/websites/${id}`, data),
  delete: (id: number, cascadeDB: boolean = true) => api.delete(`/websites/${id}`, { params: { cascade_db: cascadeDB ? 'true' : 'false' } }),
  deleteExternal: (domain: string, port: number) => api.delete(`/websites/0?domain=${domain}&port=${port}`),
  toggle: (id: number, enabled: boolean) => api.put(`/websites/${id}/toggle`, { enabled }),
  toggleExternal: (domain: string, port: number, enabled: boolean) => api.put(`/websites/0/toggle?domain=${domain}&port=${port}`, { enabled }),
  getAccessLogs: (id: number, params: any) => api.get(`/websites/${id}/logs`, { params }),
  getTrafficStats: (id: number, period: string) => api.get(`/websites/${id}/traffic`, { params: { period } }),
  listDatabases: (id: number) => api.get(`/websites/${id}/databases`),
  listWebsitesByDB: (instanceId: number) => api.get(`/databases/${instanceId}/websites`),
  getNginxStatus: () => api.get('/websites/nginx/status'),
  startNginx: () => api.post('/websites/nginx/start'),
  stopNginx: () => api.post('/websites/nginx/stop'),
  restartNginx: () => api.post('/websites/nginx/restart'),
  reloadNginx: () => api.post('/websites/nginx/reload')
}

export const databaseApi = {
  list: () => api.get('/databases'),
  create: (data: any) => api.post('/databases', data),
  update: (id: number, data: any) => api.put(`/databases/${id}`, data),
  delete: (id: number) => api.delete(`/databases/${id}`),
  test: (data: any) => api.post('/databases/test', data),
  listDatabases: (id: number) => api.get(`/databases/${id}/dbs`),
  listTables: (id: number, dbName?: string) => api.get(`/databases/${id}/tables`, { params: dbName ? { db_name: dbName } : {} }),
  createDatabase: (id: number, dbName: string) => api.post(`/databases/${id}/create-db`, { db_name: dbName }),
  createUser: (id: number, data: any) => api.post(`/databases/${id}/create-user`, data),
  dropDatabase: (id: number, dbName: string) => api.delete(`/databases/${id}/dbs/${dbName}`),
  dropUser: (id: number, username: string) => api.delete(`/databases/${id}/users/${username}`),
  describeTable: (id: number, dbName: string, tableName: string) => api.get(`/databases/${id}/tables/${dbName}/${tableName}`),
	  executeQuery: (id: number, dbName: string, query: string) => api.post(`/databases/${id}/query`, { db_name: dbName, query }),
	  backup: (id: number, dbName: string) => api.post(`/databases/${id}/backup/${dbName}`),
	  restore: (id: number, dbName: string, filePath: string) => api.post(`/databases/${id}/restore/${dbName}`, { file_path: filePath })
}

export const firewallApi = {
  list: () => api.get('/firewall/rules'),
  listDeleted: () => api.get('/firewall/rules/deleted'),
  restoreRule: (id: number) => api.post(`/firewall/rules/${id}/restore`),
  clearDeletedRules: () => api.post('/firewall/rules/clear-deleted'),
  create: (data: any) => api.post('/firewall/rules', data),
  update: (id: number, data: any) => api.put(`/firewall/rules/${id}`, data),
  delete: (id: number) => api.delete(`/firewall/rules/${id}`),
  apply: () => api.post('/firewall/apply'),
  status: () => api.get('/firewall/status'),
  start: () => api.post('/firewall/start'),
  stop: () => api.post('/firewall/stop'),
  diagnose: () => api.get('/firewall/diagnose'),
  liveRules: (chain?: string, family?: string, table?: string) => api.get('/firewall/live-rules', { params: { ...(chain ? { chain } : {}), ...(family ? { family } : {}), ...(table ? { table } : {}) } }),
  insertRule: (data: { chain: string; position: number; spec: string[]; family?: string }) => api.post('/firewall/insert', data),
  deleteLiveRule: (chain: string, num: number, family?: string) => api.delete('/firewall/live-rule', { params: { chain, num, ...(family ? { family } : {}) } }),
  lockdown: () => api.post('/firewall/lockdown')
}

export const systemApi = {
  checkServices: () => api.get('/system/services'),
  installService: (name: string) => api.post(`/system/services/${name}/install`),
  startService: (name: string) => api.post(`/system/services/${name}/start`),
  stopService: (name: string) => api.post(`/system/services/${name}/stop`),
  restartService: (name: string) => api.post(`/system/services/${name}/restart`)
}

export const userApi = {
  list: () => api.get('/users'),
  listFeatures: () => api.get('/users/features'),
  create: (data: any) => api.post('/users', data),
  update: (id: number, data: any) => api.put(`/users/${id}`, data),
  resetPassword: (id: number, data: any) => api.post(`/users/${id}/reset-password`, data),
  deleteUser: (id: number) => api.delete(`/users/${id}`)
}

export const backupApi = {
  listTasks: () => api.get('/backups/tasks'),
  createTask: (data: any) => api.post('/backups/tasks', data),
  updateTask: (id: number, data: any) => api.put(`/backups/tasks/${id}`, data),
  deleteTask: (id: number) => api.delete(`/backups/tasks/${id}`),
  runTask: (id: number) => api.post(`/backups/tasks/${id}/run`),
  listRecords: (taskId?: number) => api.get('/backups/records', { params: { task_id: taskId } }),
  deleteRecord: (id: number) => api.delete(`/backups/records/${id}`),
  restoreRecord: (id: number) => api.post(`/backups/records/${id}/restore`)
}
