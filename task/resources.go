package task

// ResourceConflictError represents a list tasks
// using conflicitng resources
type ResourceConflictError struct {
	Tasks   []Task
	Message string
}

func (e *ResourceConflictError) Error() string {
	return e.Message
}

// ResourcesSet represents a set of task resources.
// A resource is represented by its unique key
type ResourcesSet struct {
	set map[string]*Task
}

// NewResourcesSet creates new instance of resources set
func NewResourcesSet() *ResourcesSet {
	return &ResourcesSet{make(map[string]*Task)}
}

// MarkInUse given resources as used by given task
func (r *ResourcesSet) MarkInUse(resources []string, task *Task) {
	for _, resource := range resources {
		r.set[resource] = task
	}
}

// UsedBy checks whether one of given resources
// is used by a task and if yes returns slice of such task
func (r *ResourcesSet) UsedBy(resources []string) []Task {
	var tasks []Task
	for _, resource := range resources {
		task, found := r.set[resource]
		if found {
			tasks = appendTask(tasks, task)
			break
		}
	}

	return tasks
}

// appendTask only appends task to tasks slice if not already
// on slice
func appendTask(tasks []Task, task *Task) []Task {
	needsAppending := true
	for _, givenTask := range tasks {
		if givenTask.ID == task.ID {
			needsAppending = false
		}
	}

	if needsAppending {
		return append(tasks, *task)
	}

	return tasks
}

// Free removes given resources from dependency set
func (r *ResourcesSet) Free(resources []string) {
	for _, resource := range resources {
		delete(r.set, resource)
	}
}
