package dockroot

import (
	"strings"
)

// NormalizeImageRef 将各种镜像引用格式统一规范化，确保 dockroot 能正确解析。
// 支持格式：
//   - nginx:latest
//   - docker.1ms.run/openlistteam/openlist:latest-lite-aio
//   - registry.linkease.net:5443/nginx:latest
//   - docker://registry.host:5000/ns/name:tag
//
// 返回值为 "imageName:tag" 格式，imageName 中保留 registry/port/path。
func NormalizeImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}

	// 去掉 docker:// 前缀（内部处理，最后再决定是否需要保留）
	hasDockerPrefix := strings.HasPrefix(ref, "docker://")
	ref = strings.TrimPrefix(ref, "docker://")

	// 处理 digest（@sha256:...）—— dockroot pull 目前主要用 tag，digest 暂转为 latest
	if at := strings.LastIndex(ref, "@"); at != -1 {
		ref = ref[:at]
	}

	var name, tag string

	// 从右往左找最后一个 ':'，冒号后面不含 '/' 的才是真正的 tag 分隔符
	if colon := strings.LastIndex(ref, ":"); colon != -1 {
		after := ref[colon+1:]
		if !strings.Contains(after, "/") && after != "" {
			name = ref[:colon]
			tag = after
		}
	}

	if tag == "" {
		name = ref
		tag = "latest"
	}

	// 如果原字符串带 docker:// 前缀，保留它，dockroot 的解析逻辑会正确识别
	if hasDockerPrefix {
		name = "docker://" + name
	}

	return name + ":" + tag
}

// ExtractContainerName 从镜像引用中提取适合作为容器名称的短名称。
// 规则：取镜像路径最后一个 '/' 之后的部分，再去除 tag/digest。
// 示例：
//   - docker.1ms.run/openlistteam/openlist:latest → openlist
//   - registry.linkease.net:5443/nginx:alpine → nginx
//   - docker.io/library/redis:latest → redis
func ExtractContainerName(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}

	// 去掉 docker:// 前缀
	ref = strings.TrimPrefix(ref, "docker://")

	// 去掉 digest
	if at := strings.LastIndex(ref, "@"); at != -1 {
		ref = ref[:at]
	}

	// 去掉 tag（从右往左找第一个 ':'，后面不含 '/'）
	if colon := strings.LastIndex(ref, ":"); colon != -1 {
		after := ref[colon+1:]
		if !strings.Contains(after, "/") {
			ref = ref[:colon]
		}
	}

	// 取最后一个 '/' 后面的部分
	if slash := strings.LastIndex(ref, "/"); slash != -1 {
		ref = ref[slash+1:]
	}

	return ref
}
