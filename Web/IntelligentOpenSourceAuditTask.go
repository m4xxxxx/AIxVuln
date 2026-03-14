package Web

import (
	"AIxVuln/misc"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type intelligentAddTask struct {
	ID        string                 `json:"task_id"`
	Status    string                 `json:"status"` // queued|running|success|failed
	Progress  int                    `json:"progress"`
	Message   string                 `json:"message"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
	Logs      []scoutTraceItem       `json:"logs"`
	Result    map[string]interface{} `json:"result,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

const intelligentTaskKeepMax = 100
const intelligentTaskLogKeepMax = 400

func (s *Server) startIntelligentAddOpenSourceAuditTask(c *gin.Context) {
	missing := misc.CheckRequiredConfig()
	if len(missing) > 0 {
		c.JSON(400, Fail("请先在「设置」中配置必填项: "+strings.Join(missing, ", ")))
		return
	}

	var req intelligentAddOpenSourceAuditReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, Fail("invalid JSON: "+err.Error()))
		return
	}
	if err := normalizeIntelligentAddOpenSourceAuditReq(&req); err != nil {
		c.JSON(400, Fail(err.Error()))
		return
	}

	task := s.createIntelligentAddTask()
	s.appendIntelligentTaskLog(task.ID, "queued", "任务已创建，等待执行")
	go s.runIntelligentAddTask(task.ID, req)

	c.JSON(200, Success(map[string]interface{}{
		"task_id": task.ID,
		"status":  task.Status,
		"message": task.Message,
	}))
}

func (s *Server) getIntelligentAddOpenSourceAuditTask(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(400, Fail("id is required"))
		return
	}
	from := 0
	if raw := c.Query("from"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			from = v
		}
	}
	snapshot, exists := s.getIntelligentAddTaskSnapshot(id, from)
	if !exists {
		c.JSON(404, Fail("task not found"))
		return
	}
	c.JSON(200, Success(snapshot))
}

func (s *Server) createIntelligentAddTask() *intelligentAddTask {
	now := time.Now().Format("2006-01-02 15:04:05")
	task := &intelligentAddTask{
		ID:        uuid.New().String(),
		Status:    "queued",
		Progress:  0,
		Message:   "任务排队中",
		CreatedAt: now,
		UpdatedAt: now,
		Logs:      make([]scoutTraceItem, 0, 64),
	}

	s.intelligentTaskMu.Lock()
	defer s.intelligentTaskMu.Unlock()
	s.intelligentTasks[task.ID] = task
	s.intelligentTaskOrder = append(s.intelligentTaskOrder, task.ID)
	for len(s.intelligentTaskOrder) > intelligentTaskKeepMax {
		oldest := s.intelligentTaskOrder[0]
		s.intelligentTaskOrder = s.intelligentTaskOrder[1:]
		delete(s.intelligentTasks, oldest)
	}
	return task
}

func (s *Server) runIntelligentAddTask(taskID string, req intelligentAddOpenSourceAuditReq) {
	s.updateIntelligentTask(taskID, "running", 1, "开始执行智能添加")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result, err := s.runIntelligentAddOpenSourceAuditWorkflow(ctx, req, func(p intelligentAddProgress) {
		percent := p.Percent
		if percent < 1 {
			percent = 1
		}
		if percent > 99 {
			percent = 99
		}
		s.updateIntelligentTask(taskID, "running", percent, p.Detail)
		s.appendIntelligentTaskLog(taskID, p.Stage, p.Detail)
	})
	if err != nil {
		s.finishIntelligentTask(taskID, "failed", 100, err.Error(), result, err.Error())
		return
	}
	s.finishIntelligentTask(taskID, "success", 100, "智能添加完成", result, "")
}

func (s *Server) updateIntelligentTask(taskID, status string, progress int, message string) {
	s.intelligentTaskMu.Lock()
	defer s.intelligentTaskMu.Unlock()
	t, ok := s.intelligentTasks[taskID]
	if !ok {
		return
	}
	t.Status = status
	t.Progress = progress
	t.Message = message
	t.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
}

func (s *Server) appendIntelligentTaskLog(taskID, stage, detail string) {
	s.intelligentTaskMu.Lock()
	defer s.intelligentTaskMu.Unlock()
	t, ok := s.intelligentTasks[taskID]
	if !ok {
		return
	}
	t.Logs = append(t.Logs, scoutTraceItem{
		Time:   time.Now().Format("15:04:05"),
		Stage:  stage,
		Detail: detail,
	})
	if len(t.Logs) > intelligentTaskLogKeepMax {
		t.Logs = append([]scoutTraceItem(nil), t.Logs[len(t.Logs)-intelligentTaskLogKeepMax:]...)
	}
	t.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
}

func (s *Server) finishIntelligentTask(taskID, status string, progress int, message string, result map[string]interface{}, errMsg string) {
	s.intelligentTaskMu.Lock()
	defer s.intelligentTaskMu.Unlock()
	t, ok := s.intelligentTasks[taskID]
	if !ok {
		return
	}
	t.Status = status
	t.Progress = progress
	t.Message = message
	t.Result = result
	t.Error = errMsg
	t.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
}

func (s *Server) getIntelligentAddTaskSnapshot(taskID string, from int) (map[string]interface{}, bool) {
	s.intelligentTaskMu.RLock()
	defer s.intelligentTaskMu.RUnlock()
	t, ok := s.intelligentTasks[taskID]
	if !ok {
		return nil, false
	}
	if from < 0 {
		from = 0
	}
	if from > len(t.Logs) {
		from = len(t.Logs)
	}
	logs := make([]scoutTraceItem, len(t.Logs[from:]))
	copy(logs, t.Logs[from:])
	result := map[string]interface{}{
		"task_id":    t.ID,
		"status":     t.Status,
		"progress":   t.Progress,
		"message":    t.Message,
		"created_at": t.CreatedAt,
		"updated_at": t.UpdatedAt,
		"logs":       logs,
		"from":       from,
		"next_from":  len(t.Logs),
		"total_logs": len(t.Logs),
		"error":      t.Error,
		"result":     t.Result,
	}
	return result, true
}
