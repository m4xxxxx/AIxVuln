package Web

import (
	"AIxVuln/misc"
	"fmt"
)

func (s *Server) initStartQueueDispatcher() {
	go s.startQueueDispatchLoop()
}

func (s *Server) startQueueDispatchLoop() {
	for range s.startQueueNotify {
		for {
			limit := misc.GetProjectTaskMaxConcurrency()
			if limit < 1 {
				limit = 1
			}

			s.startQueueMu.Lock()
			if len(s.startRunningSet) >= limit || len(s.startQueue) == 0 {
				s.startQueueMu.Unlock()
				break
			}

			projectName := s.startQueue[0]
			s.startQueue = s.startQueue[1:]
			delete(s.startQueueSet, projectName)
			s.startRunningSet[projectName] = struct{}{}
			s.startQueueMu.Unlock()

			go s.runQueuedProject(projectName)
		}
	}
}

func (s *Server) runQueuedProject(projectName string) {
	defer func() {
		s.startQueueMu.Lock()
		delete(s.startRunningSet, projectName)
		s.startQueueMu.Unlock()
		s.notifyStartQueue()
	}()

	s.mu.RLock()
	pm, exists := s.pms[projectName]
	s.mu.RUnlock()
	if !exists {
		return
	}
	pm.StartTask()
}

func (s *Server) enqueueProjectStart(projectName string) (state string, queuePos int) {
	s.startQueueMu.Lock()
	defer s.startQueueMu.Unlock()

	if _, running := s.startRunningSet[projectName]; running {
		return "running", 0
	}
	if _, queued := s.startQueueSet[projectName]; queued {
		for i, pn := range s.startQueue {
			if pn == projectName {
				return "queued", i + 1
			}
		}
		return "queued", 1
	}

	s.startQueue = append(s.startQueue, projectName)
	s.startQueueSet[projectName] = struct{}{}
	queuePos = len(s.startQueue)
	state = "queued"
	s.notifyStartQueue()
	return state, queuePos
}

func (s *Server) removeProjectFromStartQueue(projectName string) (removed bool, queuePos int) {
	s.startQueueMu.Lock()
	defer s.startQueueMu.Unlock()

	for i, pn := range s.startQueue {
		if pn == projectName {
			queuePos = i + 1
			s.startQueue = append(s.startQueue[:i], s.startQueue[i+1:]...)
			delete(s.startQueueSet, projectName)
			return true, queuePos
		}
	}
	return false, 0
}

func (s *Server) getProjectStartQueueState(projectName string) (queued bool, queuePos int, running bool) {
	s.startQueueMu.Lock()
	defer s.startQueueMu.Unlock()

	_, running = s.startRunningSet[projectName]
	for i, pn := range s.startQueue {
		if pn == projectName {
			return true, i + 1, running
		}
	}
	return false, 0, running
}

func (s *Server) notifyStartQueue() {
	select {
	case s.startQueueNotify <- struct{}{}:
	default:
	}
}

func formatQueueStartMessage(state string, queuePos int) string {
	switch state {
	case "running":
		return "项目正在运行中"
	case "queued":
		if queuePos > 0 {
			return fmt.Sprintf("项目已加入启动队列，当前排队第 %d 位", queuePos)
		}
		return "项目已加入启动队列"
	default:
		return "项目启动请求已受理"
	}
}
