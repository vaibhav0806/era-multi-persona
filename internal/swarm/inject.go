package swarm

// InjectPlan composes the task description Pi sees: the original user-facing
// task first, followed by the planner's step list as additional context.
//
// Pi reads ERA_TASK_DESCRIPTION verbatim — there is no separate "plan" env
// var. We thread the plan through by appending it to the description.
func InjectPlan(taskDesc, planText string) string {
	if planText == "" {
		return taskDesc
	}
	return taskDesc + "\n\n--- Planner step list (from the planner persona; treat as guidance, not literal commands) ---\n" + planText
}
