package api

import (
	"strconv"

	"github.com/smira/aptly/task"
	"github.com/gin-gonic/gin"
)

// GET /tasks
func apiTasksList(c *gin.Context) {
	queue := context.Queue()
	c.JSON(200, queue.GetTasks())
}

// POST /tasks/clear
func apiTasksClear(c *gin.Context) {
	queue := context.Queue()
	queue.Clear()
	c.JSON(200, gin.H{})
}

// GET /tasks/wait
func apiTasksWait(c *gin.Context) {
	queue := context.Queue()
	queue.Wait()
	c.JSON(200, gin.H{})
}

// GET /tasks/:id
func apiTasksShow(c *gin.Context) {
	q := context.Queue()
	id, err := strconv.ParseInt(c.Params.ByName("id"), 10, 0)
	if err != nil {
		c.Fail(500, err)
		return
	}

	var task task.Task
	task, err = q.GetTaskByID(int(id))
	if err != nil {
		c.Fail(500, err)
		return
	}

	c.JSON(200, task)
}

// GET /tasks/:id/output
func apiTasksOutputShow(c *gin.Context) {
	q := context.Queue()
	id, err := strconv.ParseInt(c.Params.ByName("id"), 10, 0)
	if err != nil {
		c.Fail(500, err)
		return
	}

	var output string
	output, err = q.GetTaskOutputByID(int(id))
	if err != nil {
		c.Fail(500, err)
		return
	}

	c.JSON(200, output)
}