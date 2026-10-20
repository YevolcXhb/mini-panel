#!/system/bin/sh
# Start Mini Panel inside the chroot once the rootfs and mounts are ready,
# then keep it alive.  Runs from the Magisk module service phase, after
# post-fs-data has mounted sys/proc/dev and sshd is already up.
MP=/data/Ubuntu24.04.2LTS
MPDIR=$MP/opt/minipanel
LOG=/data/chroot-boot.log

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] minipanel: $*" >> "$LOG"; }

# 判断面板是否存活。
#
# 历史缺陷：旧版用 pgrep -f "/opt/minipanel/minipanel" 匹配命令行，但进程实际
# 以 "./minipanel"（cd 到目录后相对启动）运行，绝对路径永远匹配不上。
# 结果 watchdog 每 60 秒都以为面板死了而重复拉起 —— boot log 里 "restarting"
# 累计出现 2487 次，PID 反复变化，前端每次都要重新建立连接。
#
# 现在只认进程名（pgrep -x 精确匹配 comm），不受工作目录和启动方式影响。
is_running() {
    pgrep -x minipanel >/dev/null 2>&1 && return 0
    # 兜底：某些 Android 版本 comm 会带截断或路径，再查一次 /proc
    for p in /proc/[0-9]*/comm; do
        [ -r "$p" ] || continue
        [ "$(cat "$p" 2>/dev/null)" = "minipanel" ] && return 0
    done
    return 1
}

# chroot service.sh may still be waiting for the rootfs on the first boot.
i=0
while [ ! -x "$MP/bin/bash" ]; do
    sleep 2
    i=$((i + 1))
    [ "$i" -gt 90 ] && { log "rootfs never appeared, giving up"; exit 1; }
done

# The mounts have to exist before minipanel can see its db and bind :8888.
for d in proc sys dev; do
    grep -q " $MP/$d " /proc/mounts || {
        case $d in
            proc) mount -t proc proc "$MP/$d" ;;
            sys)  mount -t sysfs sys "$MP/$d" ;;
            dev)  mount -o bind /dev "$MP/$d" ;;
        esac
        log "mounted $d"
    }
done

log "starting minipanel"
chroot "$MP" /bin/bash -c 'export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; cd /opt/minipanel && ./start.sh' >> "$LOG" 2>&1

# start.sh only checks for one second.  Keep a slow watchdog anyway: the panel
# binds :8888 on its own and nothing else would ever restart it.
#
# 防重启风暴：拉起后若 10 秒内仍不见进程（例如二进制损坏），说明不是"意外
# 退出"而是启动本身失败，此时拉长重试间隔，避免每分钟刷屏重启。
FAILS=0
while true; do
    sleep 60
    if is_running; then
        FAILS=0
        continue
    fi

    log "minipanel is gone, restarting (fail_count=$FAILS)"
    chroot "$MP" /bin/bash -c 'export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; cd /opt/minipanel && ./start.sh' >> "$LOG" 2>&1

    # 等待启动结果：10 秒内起来就算成功
    ok=0
    for _ in 1 2 3 4 5 6 7 8 9 10; do
        sleep 1
        if is_running; then ok=1; break; fi
    done

    if [ "$ok" -eq 1 ]; then
        FAILS=0
        log "restart succeeded"
    else
        FAILS=$((FAILS + 1))
        # 连续失败时退避：1、2、4、8 分钟，最多 8 分钟
        backoff=$((60 * (1 << (FAILS > 3 ? 3 : FAILS))))
        log "restart failed, backing off ${backoff}s"
        sleep "$backoff"
    fi
done
