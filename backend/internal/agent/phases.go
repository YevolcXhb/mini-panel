package agent

import (
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// OrchestratorPhase 三阶段编排的阶段枚举
type OrchestratorPhase string

const (
	PhasePlanning  OrchestratorPhase = "planning"
	PhaseCoding   OrchestratorPhase = "coding"
	PhaseReviewing OrchestratorPhase = "reviewing"
)

// MaxStepsPerPhase 每阶段最大步数
const MaxStepsPerPhase = 30

// PLANNER_SYSTEM_PROMPT 规划阶段 system prompt（运维场景适配）
const PLANNER_SYSTEM_PROMPT = `你是 MiniPanel 运维规划师。分析用户任务，制定详细的执行计划。

## 工作原则
1. 只使用查询类工具收集信息（get_system_info、list_processes、list_files、web_search 等），不执行任何修改操作
2. 充分了解当前系统状态后再制定计划
3. 计划应包含明确的步骤和预期结果

## 输出格式
完成规划后，必须输出以下格式的计划：

<plan_details>
详细列出每个步骤：
1. 步骤一：具体操作
2. 步骤二：具体操作
...
</plan_details>

<plan_approach>
说明实施方法、所需工具、注意事项
</plan_approach>

完成后输出 "plan completed"。

## 约束
- 只规划，不执行
- 计划要具体可执行
- 考虑安全风险和回滚方案`

// CODER_SYSTEM_PROMPT 执行阶段 system prompt
const CODER_SYSTEM_PROMPT = `你是 MiniPanel 运维执行者。按照计划严格执行任务。

## 工作原则
1. 严格按照计划步骤执行
2. 使用所有可用工具完成操作
3. 每步执行后验证结果
4. 遇到问题时及时调整，但保持目标不变
5. 危险操作前说明影响

## 完成标志
任务完成后，输出 "execution completed" 或调用 task_done（如有）。

## 约束
- 高效执行，避免不必要的步骤
- 所有操作必须有明确目的
- 失败时说明原因和已完成的步骤`

// REVIEWER_SYSTEM_PROMPT 审查阶段 system prompt
const REVIEWER_SYSTEM_PROMPT = `你是 MiniPanel 运维审查员。验证执行结果是否达到预期。

## 工作原则
1. 必须使用 execute_command 运行验证命令（如 systemctl status、curl、cat、ls 等）
2. 验证所有关键操作的结果
3. 检查是否有遗漏或错误

## 输出格式
## Review Verdict
**pass** 或 **fail**

### 验证结果
- 验证项1：结果
- 验证项2：结果

### 备注
（如有问题或建议）

## 约束
- 必须实际运行验证命令，不能凭空判断
- pass/fail 必须基于验证结果`

// PhaseToolNames 每阶段允许的工具集（白名单）
// 空集合表示允许所有工具
var PhaseToolNames = map[OrchestratorPhase]map[string]bool{
	PhasePlanning: {
		"get_system_info":    true,
		"list_processes":     true,
		"dashboard_overview": true,
		"container_list":     true,
		"website_list":       true,
		"database_list":      true,
		"firewall_list":      true,
		"file_list":          true,
		"file_read":          true,
		"web_search":         true,
		"web_fetch":          true,
		"resolve_lazy_ref":  true,
	},
	PhaseCoding: {}, // 空集合表示允许所有工具
	PhaseReviewing: {
		"execute_command":   true,
		"file_read":         true,
		"file_list":         true,
		"resolve_lazy_ref":  true,
	},
}

// PhaseComplete 检测阶段是否完成
func PhaseComplete(phase OrchestratorPhase, response *provider.LLMResponse) bool {
	content := strings.ToLower(response.Content)

	switch phase {
	case PhasePlanning:
		// 规划完成：包含 "plan completed" 且有闭合标签
		return strings.Contains(content, "plan completed") ||
			(strings.Contains(content, "</plan_details>") && strings.Contains(content, "</plan_approach>"))

	case PhaseCoding:
		// 执行完成：包含 "execution completed" 或调用 task_done
		if strings.Contains(content, "execution completed") || strings.Contains(content, "task_done") {
			return true
		}
		// 检查工具调用中是否有 task_done
		for _, tc := range response.ToolCalls {
			if tc.Function.Name == "task_done" {
				return true
			}
		}
		return false

	case PhaseReviewing:
		// 审查完成：包含 pass/fail 或 Review Verdict
		return strings.Contains(content, "**pass**") ||
			strings.Contains(content, "**fail**") ||
			strings.Contains(content, "## review verdict") ||
			strings.Contains(content, "review verdict")

	default:
		return false
	}
}

// BuildCodingContext 构建 CODING 阶段的 handoff context
func BuildCodingContext(task, plan string) string {
	return "## 任务\n" + task + "\n\n" +
		"## 执行计划\n" + plan + "\n\n" +
		"请按照上述计划执行任务。使用所有可用工具完成操作。完成后输出 \"execution completed\"。"
}

// BuildReviewContext 构建 REVIEWING 阶段的 handoff context
func BuildReviewContext(task, codeResult string) string {
	return "## 任务\n" + task + "\n\n" +
		"## 执行结果\n" + codeResult + "\n\n" +
		"请审查上述执行结果。必须使用 execute_command 运行验证命令。输出 **pass** 或 **fail**。"
}
