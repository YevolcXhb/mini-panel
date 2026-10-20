<template>
  <div>
    <h2 class="page-title">📈 监控中心</h2>

	    <div class="gauge-grid">
	      <div class="info-card gauge-card">
	        <div class="info-label">CPU 使用率</div>
	        <v-chart class="gauge-chart" :option="cpuGaugeOption" autoresize />
	      </div>
	      <div class="info-card gauge-card">
	        <div class="info-label">内存使用率</div>
	        <v-chart class="gauge-chart" :option="memGaugeOption" autoresize />
	      </div>
	      <div class="info-card gauge-card">
	        <div class="info-label">磁盘使用率</div>
	        <v-chart class="gauge-chart" :option="diskGaugeOption" autoresize />
	      </div>
	      <div class="info-card gauge-card">
	        <div class="info-label">网络速率</div>
	        <div style="text-align: center; padding: 10px 0">
	          <div style="font-size: 13px; color: #888; margin-bottom: 4px">↓ 下载</div>
	          <div style="font-size: 20px; font-weight: 600; color: #4f8cff">{{ formatBytes(realtimeMetrics?.net_recv_speed) }}/s</div>
	          <div style="font-size: 13px; color: #888; margin: 8px 0 4px">↑ 上传</div>
	          <div style="font-size: 20px; font-weight: 600; color: #00d26a">{{ formatBytes(realtimeMetrics?.net_sent_speed) }}/s</div>
	        </div>
	      </div>
	      <div class="info-card gauge-card battery-card">
	        <div class="info-label">🔋 电池状态</div>
	        <template v-if="battery?.present">
	          <div class="battery-pct" :style="{ color: batteryColor }">{{ battery.capacity }}<span class="battery-pct-sign">%</span></div>
	          <div class="battery-row">
	            <span>{{ battery.status || '-' }}</span>
	            <span class="power-cell" :title="powerHint">
	              <template v-if="battery.power_source !== 'none' && battery.power_w > 0">
	                {{ battery.power_w.toFixed(2) }} W
	                <span class="power-tag">{{ powerTag }}</span>
	              </template>
	              <template v-else>— W</template>
	            </span>
	          </div>
	          <div class="battery-row"><span>{{ (battery.voltage_v || 0).toFixed(2) }} V</span><span>{{ Math.abs(battery.current_ma || 0).toFixed(1) }} mA</span></div>
	          <div class="battery-row"><span>{{ (battery.temp_c || 0).toFixed(1) }} ℃</span><span v-if="battery.health">❤ {{ battery.health }}</span></div>
	          <div v-if="battery.charge_full_mah > 0" class="battery-row">
	            <span>{{ battery.charge_now_mah.toFixed(0) }} / {{ battery.charge_full_mah.toFixed(0) }} mAh</span>
	            <span v-if="battery.cycle_count > 0">{{ battery.cycle_count }} 循环</span>
	          </div>
	          <div class="progress-bar battery-bar">
	            <div class="progress-fill" :style="{ width: battery.capacity + '%', background: batteryColor }"></div>
	          </div>
	          <div v-if="battery.power_source === 'none'" class="power-note">{{ powerHint }}</div>
	        </template>
	        <div v-else class="battery-na">
	          未检测到电池
	          <div class="diag-link" @click="showDiag">查看诊断 →</div>
	        </div>
	      </div>
	      <div class="info-card gauge-card battery-card thermal-card">
	        <div class="info-label">
	          🌡 温度传感器
	          <span v-if="thermal.length" class="thermal-count">{{ thermal.length }}</span>
	        </div>
	        <template v-if="thermal.length">
	          <div v-for="z in thermal" :key="z.index + z.type" class="thermal-row">
	            <span class="thermal-type" :title="z.type">{{ z.type || ('zone' + z.index) }}</span>
	            <span class="thermal-temp" :style="{ color: tempColor(z.temp_c) }">{{ (z.temp_c || 0).toFixed(1) }} ℃</span>
	          </div>
	        </template>
	        <div v-else class="battery-na">
	          未检测到有效温度传感器
	          <div class="diag-link" @click="showDiag">查看诊断 →</div>
	        </div>
	      </div>
	    </div>

	    <div style="margin-bottom:16px;display:flex;gap:8px;align-items:center" class="range-bar">
      <el-radio-group v-model="timeRange" size="small" @change="loadData">
        <el-radio-button label="60">1小时</el-radio-button>
        <el-radio-button label="360">6小时</el-radio-button>
        <el-radio-button label="720">12小时</el-radio-button>
        <el-radio-button label="1440">24小时</el-radio-button>
      </el-radio-group>
    </div>

    <div class="chart-grid">
      <div class="info-card" style="padding:16px">
        <div class="info-label" style="margin-bottom:8px">CPU 使用率趋势</div>
        <v-chart class="chart" :option="cpuOption" autoresize />
      </div>
      <div class="info-card" style="padding:16px">
        <div class="info-label" style="margin-bottom:8px">内存使用趋势</div>
        <v-chart class="chart" :option="memOption" autoresize />
      </div>
      <div class="info-card" style="padding:16px">
        <div class="info-label" style="margin-bottom:8px">磁盘使用趋势</div>
        <v-chart class="chart" :option="diskOption" autoresize />
      </div>
      <div class="info-card" style="padding:16px">
        <div class="info-label" style="margin-bottom:8px">网络 IO 趋势</div>
        <v-chart class="chart" :option="netOption" autoresize />
      </div>
    </div>

    <h3 class="section-title" style="margin-top:24px">网络接口实时数据</h3>
    <div class="mobile-card-list">
      <div v-for="n in netInfo" :key="n.name" class="info-card net-mobile-card">
        <div class="net-mobile-head">{{ n.name }}</div>
        <div class="net-mobile-row"><span>接收</span><span>{{ formatBytes(n.bytes_recv) }}</span></div>
        <div class="net-mobile-row"><span>发送</span><span>{{ formatBytes(n.bytes_sent) }}</span></div>
        <div class="net-mobile-row"><span>包数 收/发</span><span>{{ n.packets_recv }} / {{ n.packets_sent }}</span></div>
      </div>
      <div v-if="!netInfo.length" class="battery-na">暂无网络接口数据</div>
    </div>
    <div class="table-wrap desktop-only">
      <el-table :data="netInfo" size="small">
        <el-table-column prop="name" label="接口" />
        <el-table-column label="接收">
          <template #default="{ row }">{{ formatBytes(row.bytes_recv) }}</template>
        </el-table-column>
        <el-table-column label="发送">
          <template #default="{ row }">{{ formatBytes(row.bytes_sent) }}</template>
        </el-table-column>
        <el-table-column label="包数(收/发)">
          <template #default="{ row }">{{ row.packets_recv }} / {{ row.packets_sent }}</template>
        </el-table-column>
      </el-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { usePolling } from '../composables/usePolling'
import { use } from 'echarts/core'
	import { CanvasRenderer } from 'echarts/renderers'
	import { LineChart, GaugeChart } from 'echarts/charts'
	import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
	import VChart from 'vue-echarts'
	import { monitorApi, dashboardApi } from '../api'
	import { ElMessage, ElMessageBox } from 'element-plus'

	use([CanvasRenderer, LineChart, GaugeChart, GridComponent, TooltipComponent, LegendComponent])

const timeRange = ref(1440)
	const history = ref<any[]>([])
	const netInfo = ref<any[]>([])
	const realtimeMetrics = ref<any>(null)
	const battery = ref<any>(null)
	const thermal = ref<any[]>([])
	let realtimeLoading = false

	const batteryColor = computed(() => {
	  const c = battery.value?.capacity ?? 0
	  const charging = (battery.value?.status || '').toLowerCase().includes('charging')
	  if (charging) return '#81c995'
	  return c > 50 ? '#00d26a' : c > 20 ? '#ffa726' : '#f28b82'
	})

	// 功耗来源标签：驱动直读 / V×I 计算 / 电量变化率估算
	const powerTag = computed(() => {
	  const s = battery.value?.power_source
	  if (s === 'driver') return ''
	  if (s === 'vi') return 'V×I'
	  if (s === 'estimate') return '估算'
	  return ''
	})

	// 读不到功耗时的原因说明。满电静置时 BMS 断开充电回路，
	// current_now 物理上就是 0，这不是故障。
	const powerHint = computed(() => {
	  const b = battery.value
	  if (!b) return ''
	  const st = (b.status || '').toLowerCase()
	  if (st === 'full') return '满电静置，充电回路已断开，无电流可测'
	  if (st === 'not charging') return '未充电状态，电池无电流输出'
	  if (b.charging) return '已接外部电源，等待电流采样'
	  return '本轮采样无电荷变化，稍后自动刷新'
	})

	function tempColor(t: number) {
	  if (t >= 70) return '#f28b82'
	  if (t >= 50) return '#ffa726'
	  return '#81c995'
	}

	async function loadBattery() {
	  try {
	    const res: any = await monitorApi.battery()
	    battery.value = res.data
	  } catch (e) {}
	}

	async function loadThermal() {
	  try {
	    const res: any = await monitorApi.thermal()
	    thermal.value = res.data || []
	  } catch (e) {}
	}

	async function showDiag() {
	  try {
	    const res: any = await monitorApi.hardwareDiag()
	    const d = res.data || {}
	    const lines = [
	      '宿主根: ' + (d.host_root || '(未逃逸)'),
	      'chroot 环境: ' + (d.chroot_like ? '是' : '否'),
	      '电池命中: ' + (d.battery_hit || '(无)'),
	      '电池候选: ' + ((d.battery_dirs || []).join(' , ') || '(无)'),
	      '温度命中: ' + (d.thermal_hit || '(无)'),
	      '温度候选: ' + ((d.thermal_bases || []).join(' , ') || '(无)'),
	      '有效传感器: ' + (d.thermal_valid ?? 0),
	      '原始 zone: ' + ((d.thermal_raw || []).join(' , ') || '(无)'),
	      '电池属性: ' + ((d.battery_files || []).join(' , ') || '(无)')
	    ]
	    ElMessageBox.alert(
	      '<pre style="white-space:pre-wrap;word-break:break-all;font-size:12px;margin:0">' +
	      lines.map((l) => l.replace(/</g, '&lt;')).join('\n') + '</pre>',
	      '硬件读取诊断',
	      { dangerouslyUseHTMLString: true, customClass: 'hw-diag-box' }
	    )
	  } catch (e: any) {
	    ElMessage.error(e?.message || '诊断失败')
	  }
	}

function formatBytes(bytes?: number) {
  if (bytes === undefined || bytes === null) return '-'
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

function formatTime(ts: string) {
  const d = new Date(ts)
  return d.getHours().toString().padStart(2, '0') + ':' + d.getMinutes().toString().padStart(2, '0')
}

function buildOption(title: string, data: number[], times: string[], color: string, formatter?: (v: number) => string) {
	  return {
	    tooltip: { trigger: 'axis', formatter: (p: any) => `${p[0].axisValue}<br/>${p[0].marker} ${title}: ${formatter ? formatter(p[0].value) : p[0].value}` },
	    grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
	    xAxis: { type: 'category', boundaryGap: false, data: times, axisLine: { lineStyle: { color: '#555' } }, axisLabel: { color: '#888' } },
	    yAxis: { type: 'value', axisLine: { lineStyle: { color: '#555' } }, axisLabel: { color: '#888' }, splitLine: { lineStyle: { color: '#2a2a2a' } } },
	    series: [{ name: title, type: 'line', smooth: true, symbol: 'none', areaStyle: { opacity: 0.15, color }, lineStyle: { width: 2, color }, itemStyle: { color }, data }]
	  }
	}

	function buildGauge(value: number, color: string) {
	  return {
	    series: [{
	      type: 'gauge',
	      startAngle: 210,
	      endAngle: -30,
	      center: ['50%', '60%'],
	      radius: '90%',
	      min: 0,
	      max: 100,
	      splitNumber: 10,
	      axisLine: {
	        show: true,
	        lineStyle: { width: 12, color: [[value / 100, color], [1, '#2a2a2a']] }
	      },
	      pointer: { length: '60%', width: 6, itemStyle: { color: '#aaa' } },
	      axisTick: { distance: -12, length: 6, lineStyle: { width: 1, color: '#555' } },
	      splitLine: { distance: -18, length: 14, lineStyle: { width: 2, color: '#555' } },
	      axisLabel: { color: '#888', distance: 20, fontSize: 10 },
	      detail: { valueAnimation: true, formatter: '{value}%', color: '#fff', fontSize: 18, offsetCenter: [0, '70%'] },
	      data: [{ value: value }]
	    }]
	  }
	}

const cpuOption = ref({})
	const memOption = ref({})
	const diskOption = ref({})
	const netOption = ref({})
	const cpuGaugeOption = ref(buildGauge(0, '#00d26a'))
	const memGaugeOption = ref(buildGauge(0, '#4f8cff'))
	const diskGaugeOption = ref(buildGauge(0, '#ffa726'))

function updateCharts() {
  const items = history.value
  const times = items.map((i: any) => formatTime(i.recorded_at))
  const cpuData = items.map((i: any) => parseFloat(i.cpu_usage.toFixed(1)))
  const memData = items.map((i: any) => parseFloat(((i.mem_used / (i.mem_total || 1)) * 100).toFixed(1)))
  const diskData = items.map((i: any) => parseFloat(((i.disk_used / (i.disk_total || 1)) * 100).toFixed(1)))
  const netData = items.map((i: any) => i.net_recv)
  cpuOption.value = buildOption('CPU%', cpuData, times, '#00d26a')
  memOption.value = buildOption('内存', memData, times, '#4f8cff', (v: number) => v + '%')
  diskOption.value = buildOption('磁盘', diskData, times, '#ffa726', (v: number) => v + '%')
  netOption.value = buildOption('接收', netData, times, '#ab47bc', formatBytes)
}

async function loadData() {
  try {
    const res: any = await monitorApi.history(timeRange.value)
    history.value = res.data || []
    updateCharts()
  } catch (e: any) {
    ElMessage.error(e?.message || '加载监控数据失败')
  }
}

async function loadNetInfo() {
	  try {
	    const res: any = await dashboardApi.getMonitor()
	    netInfo.value = res.data?.network || []
	  } catch (e) {}
	}

	async function loadRealtime() {
	  // 互斥：/monitor/realtime 单次要 1.8 秒，5 秒间隔下若上一次未完成就跳过，
	  // 否则请求会越堆越多形成自我拥塞
	  if (realtimeLoading) return
	  realtimeLoading = true
	  try {
	    const res: any = await monitorApi.realtime()
	    const m = res.data
	    realtimeMetrics.value = m
	    cpuGaugeOption.value = buildGauge(parseFloat((m?.cpu_percent || 0).toFixed(1)), '#00d26a')
	    memGaugeOption.value = buildGauge(parseFloat((m?.mem_percent || 0).toFixed(1)), '#4f8cff')
	    diskGaugeOption.value = buildGauge(parseFloat((m?.disk_percent || 0).toFixed(1)), '#ffa726')
	  } catch (e) {
	  } finally {
	    realtimeLoading = false
	  }
	}

	// keep-alive 兼容：切走页面即停轮询，切回来恢复
	const slowPolling = usePolling(() => {
	  loadData(); loadNetInfo(); loadBattery(); loadThermal()
	}, 60000)
	const realtimePolling = usePolling(loadRealtime, 10000)

	// 首次拉取由 onMounted 负责（usePolling 首轮激活只启动定时器，不会重复请求），
	// 之后切回页面时由 usePolling 的 onActivated 补一次刷新。
	onMounted(() => {
	  loadData()
	  loadNetInfo()
	  loadRealtime()
	  loadBattery()
	  loadThermal()
	  slowPolling.start()
	  realtimePolling.start()
	})
</script>

<style scoped>
	.gauge-grid {
	  display: grid;
	  grid-template-columns: repeat(4, 1fr);
	  gap: 16px;
	  margin-bottom: 20px;
	}
	@media (max-width: 900px) {
	  .gauge-grid {
	    grid-template-columns: repeat(2, 1fr);
	  }
	}
	@media (max-width: 480px) {
	  .gauge-grid { grid-template-columns: 1fr; }
	  .gauge-chart { height: 150px; }
	}
	.gauge-card {
	  padding: 12px;
	  text-align: center;
	}
	.battery-card { display: flex; flex-direction: column; justify-content: center; }
	.battery-pct { font-size: 28px; font-weight: 700; line-height: 1.2; }
	.battery-pct-sign { font-size: 14px; font-weight: 600; margin-left: 2px; }
	.battery-row {
	  display: flex; justify-content: space-between;
	  font-size: 12px; color: #888; margin-top: 4px; gap: 8px;
	}
	.battery-row span { white-space: nowrap; }
	.battery-bar { margin-top: 10px; }
	.power-cell { display: inline-flex; align-items: center; gap: 4px; }
	.power-tag {
	  font-size: 10px; padding: 0 4px; border-radius: 3px;
	  background: rgba(79, 140, 255, 0.15); color: #7aa7ff;
	  font-weight: 500; line-height: 15px;
	}
	.power-note {
	  margin-top: 8px; font-size: 11px; color: #6b7280;
	  line-height: 1.4; text-align: left;
	}
	.battery-na { color: #888; font-size: 13px; padding: 24px 0; }
	.thermal-row { display: flex; justify-content: space-between; align-items: center; padding: 3px 0; font-size: 13px; gap: 8px; }
	.thermal-type { color: #9aa0a6; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 60%; font-family: 'SF Mono', Consolas, monospace; }
	.thermal-temp { font-weight: 600; white-space: nowrap; }
	.thermal-count { float: right; font-size: 11px; color: #888; font-weight: normal; }
	.diag-link { margin-top: 6px; font-size: 12px; color: #4f8cff; cursor: pointer; }
	.diag-link:hover { text-decoration: underline; }
	.gauge-chart {
	  width: 100%;
	  height: 180px;
	}
	.chart-grid {
	  display: grid;
	  grid-template-columns: repeat(2, 1fr);
	  gap: 16px;
	}
	@media (max-width: 900px) {
	  .chart-grid {
	    grid-template-columns: 1fr;
	  }
	}
	.chart {
	  width: 100%;
	  height: 260px;
	}
	/* 网络接口：移动端用卡片，桌面端用表格 */
	.mobile-card-list { display: none; }

	/* 平板宽度（768–900）仍走表格，容器需能收缩；
	   不给 el-table 设 min-width，否则整表会撑破视口，
	   横向滚动交给 el-table 内部的 .el-scrollbar__wrap。 */
	.table-wrap {
	  width: 100%;
	  max-width: 100%;
	  min-width: 0;
	  overflow-x: auto;
	  overflow-y: hidden;
	  -webkit-overflow-scrolling: touch;
	  overscroll-behavior-x: contain;
	}
	.table-wrap :deep(.el-table) { width: 100%; min-width: 0; max-width: 100%; }
	.net-mobile-card { padding: 12px; margin-bottom: 10px; }
	.net-mobile-head {
	  font-size: 14px; font-weight: 600; color: #e8eaed;
	  padding-bottom: 8px; margin-bottom: 6px; border-bottom: 1px solid #2a2a2a;
	  font-family: 'SF Mono', Consolas, monospace;
	}
	.net-mobile-row {
	  display: flex; justify-content: space-between; align-items: center;
	  font-size: 13px; color: #9aa0a6; padding: 3px 0;
	}
	.net-mobile-row span:last-child { color: #e8eaed; font-weight: 600; }
	@media (max-width: 768px) {
	  .desktop-only { display: none; }
	  .mobile-card-list { display: block; }
	  .range-bar :deep(.el-radio-group) { flex-wrap: wrap; width: 100%; }
	  .range-bar :deep(.el-radio-button) { flex: 1 1 45%; }
	  .range-bar :deep(.el-radio-button__inner) { width: 100%; padding: 8px 0; }
	  .chart { height: 200px; }
	  .thermal-type { max-width: 55%; font-size: 12px; }
	  .battery-row { font-size: 12px; }
	}
</style>
