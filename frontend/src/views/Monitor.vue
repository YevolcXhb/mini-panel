<template>
  <div>
    <h2 class="page-title">📈 监控中心</h2>

    <div style="margin-bottom:16px;display:flex;gap:8px;align-items:center">
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
    <div class="table-wrap">
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
import { ref, onMounted, onUnmounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { monitorApi, dashboardApi } from '../api'
import { ElMessage } from 'element-plus'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent])

const timeRange = ref(1440)
const history = ref<any[]>([])
const netInfo = ref<any[]>([])
let timer: any

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

const cpuOption = ref({})
const memOption = ref({})
const diskOption = ref({})
const netOption = ref({})

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
    const res: any = await dashboardApi.monitor()
    netInfo.value = res.data?.network || []
  } catch (e) {}
}

onMounted(() => {
  loadData()
  loadNetInfo()
  timer = setInterval(() => { loadData(); loadNetInfo() }, 60000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
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
</style>
