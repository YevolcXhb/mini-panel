<template>
  <div class="page-container">
    <div class="page-header">
      <h2 class="page-title">
        <span class="icon">🛡️</span> 防火墙
      </h2>
    </div>

    <el-alert v-if="!serviceStatus.installed" type="warning" show-icon :closable="false" style="margin-bottom: 16px">
      <template #title>
        <div style="display: flex; align-items: center; justify-content: space-between; width: 100%">
          <span>{{ serviceStatus.message || '防火墙功能不可用' }}</span>
          <el-button size="small" type="primary" @click="runDiagnose" :loading="diagnosing">一键诊断</el-button>
        </div>
      </template>
      <template #default>
        <div style="margin-top: 8px">
          <pre v-if="serviceStatus.diagnosis" style="margin: 0; white-space: pre-wrap; color: #666; font-size: 13px; font-family: monospace">{{ serviceStatus.diagnosis }}</pre>
          <p v-else style="margin: 0; color: #666; font-size: 13px">点击"一键诊断"获取详细原因</p>
        </div>
      </template>
    </el-alert>

    <el-alert v-else-if="serviceStatus.kernel_warning" type="warning" show-icon :closable="false" style="margin-bottom: 16px">
      <template #title>内核模块警告</template>
      <template #default>
        <div style="margin-top: 8px; color: #666; font-size: 13px">{{ serviceStatus.kernel_warning }}</div>
      </template>
    </el-alert>

    <template v-else>
      <!-- 状态卡片 -->
      <el-card style="margin-bottom: 16px">
        <div style="display: flex; justify-content: space-between; align-items: center">
          <div style="display: flex; align-items: center; gap: 12px">
            <el-tag :type="serviceStatus.running ? 'success' : 'danger'" size="large">
              {{ serviceStatus.running ? '运行中' : '已停止' }}
            </el-tag>
            <span>{{ firewallType }} {{ serviceStatus.version ? 'v' + serviceStatus.version : '' }}</span>
            <el-tag v-if="serviceStatus.ipv6_supported" type="success" size="small">IPv6 已支持</el-tag>
            <el-tag v-else type="info" size="small">IPv6 不可用</el-tag>
          </div>
          <div style="display: flex; gap: 8px; flex-wrap: wrap">
            <el-button size="small" type="primary" @click="startFirewall" v-if="!serviceStatus.running" :loading="actionLoading">启动</el-button>
            <el-button size="small" type="warning" @click="stopFirewall" v-if="serviceStatus.running" :loading="actionLoading">停止</el-button>
            <el-button size="small" @click="checkStatus">刷新</el-button>
          </div>
        </div>
      </el-card>

      <!-- 快捷操作栏 -->
      <el-card style="margin-bottom: 16px">
        <div class="quick-actions">
          <el-button type="primary" @click="showLiveRules" :loading="liveLoading">📋 查看系统规则</el-button>
          <el-button type="success" @click="quickOpenPort">🔓 快速开放端口</el-button>
          <el-button type="danger" @click="quickBanIP">🚫 快速封禁IP</el-button>
          <el-button type="warning" @click="insertDialogVisible = true">📌 插入规则</el-button>
          <el-button type="info" @click="lockdown" :loading="lockdownLoading">🔒 一键内网模式</el-button>
          <el-button type="danger" plain @click="resetAllRules" :loading="resetLoading">🗑️ 重置（清空所有规则）</el-button>
          <el-button @click="applyRules" :loading="applying">✅ 应用面板规则</el-button>
          <el-button type="primary" @click="openDialog()">➕ 添加面板规则</el-button>
        </div>
      </el-card>

      <el-alert
        v-if="serviceStatus.ipv6_supported"
        type="success"
        show-icon
        :closable="false"
        style="margin-bottom: 12px"
      >
        <template #title>端口规则会同时应用到 IPv4 (iptables) 和 IPv6 (ip6tables)；IP 规则按地址类型自动匹配对应协议族</template>
      </el-alert>

      <!-- 面板规则表格 -->
      <el-table :data="rules" style="width: 100%" v-loading="loading">
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="type" label="类型" width="80">
          <template #default="{ row }">
            <el-tag :type="row.type === 'port' ? 'primary' : row.type === 'dnat' ? 'success' : 'warning'">{{ row.type === 'port' ? '端口' : row.type === 'dnat' ? '转发' : 'IP' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="action" label="动作" width="80">
          <template #default="{ row }">
            <el-tag :type="row.action === 'allow' ? 'success' : 'danger'">{{ row.action === 'allow' ? '允许' : '拒绝' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="protocol" label="协议" width="80" />
        <el-table-column prop="port" label="端口" width="100" />
        <el-table-column prop="ip" label="IP" width="120" />
        <el-table-column prop="target_port" label="目标端口" width="100">
          <template #default="{ row }">
            <span v-if="row.type === 'dnat'">{{ row.target_port }}</span>
            <span v-else style="color: #c0c4cc">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="direction" label="方向" width="80">
          <template #default="{ row }">
            {{ row.direction === 'in' ? '入站' : '出站' }}
          </template>
        </el-table-column>
        <el-table-column prop="enabled" label="状态" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.enabled" type="success">启用</el-tag>
            <el-tag v-else type="info">停用</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="handleDelete(row.id)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 回收站：已删除的面板规则 -->
      <el-card style="margin-top: 16px">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px">
          <span style="font-weight: 600">🗑️ 已删除规则（可恢复）</span>
          <div style="display: flex; gap: 8px">
            <el-button size="small" @click="loadDeletedRules" :loading="deletedLoading">刷新</el-button>
            <el-button size="small" type="primary" @click="restoreAllDeleted" :disabled="!deletedRules.length">全部恢复</el-button>
          </div>
        </div>
        <el-table :data="deletedRules" size="small" v-loading="deletedLoading" empty-text="没有已删除的规则">
          <el-table-column prop="name" label="名称" min-width="120" />
          <el-table-column prop="type" label="类型" width="80">
            <template #default="{ row }">
              <el-tag :type="row.type === 'port' ? 'primary' : row.type === 'dnat' ? 'success' : 'warning'" size="small">{{ row.type === 'port' ? '端口' : row.type === 'dnat' ? '转发' : 'IP' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="action" label="动作" width="80">
            <template #default="{ row }">
              <el-tag :type="row.action === 'allow' ? 'success' : 'danger'" size="small">{{ row.action === 'allow' ? '允许' : '拒绝' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="protocol" label="协议" width="80" />
          <el-table-column prop="port" label="端口" width="100" />
          <el-table-column prop="ip" label="IP" width="120" />
          <el-table-column prop="direction" label="方向" width="80">
            <template #default="{ row }">{{ row.direction === 'in' ? '入站' : '出站' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="primary" @click="restoreRule(row.id)">恢复</el-button>
            </template>
          </el-table-column>
        </el-table>
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
          <el-descriptions-item label="容器环境">
            <el-tag :type="diagnoseReport.in_container ? 'warning' : 'success'">{{ diagnoseReport.in_container ? `在容器中 (${diagnoseReport.container_type || 'unknown'})` : '物理机/虚拟机' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="可用后端">{{ (diagnoseReport.available_backends || []).join(', ') || 'none' }}</el-descriptions-item>
          <el-descriptions-item label="IPv6 支持">
            <el-tag :type="diagnoseReport.ipv6_supported ? 'success' : 'danger'">{{ diagnoseReport.ipv6_supported ? '可用' : '不可用' }}</el-tag>
            <span v-if="diagnoseReport.ipv6_error" style="margin-left: 8px; color: #f56c6c; font-size: 12px">{{ diagnoseReport.ipv6_error }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="工具安装情况">
            <div style="display: flex; gap: 8px; flex-wrap: wrap">
              <el-tag v-for="(installed, tool) in (diagnoseReport.tools_installed || {})" :key="tool" :type="installed ? 'success' : 'info'">{{ tool }}: {{ installed ? '已装' : '未装' }}</el-tag>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="内核模块">
            <div v-if="diagnoseReport.kernel_modules" style="display: flex; gap: 8px; flex-wrap: wrap">
              <el-tag v-for="(status, mod) in diagnoseReport.kernel_modules" :key="mod" :type="status === 'ok' ? 'success' : 'danger'">{{ mod }}: {{ status }}</el-tag>
            </div>
            <span v-else style="color: #c0c4cc">无</span>
          </el-descriptions-item>
          <el-descriptions-item label="总结">
            <span style="color: #f56c6c; font-weight: bold">{{ diagnoseReport.summary }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="建议">
            <pre style="margin: 0; white-space: pre-wrap; color: #666; font-size: 13px">{{ diagnoseReport.recommendation }}</pre>
          </el-descriptions-item>
        </el-descriptions>
      </div>
      <div v-else style="text-align: center; padding: 20px">诊断中...</div>
    </el-dialog>

    <!-- 面板规则添加/编辑弹窗 -->
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑规则' : '添加规则'" width="500px">
      <el-form :model="form" label-width="100px" ref="formRef" :rules="formRules">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="如: 允许 SSH" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="form.type" placeholder="选择类型" style="width: 100%">
            <el-option label="端口规则" value="port" />
            <el-option label="IP 规则" value="ip" />
            <el-option label="DNAT 端口转发" value="dnat" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作" prop="action">
          <el-select v-model="form.action" placeholder="选择动作" style="width: 100%">
            <el-option label="允许" value="allow" />
            <el-option label="拒绝" value="deny" />
          </el-select>
        </el-form-item>
        <el-form-item label="协议" v-if="form.type === 'port' || form.type === 'dnat'">
          <el-select v-model="form.protocol" placeholder="选择协议" style="width: 100%">
            <el-option label="TCP" value="tcp" />
            <el-option label="UDP" value="udp" />
            <el-option label="全部" value="all" />
          </el-select>
        </el-form-item>
        <el-form-item label="端口" v-if="form.type === 'port' || form.type === 'dnat'">
          <el-input v-model="form.port" :placeholder="form.type === 'dnat' ? '公网端口，如: 25565' : '如: 22, 80, 3306-3308'" />
        </el-form-item>
        <el-form-item label="IP 地址" v-if="form.type === 'ip' || form.type === 'dnat'">
          <el-input v-model="form.ip" :placeholder="form.type === 'dnat' ? '目标 IP，如: 192.168.3.50' : '如: 192.168.1.100 或 10.0.0.0/24'" />
        </el-form-item>
        <el-form-item label="目标端口" v-if="form.type === 'dnat'">
          <el-input v-model="form.target_port" placeholder="如: 25565" />
        </el-form-item>
        <el-form-item label="转发链" v-if="form.type === 'dnat'">
          <el-select v-model="form.chain" style="width: 100%">
            <el-option label="PREROUTING (标准)" value="PREROUTING" />
            <el-option label="oem_nat_pre (Android)" value="oem_nat_pre" />
          </el-select>
        </el-form-item>
        <el-form-item label="回程NAT" v-if="form.type === 'dnat'">
          <el-switch v-model="form.masq" />
          <span style="margin-left: 8px; color: #909399; font-size: 12px">自动添加 MASQUERADE，外网访问通常需要开启</span>
        </el-form-item>
        <el-form-item label="方向">
          <el-select v-model="form.direction" placeholder="选择方向" style="width: 100%">
            <el-option label="入站" value="in" />
            <el-option label="出站" value="out" />
          </el-select>
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="2" placeholder="备注信息" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <!-- 系统实时规则弹窗 -->
    <el-dialog v-model="liveDialogVisible" title="系统实时规则 (iptables -L)" width="850px" top="5vh">
      <div style="margin-bottom: 12px; display: flex; gap: 8px; align-items: center">
        <el-select v-model="liveChain" placeholder="选择链" style="width: 150px" @change="showLiveRules">
          <el-option label="全部链" value="" />
          <el-option label="INPUT (入站)" value="INPUT" />
          <el-option label="OUTPUT (出站)" value="OUTPUT" />
          <el-option label="FORWARD (转发)" value="FORWARD" />
        </el-select>
        <el-radio-group v-model="liveFamily" size="small" @change="showLiveRules">
          <el-radio-button label="">全部</el-radio-button>
          <el-radio-button label="4">IPv4</el-radio-button>
          <el-radio-button label="6">IPv6</el-radio-button>
        </el-radio-group>
        <el-select v-model="liveTable" placeholder="选择表" style="width: 130px" @change="showLiveRules">
          <el-option label="filter 表" value="" />
          <el-option label="nat 表" value="nat" />
        </el-select>
        <el-button size="small" @click="showLiveRules" :loading="liveLoading">刷新</el-button>
        <span style="color: #999; font-size: 12px; margin-left: auto">删除规则后行号会变化，建议从大到小删</span>
      </div>
      <div style="background: #1e1e1e; color: #d4d4d4; padding: 16px; border-radius: 8px; max-height: 500px; overflow: auto; font-family: 'Courier New', monospace; font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-all">{{ liveRulesText || '加载中...' }}</div>
    </el-dialog>

    <!-- 插入规则弹窗 -->
    <el-dialog v-model="insertDialogVisible" title="插入规则到指定位置 (-I)" width="600px">
      <el-alert type="info" show-icon :closable="false" style="margin-bottom: 16px">
        <template #default>直接操作系统的 iptables -I 命令，将规则插入到指定链的指定位置。位置 1 = 最前面（优先级最高）。</template>
      </el-alert>
      <el-form :model="insertForm" label-width="100px">
        <el-form-item label="链">
          <el-select v-model="insertForm.chain" style="width: 100%">
            <el-option label="INPUT (入站)" value="INPUT" />
            <el-option label="OUTPUT (出站)" value="OUTPUT" />
            <el-option label="FORWARD (转发)" value="FORWARD" />
          </el-select>
        </el-form-item>
        <el-form-item label="协议族">
          <el-select v-model="insertForm.family" style="width: 100%">
            <el-option label="IPv4 (iptables)" value="4" />
            <el-option label="IPv6 (ip6tables)" value="6" />
          </el-select>
        </el-form-item>
        <el-form-item label="插入位置">
          <el-input-number v-model="insertForm.position" :min="1" :max="999" style="width: 100%" />
        </el-form-item>
        <el-form-item label="规则参数">
          <el-input v-model="insertForm.spec" type="textarea" :rows="3" placeholder='如: -p tcp --dport 80 -j ACCEPT' />
          <div style="color: #999; font-size: 12px; margin-top: 4px">完整 iptables 参数（不含 -I 和链名），例如：<code>-p tcp --dport 25565 -j ACCEPT</code></div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="insertDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="doInsert">插入</el-button>
      </template>
    </el-dialog>

    <!-- 快速开放端口弹窗 -->
    <el-dialog v-model="quickPortDialogVisible" title="快速开放端口" width="420px">
      <el-form :model="quickPortForm" label-width="80px">
        <el-form-item label="端口">
          <el-input v-model="quickPortForm.port" placeholder="如: 25565" />
        </el-form-item>
        <el-form-item label="协议">
          <el-radio-group v-model="quickPortForm.protocol">
            <el-radio value="tcp">TCP</el-radio>
            <el-radio value="udp">UDP</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="链">
          <el-radio-group v-model="quickPortForm.chain">
            <el-radio value="INPUT">INPUT (入站)</el-radio>
            <el-radio value="OUTPUT">OUTPUT (出站)</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="quickPortDialogVisible = false">取消</el-button>
        <el-button type="success" @click="doQuickOpenPort" :loading="quickLoading">立即开放</el-button>
      </template>
    </el-dialog>

    <!-- 快速封禁IP弹窗 -->
    <el-dialog v-model="quickIPDialogVisible" title="快速封禁IP" width="420px">
      <el-form :model="quickIPForm" label-width="80px">
        <el-form-item label="IP地址">
          <el-input v-model="quickIPForm.ip" placeholder="如: 192.168.1.100 或 10.0.0.0/8" />
        </el-form-item>
        <el-form-item label="动作">
          <el-radio-group v-model="quickIPForm.action">
            <el-radio value="DROP">DROP (丢弃，对方卡住)</el-radio>
            <el-radio value="REJECT">REJECT (拒绝，对方收到拒绝)</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="quickIPDialogVisible = false">取消</el-button>
        <el-button type="danger" @click="doQuickBanIP" :loading="quickLoading">立即封禁</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { firewallApi } from '../api'

const rules = ref<any[]>([])
const loading = ref(false)
const applying = ref(false)
const actionLoading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref<any>(null)
const serviceStatus = ref<any>({ installed: false, running: false, version: '', name: 'firewalld' })
const diagnosing = ref(false)
const diagnoseDialogVisible = ref(false)
const diagnoseReport = ref<any>(null)

// 实时规则
const liveDialogVisible = ref(false)
const liveLoading = ref(false)
const liveRulesText = ref('')
const liveChain = ref('INPUT')
const liveFamily = ref('')
const liveTable = ref('')

// 插入规则
const insertDialogVisible = ref(false)
const insertForm = reactive({ chain: 'INPUT', position: 1, spec: '', family: '4' })

// 快捷操作
const quickPortDialogVisible = ref(false)
const quickIPDialogVisible = ref(false)
const quickLoading = ref(false)
const quickPortForm = reactive({ port: '', protocol: 'tcp', chain: 'INPUT' })
const quickIPForm = reactive({ ip: '', action: 'DROP' })
const lockdownLoading = ref(false)
const resetLoading = ref(false)
const deletedRules = ref<any[]>([])
const deletedLoading = ref(false)

const firewallType = computed(() => {
  const name = serviceStatus.value.backend || serviceStatus.value.name || 'firewalld'
  if (name === 'ufw') return 'UFW'
  if (name === 'nftables') return 'nftables'
  if (name === 'android-iptables') return 'Android iptables'
  if (name === 'iptables') return 'iptables'
  return 'firewalld'
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
  Object.assign(form, { id: 0, name: '', type: 'port', action: 'allow', protocol: 'tcp', port: '', ip: '', target_port: '', chain: 'PREROUTING', masq: true, direction: 'in', enabled: true, note: '' })
}

function openDialog(row?: any) {
  if (row) { isEdit.value = true; Object.assign(form, row) }
  else { isEdit.value = false; resetForm() }
  dialogVisible.value = true
}

async function checkStatus() {
  try {
    const res: any = await firewallApi.status()
    serviceStatus.value = res.data || { installed: false, running: false, name: 'firewalld' }
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
  try { await firewallApi.start(); ElMessage.success('防火墙启动成功'); await checkStatus() }
  catch (e: any) { ElMessage.error(e?.message || '启动失败') }
  finally { actionLoading.value = false }
}

async function stopFirewall() {
  actionLoading.value = true
  try { await firewallApi.stop(); ElMessage.success('防火墙停止成功'); await checkStatus() }
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
    loadDeletedRules()
    loadRules()
  } catch (e: any) { ElMessage.error(e?.message || '恢复失败') }
}

async function restoreAllDeleted() {
  if (!deletedRules.value.length) return
  try {
    await ElMessageBox.confirm(`确定恢复全部 ${deletedRules.value.length} 条已删除规则吗？`, '恢复规则', { type: 'warning' })
    for (const r of [...deletedRules.value]) {
      await firewallApi.restoreRule(r.id)
    }
    ElMessage.success('已全部恢复')
    loadDeletedRules()
    loadRules()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error(e?.message || '恢复失败') }
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

async function applyRules() {
  applying.value = true
  try { const res: any = await firewallApi.apply(); ElMessage.success(res.message || '规则已应用') }
  catch (e: any) { ElMessage.error(e?.message || '应用失败') }
  finally { applying.value = false }
}

// === 实时规则查看 ===
async function showLiveRules() {
  liveLoading.value = true
  liveDialogVisible.value = true
  try {
    const res: any = await firewallApi.liveRules(liveChain.value, liveFamily.value, liveTable.value)
    liveRulesText.value = res.data || '(空)'
  } catch (e: any) {
    liveRulesText.value = '加载失败: ' + (e?.message || '未知错误')
  } finally { liveLoading.value = false }
}

// === 插入规则 ===
async function doInsert() {
  if (!insertForm.spec.trim()) { ElMessage.warning('请输入规则参数'); return }
  const spec = insertForm.spec.trim().split(/\s+/)
  try {
    await firewallApi.insertRule({ chain: insertForm.chain, position: insertForm.position, spec, family: insertForm.family })
    ElMessage.success('规则已插入')
    insertDialogVisible.value = false
    showLiveRules()
  } catch (e: any) { ElMessage.error(e?.message || '插入失败') }
}

// === 快速开放端口 ===
function quickOpenPort() {
  quickPortForm.port = ''; quickPortForm.protocol = 'tcp'; quickPortForm.chain = 'INPUT'
  quickPortDialogVisible.value = true
}

async function doQuickOpenPort() {
  if (!quickPortForm.port.trim()) { ElMessage.warning('请输入端口'); return }
  quickLoading.value = true
  const spec = ['-p', quickPortForm.protocol, '--dport', quickPortForm.port.trim(), '-j', 'ACCEPT']
  try {
    await firewallApi.insertRule({ chain: quickPortForm.chain, position: 1, spec })
    ElMessage.success(`端口 ${quickPortForm.port} 已开放 (${quickPortForm.protocol})`)
    quickPortDialogVisible.value = false
  } catch (e: any) { ElMessage.error(e?.message || '开放失败') }
  finally { quickLoading.value = false }
}

// === 快速封禁IP ===
function quickBanIP() {
  quickIPForm.ip = ''; quickIPForm.action = 'DROP'
  quickIPDialogVisible.value = true
}

async function doQuickBanIP() {
  if (!quickIPForm.ip.trim()) { ElMessage.warning('请输入IP地址'); return }
  quickLoading.value = true
  const spec = ['-s', quickIPForm.ip.trim(), '-j', quickIPForm.action]
  try {
    await firewallApi.insertRule({ chain: 'INPUT', position: 1, spec })
    ElMessage.success(`IP ${quickIPForm.ip} 已封禁 (${quickIPForm.action})`)
    quickIPDialogVisible.value = false
  } catch (e: any) { ElMessage.error(e?.message || '封禁失败') }
  finally { quickLoading.value = false }
}

// === 一键内网模式 ===
async function lockdown() {
  try {
    await ElMessageBox.confirm(
      '此操作将清空 INPUT 链并设置为只允许内网访问，外网将被全部拒绝。确定继续？',
      '一键内网模式',
      { type: 'warning', confirmButtonText: '确认开启', cancelButtonText: '取消' }
    )
    lockdownLoading.value = true
    const res: any = await firewallApi.lockdown()
    ElMessage.success('内网模式已开启')
    if (res.message) ElMessageBox.alert(res.message, '执行结果', { type: 'success' })
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '操作失败')
  } finally { lockdownLoading.value = false }
}

// === 重置（清空所有规则） ===
async function resetAllRules() {
  try {
    await ElMessageBox.confirm(
      '此操作将清空 INPUT、OUTPUT、FORWARD 三条链的所有规则，恢复到出厂状态。确定继续？',
      '重置防火墙',
      { type: 'error', confirmButtonText: '确认重置', cancelButtonText: '取消' }
    )
    resetLoading.value = true
    // 直接调用 stop 接口（后端执行 -F 清空所有链）
    await firewallApi.stop()
    ElMessage.success('所有规则已清空，防火墙已重置')
    await checkStatus()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e?.message || '重置失败')
  } finally { resetLoading.value = false }
}

onMounted(async () => {
  await checkStatus()
  if (serviceStatus.value.installed) {
    loadRules()
    loadDeletedRules()
  }
})
</script>

<style scoped>
.page-container { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.page-title { font-size: 1.5rem; font-weight: 600; margin: 0; display: flex; align-items: center; gap: 8px; }
.icon { font-size: 1.3rem; }
.quick-actions { display: flex; gap: 8px; flex-wrap: wrap; }
</style>
