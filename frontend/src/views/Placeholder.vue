<template>
  <div>
    <h2 class="page-title">{{ icon }} {{ title }}</h2>
    <div class="info-card" style="max-width:500px;text-align:center;padding:48px 24px">
      <div style="font-size:48px;margin-bottom:16px;opacity:0.4">{{ icon }}</div>
      <div style="font-size:16px;font-weight:600;margin-bottom:8px;color:var(--txt)">{{ title }}</div>
      <div style="font-size:13px;color:var(--dim);margin-bottom:24px">{{ desc }}</div>
      <el-tag type="info" size="large">暂未实现</el-tag>
      <div style="margin-top:24px;font-size:12px;color:var(--dim)">
        该功能正在开发中，敬请期待
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  icon: string
  desc?: string
}>()
</script>
