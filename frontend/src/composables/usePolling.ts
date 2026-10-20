// usePolling —— 与 keep-alive 兼容的轮询定时器。
//
// 为什么需要它：Layout.vue 用 <keep-alive> 包住了 router-view，所有访问过的
// 页面组件会被**缓存**而不是销毁。组件被缓存时 Vue 调用的是 onDeactivated，
// 不会调用 onUnmounted —— 而各页面原先只写了 onUnmounted(() => clearInterval(timer))，
// 这个回调在有 keep-alive 的情况下永远不会执行。
//
// 后果是每进一次页面就多注册一个定时器，切过的页面全都在后台按各自间隔打接口。
// 实测曾在设备上看到 /dashboard/monitor 被以 13 次/秒的频率请求。
//
// 本组合式函数做了三件事：
//   1. 页面隐藏（deactivated）时停掉定时器，重新显示（activated）时恢复
//   2. 组件真正销毁（unmounted）时兜底清理
//   3. 注册前先清旧定时器，避免重复调用导致叠加
import { onActivated, onDeactivated, onUnmounted, getCurrentInstance } from 'vue'

/**
 * 注册一个随页面激活状态自动启停的轮询。
 *
 * @param fn 每次触发执行的函数
 * @param interval 间隔毫秒
 * @param opts.immediate 是否立即执行一次，默认 false（组件自己的 onMounted 通常已拉过数据）
 * @returns 手动控制句柄：start / stop / isRunning
 */
export function usePolling(
  fn: () => void | Promise<void>,
  interval: number,
  opts: { immediate?: boolean } = {},
) {
  let timer: ReturnType<typeof setInterval> | null = null

  const stop = () => {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
  }

  const start = () => {
    // 先清再建：重复调用不会产生并行定时器
    stop()
    if (opts.immediate) {
      void fn()
    }
    timer = setInterval(() => {
      void fn()
    }, interval)
  }

  const isRunning = () => timer !== null

  // 只有在组件上下文里才注册生命周期钩子（也方便单测直接调用 start/stop）
  if (getCurrentInstance()) {
    onActivated(() => start())
    onDeactivated(() => stop())
    onUnmounted(() => stop())
  }

  return { start, stop, isRunning }
}
