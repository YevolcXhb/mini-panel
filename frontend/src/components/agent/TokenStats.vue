<template>
  <div class="token-stats" v-if="hasData">
    <span class="stat-item" title="压缩节省的 token 数">
      <span class="stat-icon">💾</span>
      <span class="stat-value">{{ formatNum(tokensSaved) }}</span>
      <span class="stat-label">tokens 节省</span>
    </span>
    <span class="stat-sep">|</span>
    <span class="stat-item" title="压缩触发次数">
      <span class="stat-icon">🗜️</span>
      <span class="stat-value">{{ compressionCount }}</span>
      <span class="stat-label">次压缩</span>
    </span>
    <span class="stat-sep">|</span>
    <span class="stat-item" title="工具缓存命中次数">
      <span class="stat-icon">⚡</span>
      <span class="stat-value">{{ cacheHits }}</span>
      <span class="stat-label">次缓存命中</span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  tokensSaved: number
  compressionCount: number
  cacheHits: number
}>()

const hasData = computed(() => props.tokensSaved > 0 || props.compressionCount > 0 || props.cacheHits > 0)

function formatNum(n: number): string {
  if (n >= 10000) return (n / 1000).toFixed(1) + 'k'
  return String(n)
}
</script>

<style scoped>
.token-stats {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--card);
  border: 1px solid var(--bdr);
  border-radius: 8px;
  font-size: 12px;
  color: var(--txt2);
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 4px;
}

.stat-icon {
  font-size: 13px;
}

.stat-value {
  font-weight: 600;
  color: var(--txt);
}

.stat-label {
  font-size: 11px;
}

.stat-sep {
  color: var(--dim);
  margin: 0 2px;
}
</style>
