<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">🛡️</span> 防火墙
      </h2>
    </div>

    <el-alert v-if="!serviceStatus.installed" type="warning" show-icon :closable="false" style="margin-bottom: 16px">
      <template #title>
        <div class="alert-bar">
          <span>{{ serviceStatus.message || '防火墙功能不可用' }}</span>
          <el-button size="small" type="primary" @click="runDiagnose" :loading="diagnosing">一键诊断</el-button>
        </div>
      </template>
      <template #default>
        <div style="margin-top: 8px">
          <pre v-if="serviceStatus.diagnosis" class="diag-pre">{{ serviceStatus.diagnosis }}</pre>
          <p v-else class="muted">点击"一键诊断"获取详细原因</p>
        </div>
      </template>
    </el-alert>

    <template v-else>
      <!-- 状态卡片 -->
      <el-card style="margin-bottom: 16px">
        <div class="svc-bar">
          <div class="svc-bar-left">
            <el-tag :type="serviceStatus.running ? 'success' : 'danger'" size="large">
              {{ serviceStatus.running ? '运行中' : '已停止' }}
            </el-tag>
            <span class="backend-name">{{ firewallType }}</span>
            <el-tag v-if="isNft" type="success" size="small" effect="plain">内核直连 · 无需 nft 命令</el-tag>
            <el-tag v-if="serviceStatus.ipv6_supported" type="success" size="small">IPv6 已支持</el-tag>
          </div>
          <div class="svc-bar-right">
            <el-button size="small" type="primary" @click="startFirewall" v-if="!serviceStatus.running" :loading="actionLoading">启动</el-button>
            <el-button size="small" type="warning" @click="stopFirewall" v-if="serviceStatus.running" :loading="actionLoading">停止</el-button>
            <el-button size="small" @click="checkStatus">刷新</el-button>
          </div>
        </div>

        <!-- nft 后端：库与内核的对照数据 -->
        <div v-if="isNft" class="stat-row">
          <div class="stat">
            <span class="stat-label">面板规则（库）</span>
            <span class="stat-value">{{ serviceStatus.rule_count_db ?? 0 }}</span>
          </div>
          <div class="stat">
            <span class="stat-label">内核规则</span>
            <span class="stat-value">{{ serviceStatus.rule_count_kernel ?? 0 }}</span>
          </div>
          <div class="stat">
            <span class="stat-label">表归属</span>
            <span class="stat-value small">{{ serviceStatus.panel_owned ? 'mp_filter / mp_nat' : '未建立' }}</span>
          </div>
        </div>

        <div v-if="serviceStatus.kernel_warning" class="warn-line">
          ⚠️ {{ serviceStatus.kernel_warning }}
          <el-button size="small" type="primary" link @click="applyRules">立即重新应用</el-button>
        </div>
      </el-card>

      <!-- 快捷操作栏 -->
      <el-card style="margin-bottom: 16px">
        <div class="quick-actions">
          <el-button type="primary" @click="openSystemRules">📋 查看内核规则</el-button>
          <el-button type="success" @click="applyPanelRules">✅ 应用面板规则</el-button>
          <el-button type="primary" @click="openDialog()">➕ 添加面板规则</el-button>
          <el-button type="warning" @click="openInsert">📌 插入内核规则</el-button>
          <el-button type="info" @click="lockdown" :loading="lockdownLoading">🔒 一键内网模式</el-button>
          <el-button type="danger" plain @click="resetAllRules" :loading="resetLoading">🗑️ 清空面板表</el-button>
        </div>
      </el-card>

      <!-- 面板规则表格 -->
      <el-card>
        <div class="section-bar">
          <span class="section-title">面板规则</span>
          <span class="muted">保存后需点「应用面板规则」才会写入内核</span>
        </div>
        <div class="table-wrap">
          <el-table :data="rules" style="width: 100%" v-loading="loading">
            <el-table-column prop="name" label="名称" min-width="130" />
            <el-table-column prop="type" label="类型" width="90">
              <template #default="{ row }">
                <el-tag :type="row.type === 'port' ? 'primary' : row.type === 'dnat' ? 'success' : 'warning'" size="small">
                  {{ row.type === 'port' ? '端口' : row.type === 'dnat' ? '转发' : 'IP' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="action" label="动作" width="80">
              <template #default="{ row }">
                <el-tag :type="row.action === 'allow' ? 'success' : 'danger'" size="small">{{ row.action === 'allow' ? '允许' : '拒绝' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="protocol" label="协议" width="80" />
            <el-table-column prop="port" label="端口" width="140" />
            <el-table-column prop="ip" label="IP / 目标" min-width="140" />
            <el-table-column prop="direction" label="方向" width="80">
              <template #default="{ row }">{{ row.direction === 'in' ? '入站' : '出站' }}</template>
            </el-table-column>
            <el-table-column prop="enabled" label="状态" width="80">
              <template #default="{ row }">
                <el-tag v-if="row.enabled" type="success" size="small">启用</el-tag>
                <el-tag v-else type="info" size="small">停用</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" :width="isWide ? 180 : 140" :fixed="isWide ? 'right' : false">
              <template #default="{ row }">
                <div class="row-actions">
                  <el-button size="small" @click="openDialog(row)">编辑</el-button>
                  <el-button size="small" type="danger" @click="handleDelete(row.id)">删除</el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-card>

      <!-- 回收站 -->
      <el-card style="margin-top: 16px">
        <div class="section-bar">
          <span class="section-title">🗑️ 已删除规则（可恢复）</span>
          <div class="trash-actions">
            <el-button size="small" @click="loadDeletedRules" :loading="deletedLoading">刷新</el-button>
            <el-button size="small" type="primary" @click="restoreAllDeleted" :disabled="!deletedRules.length">全部恢复</el-button>
            <el-button size="small" type="danger" plain @click="clearDeletedRules" :disabled="!deletedRules.length">一键清空</el-button>
          </div>
        </div>
        <div class="table-wrap">
          <el-table :data="deletedRules" size="small" v-loading="deletedLoading" empty-text="没有已删除的规则">
            <el-table-column prop="name" label="名称" min-width="120" />
            <el-table-column prop="type" label="类型" width="80">
              <template #default="{ row }">{{ row.type === 'port' ? '端口' : row.type === 'dnat' ? '转发' : 'IP' }}</template>
            </el-table-column>
            <el-table-column prop="action" label="动作" width="80">
              <template #default="{ row }">{{ row.action === 'allow' ? '允许' : '拒绝' }}</template>
            </el-table-column>
            <el-table-column prop="protocol" label="协议" width="80" />
            <el-table-column prop="port" label="端口" width="140" />
            <el-table-column prop="ip" label="IP" min-width="120" />
            <el-table-column label="操作" width="90" :fixed="isWide ? 'right' : false">
              <template #default="{ row }">
                <div class="row-actions">
                  <el-button size="small" type="primary" @click="restoreRule(row.id)">恢复</el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-card>
    </template>

    <!-- 诊断弹窗 -->
    <el-dialog v-model="diagnoseDialogVisible" title="防火墙环境诊断报告" width="700px">
      <div v-if="diagnoseReport">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="诊断时间">{{ diagnoseReport.timestamp }}</el-descriptions-item>
          <el-descriptions-item label="操作系统">{{ diagnoseReport.platform }} {{ diagnoseReport.platform_supported ? '(支持)' : '(不支持)' }}</el-descriptions-item>
          <el-descriptions-item label="运行权限">
            <el-tag :type="diagnoseReport.is_root ? 'success' : 'danger'">{{ diagnoseReport.is_root ? 'root (有权限)' : '非 root (权限不足)' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="可用后端">{{ (diagnoseReport.available_backends || []).join(', ') || 'none' }}</el-descriptions-item>
          <el-descriptions-item label="总结">
            <span class="danger-text">{{ diagnoseReport.summary }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="建议">
            <pre class="diag-pre">{{ diagnoseReport.recommendation }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>
      <div v-else style="text-align: center; padding: 20px">诊断中...</div>
    </el-dialog>

    <!-- 面板规则弹窗 -->
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑规则' : '添加规则'" width="520px">
      <el-form :model="form" label-width="100px" ref="formRef" :rules="formRules">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="如: 允许 SSH" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="端口规则" value="port" />
            <el-option label="IP 规则" value="ip" />
            <el-option label="DNAT 端口转发" value="dnat" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作" prop="action">
          <el-select v-model="form.action" style="width: 100%">
            <el-option label="允许" value="allow" />
            <el-option label="拒绝" value="deny" />
          </el-select>
        </el-form-item>
        <el-form-item label="协议" v-if="form.type !== 'ip'">
          <el-select v-model="form.protocol" style="width: 100%">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
            <el-option label="全部" value="all" />
          </el-select>
        </el-form-item>
        <el-form-item label="端口" v-if="form.type !== 'ip'">
          <el-input v-model="form.port" :placeholder="form.type === 'dnat' ? '对外端口，如: 25565' : '如: 22, 80, 3306-3308'" />
        </el-form-item>
        <el-form-item label="IP / 网段" v-if="form.type !== 'port'">
          <el-input v-model="form.ip" :placeholder="form.type === 'dnat' ? '目标 IP，如: 192.168.3.50' : '如: 192.168.1.100 或 10.0.0.0/24'" />
        </el-form-item>
        <el-form-item label="目标端口" v-if="form.type === 'dnat'">
          <el-input v-model="form.target_port" placeholder="如: 25565" />
        </el-form-item>
        <el-form-item label="回程 NAT" v-if="form.type === 'dnat'">
          <el-switch v-model="form.masq" />
          <span class="hint">自动 masquerade，外网访问通常需要开启</span>
        </el-form-item>
        <el-form-item label="方向">
          <el-select v-model="form.direction" style="width: 100%">
            <el-option label="入站 (input / prerouting)" value="in" />
            <el-option label="出站 (output)" value="out" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <!-- 内核规则查看 -->
    <el-dialog v-model="sysDialogVisible" title="内核规则（nf_tables 实时读取）" width="900px" top="5vh">
      <div class="sys-toolbar">
        <el-select v-model="sysTable" size="small" style="width: 150px" @change="loadSystemRules">
          <el-option label="全部表" value="" />
          <el-option label="mp_filter" value="mp_filter" />
          <el-option label="mp_nat" value="mp_nat" />
        </el-select>
        <el-select v-model="sysChain" size="small" style="width: 150px" @change="loadSystemRules">
          <el-option label="全部链" value="" />
          <el-option v-for="c in chainOptions" :key="c" :label="c" :value="c" />
        </el-select>
        <el-button size="small" @click="loadSystemRules" :loading="sysLoading">刷新</el-button>
        <span class="hint" style="margin-left: auto">按 handle 删除，不受顺序变化影响</span>
      </div>

      <div v-loading="sysLoading" class="sys-body">
        <div v-if="!sysView.tables.length" class="muted" style="padding: 24px; text-align: center">
          内核里没有任何 ip 表
        </div>
        <div v-for="t in sysView.tables" :key="t.name" class="sys-table">
          <div class="sys-table-head">
            <span class="sys-table-name">table ip {{ t.name }}</span>
            <el-tag v-if="t.panel_owned" type="success" size="small" effect="dark">面板</el-tag>
            <el-tag v-else type="info" size="small">系统</el-tag>
          </div>
          <div v-for="c in t.chains" :key="c.name" class="sys-chain">
            <div class="sys-chain-head">
              <span class="sys-chain-name">chain {{ c.name }}</span>
              <span v-if="c.has_hook" class="sys-meta">type {{ c.type }} · hook {{ c.hook }} · prio {{ c.priority }} · policy {{ c.policy || '—' }}</span>
              <span v-else class="sys-meta">普通链</span>
              <span class="sys-count">{{ c.rules.length }} 条</span>
            </div>
            <el-table v-if="c.rules.length" :data="c.rules" size="small" class="sys-rule-table">
              <el-table-column prop="handle" label="#" width="70" />
              <el-table-column prop="text" label="规则" min-width="300">
                <template #default="{ row }">
                  <span class="mono">{{ row.text || '(空)' }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="verdict" label="判决" width="90">
                <template #default="{ row }">
                  <el-tag v-if="row.verdict === 'accept'" type="success" size="small">accept</el-tag>
                  <el-tag v-else-if="row.verdict === 'drop'" type="danger" size="small">drop</el-tag>
                  <span v-else class="muted">—</span>
                </template>
              </el-table-column>
              <el-table-column label="计数" width="140">
                <template #default="{ row }">
                  <span v-if="row.has_count" class="mono">{{ row.packets }} pkt / {{ formatBytes(row.bytes) }}</span>
                  <span v-else class="muted">—</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="80">
                <template #default="{ row }">
                  <el-button size="small" type="danger" link @click="deleteKernelRule(t, c, row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <div v-else class="muted" style="padding: 6px 0 6px 12px">（空链）</div>
          </div>
        </div>
      </div>
    </el-dialog>

    <!-- 插入内核规则 -->
    <el-dialog v-model="insertDialogVisible" title="插入内核规则" width="600px">
      <el-alert type="info" show-icon :closable="false" style="margin-bottom: 16px">
        <template #default>
          直接写内核 nf_tables。规则写法是自然顺序，如 <code>tcp dport 8080 accept</code>、<code>saddr 10.0.0.5 drop</code>；位置 1 = 链首。
        </template>
      </el-alert>
      <el-form :model="insertForm" label-width="100px">
        <el-form-item label="表">
          <el-select v-model="insertForm.table" style="width: 100%">
            <el-option label="mp_filter (过滤)" value="mp_filter" />
            <el-option label="mp_nat (NAT)" value="mp_nat" />
          </el-select>
        </el-form-item>
        <el-form-item label="链">
          <el-select v-model="insertForm.chain" style="width: 100%">
            <el-option label="input (入站)" value="input" />
            <el-option label="output (出站)" value="output" />
            <el-option label="forward (转发)" value="forward" />
            <el-option label="prerouting (NAT 前置)" value="prerouting" />
            <el-option label="postrouting (NAT 后置)" value="postrouting" />
          </el-select>
        </el-form-item>
        <el-form-item label="插入位置">
          <el-input-number v-model="insertForm.position" :min="1" :max="9999" style="width: 100%" />
        </el-form-item>
        <el-form-item label="规则">
          <el-input v-model="insertForm.spec" type="textarea" :rows="3" placeholder="tcp dport 8080 accept" />
          <div class="hint" style="margin-top: 4px">
            支持：<code>tcp|udp|icmp</code>、<code>dport|sport 80</code> 或 <code>8000-8100</code>、<code>saddr|daddr 10.0.0.0/8</code>，结尾 <code>accept|drop|reject</code>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="insertDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="doInsert">插入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { firewallApi } from '../api'

// 宽屏才启用 fixed 列：sticky 操作列在窄屏会横向覆盖其它列。
const WIDE_MQ = '(min-width: 901px)'
const isWide = ref(typeof window !== 'undefined' ? window.matchMedia(WIDE_MQ).matches : true)
let mql: MediaQueryList | null = null
function syncWide(e: MediaQueryListEvent) { isWide.value = e.matches }

const rules = ref<any[]>([])
const loading = ref(false)
const applying = ref(false)
const actionLoading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<any>(null)
const serviceStatus = ref<any>({ installed: false, running: false, name: '' })
const diagnosing = ref(false)
const diagnoseDialogVisible = ref(false)
const diagnoseReport = ref<any>(null)
const deletedRules = ref<any[]>([])
const deletedLoading = ref(false)
const lockdownLoading = ref(false)
const resetLoading = ref(false)

// 内核规则视图
const sysDialogVisible = ref(false)
const sysLoading = ref(false)
const sysTable = ref('')
const sysChain = ref('')
const sysView = ref<any>({ family: 'ipv4', tables: [] })

// 插入内核规则
const insertDialogVisible = ref(false)
const insertForm = reactive({ table: 'mp_filter', chain: 'input', position: 1, spec: '' })

const isNft = computed(() => serviceStatus.value.backend === 'nft-netlink')

const firewallType = computed(() => {
  const name = serviceStatus.value.backend || serviceStatus.value.name || ''
  if (name === 'nft-netlink') return 'nf_tables（内核直连）'
  if (name === 'nftables') return 'nftables'
  if (name === 'android-iptables') return 'Android iptables'
  if (name === 'iptables') return 'iptables'
  if (name === 'ufw') return 'UFW'
  if (name === 'firewalld') return 'firewalld'
  return name || '未知后端'
})

// 当前内核里所有链名，供筛选下拉用
const chainOptions = computed(() => {
  const set = new Set<string>()
  for (const t of sysView.value.tables || []) {
    for (const c of t.chains || []) set.add(c.name)
  }
  return [...set]
})

const form = reactive({
  id: 0, name: '', type: 'port', action: 'allow', protocol: 'tcp',
  port: '', ip: '', target_port: '', chain: 'PREROUTING', masq: true,
  direction: 'in', enabled: true, note: ''
})

const formRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  action: [{ required: true, message: '请选择动作', trigger: 'change' }]
}

function resetForm() {
  Object.assign(form, {
    id: 0, name: '', type: 'port', action: 'allow', protocol: 'tcp',
    port: '', ip: '', target_port: '', chain: 'PREROUTING', masq: true,
    direction: 'in', enabled: true, note: ''
  })
}

function openDialog(row?: any) {
  if (row) { isEdit.value = true; Object.assign(form, row) }
  else { isEdit.value = false; resetForm() }
  dialogVisible.value = true
}

function formatBytes(n: number) {
  if (!n) return '0 B'
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}

async function checkStatus() {
  try {
    const res: any = await firewallApi.status()
    serviceStatus.value = res.data || { installed: false, running: false, name: '' }
  } catch (e: any) { ElMessage.error(e?.message || '检查服务状态失败') }
}

async function runDiagnose() {
  diagnosing.value = true
  try {
    const res: any = await firewallApi.diagnose()
    diagnoseReport.value = res.data
    diagnoseDialogVisible.value = true
  } catch (e: any) { ElMessage.error(e?.message || '诊断失败') }
  finally { diagnosing.value = false }
}

async function startFirewall() {
  actionLoading.value = true
  try { await firewallApi.start(); ElMessage.success('防火墙已启动'); await checkStatus() }
  catch (e: any) { ElMessage.error(e?.message || '启动失败') }
  finally { actionLoading.value = false }
}

async function stopFirewall() {
  actionLoading.value = true
  try { await firewallApi.stop(); ElMessage.success('防火墙已停止'); await checkStatus() }
  catch (e: any) { ElMessage.error(e?.message || '停止失败') }
  finally { actionLoading.value = false }
}

async function loadRules() {
  loading.value = true
  try { const res: any = await firewallApi.list(); rules.value = res.data || [] }
  catch (e: any) { ElMessage.error(e?.message || '加载失败') }
  finally { loading.value = false }
}

async function loadDeletedRules() {
  deletedLoading.value = true
  try { const res: any = await firewallApi.listDeleted(); deletedRules.value = res.data || [] }
  catch (e: any) { ElMessage.error(e?.message || '加载已删除规则失败') }
  finally { deletedLoading.value = false }
}

async function restoreRule(id: number) {
  try {
    await firewallApi.restoreRule(id)
    ElMessage.success('规则已恢复')
    loadDeletedRules(); loadRules()
  } catch (e: any) { ElMessage.error(e?.message || '恢复失败') }
}

async function restoreAllDeleted() {
  if (!deletedRules.value.length) return
  try {
    await ElMessageBox.confirm(`确定恢复全部 ${deletedRules.value.length} 条已删除规则吗？`, '恢复规则', { type: 'warning' })
    for (const r of [...deletedRules.value]) await firewallApi.restoreRule(r.id)
    ElMessage.success('已全部恢复')
    loadDeletedRules(); loadRules()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e?.message || '恢复失败') }
}

async function clearDeletedRules() {
  if (!deletedRules.value.length) return
  try {
    await ElMessageBox.confirm(`确定永久清空 ${deletedRules.value.length} 条已删除规则吗？清空后无法恢复！`, '清空回收站', { type: 'error', confirmButtonText: '清空', cancelButtonText: '取消' })
    const res: any = await firewallApi.clearDeletedRules()
    ElMessage.success(res.message || '已清空')
    loadDeletedRules(); loadRules()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e?.message || '清空失败') }
}

async function saveRule() {
  await formRef.value?.validate()
  try {
    if (isEdit.value) { await firewallApi.update(form.id, { ...form }); ElMessage.success('更新成功') }
    else { await firewallApi.create({ ...form }); ElMessage.success('添加成功') }
    dialogVisible.value = false; loadRules()
  } catch (e: any) { ElMessage.error(e?.message || '保存失败') }
}

async function handleDelete(id: number) {
  try {
    await ElMessageBox.confirm('确定要删除这条面板规则吗？', '提示', { type: 'warning' })
    await firewallApi.delete(id); ElMessage.success('删除成功'); loadRules()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e?.message || '删除失败') }
}

// 应用面板规则：把库里的规则整体重建进内核
async function applyRules(quiet = false) {
  applying.value = true
  try {
    const res: any = await firewallApi.apply()
    if (!quiet) ElMessage.success(res.message || '规则已应用')
    await checkStatus()
    if (sysDialogVisible.value) await loadSystemRules()
    return true
  } catch (e: any) {
    ElMessage.error(e?.message || '应用失败')
    return false
  } finally { applying.value = false }
}

function applyPanelRules() { applyRules() }

// === 内核规则 ===
async function openSystemRules() {
  sysDialogVisible.value = true
  await loadSystemRules()
}

async function loadSystemRules() {
  sysLoading.value = true
  try {
    const res: any = await firewallApi.systemRules(sysChain.value || undefined, sysTable.value || undefined)
    sysView.value = res.data || { family: 'ipv4', tables: [] }
  } catch (e: any) {
    ElMessage.error(e?.message || '读取内核规则失败')
    sysView.value = { family: 'ipv4', tables: [] }
  } finally { sysLoading.value = false }
}

async function deleteKernelRule(t: any, c: any, row: any) {
  try {
    await ElMessageBox.confirm(
      `删除 ${t.name}/${c.name} 中 #${row.handle} 的规则？\n${row.text}`,
      '删除内核规则',
      { type: 'warning' }
    )
    await firewallApi.deleteLiveRule(c.name, row.handle, undefined, t.name)
    ElMessage.success('已删除')
    await loadSystemRules()
    await checkStatus()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e?.message || '删除失败') }
}

// === 插入内核规则 ===
function openInsert() {
  insertForm.table = 'mp_filter'
  insertForm.chain = 'input'
  insertForm.position = 1
  insertForm.spec = ''
  insertDialogVisible.value = true
}

async function doInsert() {
  const spec = insertForm.spec.trim()
  if (!spec) { ElMessage.warning('请输入规则内容'); return }
  try {
    await firewallApi.insertRule({
      chain: insertForm.chain,
      position: insertForm.position,
      spec: spec.split(/\s+/),
      table: insertForm.table === 'mp_nat' ? 'nat' : undefined
    })
    ElMessage.success('规则已插入')
    insertDialogVisible.value = false
    if (sysDialogVisible.value) await loadSystemRules()
    await checkStatus()
  } catch (e: any) { ElMessage.error(e?.message || '插入失败') }
}

// === 一键内网模式 ===
async function lockdown() {
  try {
    await ElMessageBox.confirm(
      isNft
        ? '将以 nft 重建 mp_filter：只允许已建立连接、回环与内网段，其余入站全部丢弃。确定继续？'
        : '此操作将清空 INPUT 链并设置为只允许内网访问，外网将被全部拒绝。确定继续？',
      '一键内网模式',
      { type: 'warning', confirmButtonText: '确认开启', cancelButtonText: '取消' }
    )
    lockdownLoading.value = true
    const res: any = await firewallApi.lockdown()
    ElMessage.success('内网模式已开启')
    if (res.message) ElMessageBox.alert(res.message, '执行结果', { type: 'success' })
    await checkStatus()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '操作失败')
  } finally { lockdownLoading.value = false }
}

// === 清空面板表 ===
async function resetAllRules() {
  try {
    await ElMessageBox.confirm(
      isNft
        ? '将删除面板的两张表（mp_filter / mp_nat），内核里面板规则全部清空。系统自带的 oem_* 等表不受影响。确定继续？'
        : '此操作将清空 INPUT、OUTPUT、FORWARD 三条链的所有规则。确定继续？',
      '清空面板规则',
      { type: 'error', confirmButtonText: '确认清空', cancelButtonText: '取消' }
    )
    resetLoading.value = true
    await firewallApi.stop()
    ElMessage.success('面板规则已清空')
    await checkStatus()
    if (sysDialogVisible.value) await loadSystemRules()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '清空失败')
  } finally { resetLoading.value = false }
}

onMounted(async () => {
  mql = window.matchMedia(WIDE_MQ)
  mql.addEventListener('change', syncWide)
  await checkStatus()
  if (serviceStatus.value.installed) {
    loadRules()
    loadDeletedRules()
  }
})

onUnmounted(() => { mql?.removeEventListener('change', syncWide) })
</script>

<style scoped>
.page-container { padding: 20px; }
.page-header { margin-bottom: 20px; }
.page-title { font-size: 1.5rem; font-weight: 600; margin: 0; display: flex; align-items: center; gap: 8px; }
.icon { font-size: 1.3rem; }
.muted { color: #909399; font-size: 13px; }
.hint { margin-left: 8px; color: #909399; font-size: 12px; }
.mono { font-family: 'Courier New', monospace; font-size: 12px; }
.danger-text { color: #f56c6c; font-weight: 600; }
.diag-pre { margin: 0; white-space: pre-wrap; color: #666; font-size: 13px; font-family: monospace; }

.quick-actions { display: flex; gap: 8px; flex-wrap: wrap; }

/* 状态条 */
.svc-bar { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.svc-bar-left { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; min-width: 0; }
.svc-bar-right { display: flex; gap: 8px; flex-wrap: wrap; }
.backend-name { font-weight: 600; }

/* nft 统计行 */
.stat-row { display: flex; gap: 24px; flex-wrap: wrap; margin-top: 14px; padding-top: 14px; border-top: 1px solid #f0f0f0; }
.stat { display: flex; flex-direction: column; gap: 2px; }
.stat-label { font-size: 12px; color: #909399; }
.stat-value { font-size: 20px; font-weight: 600; font-variant-numeric: tabular-nums; }
.stat-value.small { font-size: 13px; font-weight: 500; }

.warn-line { margin-top: 12px; font-size: 13px; color: #e6a23c; display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }

/* 分区标题条 */
.section-bar { display: flex; justify-content: space-between; align-items: center; gap: 10px; flex-wrap: wrap; margin-bottom: 12px; }
.section-title { font-weight: 600; }
.trash-actions { display: flex; gap: 8px; flex-wrap: wrap; }

/* 顶部告警条 */
.alert-bar { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; width: 100%; }

/* 内核规则面板 */
.sys-toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 14px; }
.sys-body { max-height: 60vh; overflow: auto; }
.sys-table { margin-bottom: 18px; border: 1px solid #ebeef5; border-radius: 8px; overflow: hidden; }
.sys-table-head {
  display: flex; align-items: center; gap: 10px; padding: 10px 14px;
  background: #fafafa; border-bottom: 1px solid #ebeef5;
}
.sys-table-name { font-family: 'Courier New', monospace; font-weight: 700; font-size: 14px; }
.sys-chain { border-top: 1px solid #f5f5f5; }
.sys-chain:first-of-type { border-top: none; }
.sys-chain-head { display: flex; align-items: center; gap: 12px; padding: 8px 14px; flex-wrap: wrap; }
.sys-chain-name { font-family: 'Courier New', monospace; font-weight: 600; font-size: 13px; }
.sys-meta { font-size: 12px; color: #909399; font-family: 'Courier New', monospace; }
.sys-count { margin-left: auto; font-size: 12px; color: #c0c4cc; }
.sys-rule-table { padding: 0 6px 6px; }

/* 表格横向滚动容器 */
.table-wrap { width: 100%; max-width: 100%; min-width: 0; overflow-x: auto; overflow-y: hidden; -webkit-overflow-scrolling: touch; }
.table-wrap :deep(.el-table) { width: 100%; min-width: 0; max-width: 100%; }

.row-actions { display: flex; flex-wrap: wrap; gap: 6px; }
.row-actions :deep(.el-button) { margin-left: 0; }

@media (max-width: 900px) {
  .quick-actions :deep(.el-button) { flex: 1 1 45%; margin-left: 0; }
  .row-actions :deep(.el-button) { font-size: 12px; padding: 5px 8px; }
  .stat-row { gap: 16px; }
}

@media (max-width: 480px) {
  .page-container { padding: 14px; }
  .svc-bar { flex-direction: column; align-items: stretch; }
  .svc-bar-right :deep(.el-button) { flex: 1 1 45%; margin-left: 0; }
  .section-bar { flex-direction: column; align-items: stretch; }
  .trash-actions :deep(.el-button) { flex: 1 1 30%; margin-left: 0; }
  .quick-actions :deep(.el-button) { flex: 1 1 100%; margin-left: 0; }
  .row-actions :deep(.el-button) { flex: 1 1 45%; }
}
</style>
