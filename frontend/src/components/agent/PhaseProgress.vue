<template>
  <div class="phase-progress" v-if="currentPhase">
    <div
      v-for="p in phases"
      :key="p.key"
      class="phase-card"
      :class="phaseStatus(p.key)"
    >
      <div class="phase-icon">
        <span v-if="phaseStatus(p.key) === 'done'">✅</span>
        <span v-else-if="phaseStatus(p.key) === 'active'" class="pulse">{{ p.icon }}</span>
        <span v-else class="dim">{{ p.icon }}</span>
      </div>
      <div class="phase-info">
        <div class="phase-name">{{ p.label }}</div>
        <div class="phase-step" v-if="phaseStatus(p.key) === 'active' && maxSteps > 0">
          步骤 {{ stepNumber }}/{{ maxSteps }}
        </div>
        <div class="phase-step" v-else-if="phaseStatus(p.key) === 'done'">
          已完成
        </div>
      </div>
      <div class="phase-arrow" v-if="p.key !== phases[phases.length - 1].key">→</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  currentPhase: string
  completedPhases: string[]
  stepNumber: number
  maxSteps: number
}>()

const phases = [
  { key: 'planning', label: '规划', icon: '📝' },
  { key: 'coding', label: '执行', icon: '🔧' },
  { key: 'reviewing', label: '审查', icon: '✅' }
]

const order = computed(() => phases.map(p => p.key))

function phaseStatus(key: string): 'pending' | 'active' | 'done' {
  if (props.completedPhases.includes(key)) return 'done'
  if (props.currentPhase === key) return 'active'
  return 'pending'
}
</script>

<style scoped>
.phase-progress {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 10px 16px;
  background: var(--card);
  border: 1px solid var(--bdr);
  border-radius: 10px;
  margin-bottom: 12px;
}

.phase-card {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-radius: 8px;
  transition: all 0.3s;
}

.phase-card.active {
  background: var(--acc-bg);
  border: 1px solid var(--acc);
}

.phase-card.done {
  opacity: 0.6;
}

.phase-card.pending {
  opacity: 0.4;
}

.phase-icon {
  font-size: 18px;
}

.pulse {
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.6; transform: scale(1.15); }
}

.phase-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--txt);
}

.phase-card.active .phase-name {
  color: var(--acc);
}

.phase-step {
  font-size: 11px;
  color: var(--txt2);
  margin-top: 2px;
}

.dim {
  filter: grayscale(1);
  opacity: 0.5;
}

.phase-arrow {
  color: var(--txt2);
  font-size: 14px;
  margin: 0 4px;
}
</style>
