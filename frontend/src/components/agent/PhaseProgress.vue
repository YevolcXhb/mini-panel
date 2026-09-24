<template>
  <div class="phase-progress-pill" v-if="currentPhase">
    <div
      v-for="(p, idx) in phases"
      :key="p.key"
      class="phase-node"
      :class="phaseStatus(p.key)"
    >
      <div class="node-indicator">
        <span v-if="phaseStatus(p.key) === 'done'" class="done-check">✓</span>
        <span v-else-if="phaseStatus(p.key) === 'active'" class="active-dot"></span>
        <span v-else class="pending-dot"></span>
      </div>
      <div class="node-meta">
        <span class="node-title">{{ p.label }}</span>
        <span class="node-sub" v-if="phaseStatus(p.key) === 'active' && maxSteps > 0">
          {{ stepNumber }}/{{ maxSteps }}
        </span>
      </div>
      <div class="connector-line" v-if="idx < phases.length - 1"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  currentPhase: string
  completedPhases: string[]
  stepNumber: number
  maxSteps: number
}>()

const phases = [
  { key: 'planning', label: '规划方案' },
  { key: 'coding', label: '调度执行' },
  { key: 'reviewing', label: '审查交付' }
]

function phaseStatus(key: string): 'pending' | 'active' | 'done' {
  if (props.completedPhases.includes(key)) return 'done'
  if (props.currentPhase === key) return 'active'
  return 'pending'
}
</script>

<style scoped>
.phase-progress-pill {
  display: inline-flex;
  align-items: center;
  background: var(--card);
  border: 1px solid var(--bdr);
  border-radius: 20px;
  padding: 6px 16px;
  gap: 12px;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.04);
}

.phase-node {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--dim);
  transition: all 0.25s ease;
}

.phase-node.active {
  color: var(--txt);
  font-weight: 600;
}

.phase-node.done {
  color: var(--txt2);
}

.node-indicator {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg2);
  border: 1px solid var(--bdr);
}

.phase-node.active .node-indicator {
  border-color: var(--acc);
  background: var(--acc-bg);
}

.phase-node.done .node-indicator {
  border-color: var(--grn);
  background: rgba(52, 211, 153, 0.15);
}

.done-check {
  font-size: 11px;
  color: var(--grn);
  font-weight: bold;
}

.active-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--acc);
  animation: pulse-gemini 1.4s infinite;
}

@keyframes pulse-gemini {
  0% { transform: scale(0.8); opacity: 0.7; }
  50% { transform: scale(1.2); opacity: 1; }
  100% { transform: scale(0.8); opacity: 0.7; }
}

.pending-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--dim);
}

.node-meta {
  display: flex;
  align-items: center;
  gap: 4px;
}

.node-sub {
  font-size: 11px;
  background: var(--bg2);
  padding: 1px 5px;
  border-radius: 8px;
  color: var(--acc);
}

.connector-line {
  width: 18px;
  height: 2px;
  background: var(--bdr);
  margin-left: 4px;
}

.phase-node.done .connector-line {
  background: var(--grn);
}
</style>
