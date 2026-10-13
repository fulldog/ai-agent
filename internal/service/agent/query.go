package agent

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"gorm.io/gorm"
)

var errUIDRequired = errors.New("uid required")

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

func (s *Service) ListTools() []ToolInfo {
	if s == nil {
		return nil
	}
	listed := s.registry.List()
	var defaults []string
	if s.cfg != nil {
		defaults = s.cfg.Agent.DefaultTools
	}
	want := make(map[string]struct{}, len(defaults))
	for _, n := range defaults {
		want[n] = struct{}{}
	}
	out := make([]ToolInfo, 0, len(listed))
	for _, t := range listed {
		_, def := want[t.Name]
		out = append(out, ToolInfo{Name: t.Name, Description: t.Description, Default: def})
	}
	return out
}

type ListRunsInput struct {
	UID            string
	All            bool
	ConversationID *uuid.UUID
	Status         string
	Limit          int
	Offset         int
}

func normalizeListRuns(uid string, all bool, limit, offset int) (string, int, int, error) {
	uid = strings.TrimSpace(uid)
	if !all && uid == "" {
		return "", 0, 0, errUIDRequired
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return uid, limit, offset, nil
}

func runUIDMatch(run *model.AgentRun, uid string, admin bool) bool {
	if admin {
		return true
	}
	return run != nil && run.UID != "" && run.UID == uid
}

func (s *Service) GetRun(id uuid.UUID) (*model.AgentRun, []model.AgentStep, error) {
	return s.GetRunFor(id, "", true)
}

func (s *Service) GetRunFor(id uuid.UUID, uid string, admin bool) (*model.AgentRun, []model.AgentStep, error) {
	var run model.AgentRun
	if err := s.db.First(&run, "id = ?", id).Error; err != nil {
		return nil, nil, err
	}
	if !runUIDMatch(&run, uid, admin) && !s.runOwnedViaConversation(&run, uid) {
		return nil, nil, gorm.ErrRecordNotFound
	}
	var steps []model.AgentStep
	if err := s.db.Where("run_id = ?", id).Order("step_index asc").Find(&steps).Error; err != nil {
		return nil, nil, err
	}
	return &run, steps, nil
}

func (s *Service) runOwnedViaConversation(run *model.AgentRun, uid string) bool {
	if run == nil || run.ConversationID == nil || strings.TrimSpace(uid) == "" {
		return false
	}
	if run.UID != "" {
		return false
	}
	var n int64
	if err := s.db.Model(&model.Conversation{}).
		Where("id = ? AND uid = ?", *run.ConversationID, uid).
		Count(&n).Error; err != nil {
		return false
	}
	return n > 0
}

// ListRuns 分页列出 Agent 运行。all=true 时列出全库（可选 uid 过滤）；否则必须带 uid。
func (s *Service) ListRuns(in ListRunsInput) ([]model.AgentRun, int64, error) {
	uid, limit, offset, err := normalizeListRuns(in.UID, in.All, in.Limit, in.Offset)
	if err != nil {
		return nil, 0, err
	}
	q := s.db.Model(&model.AgentRun{})
	if uid != "" {
		if in.All {
			q = q.Where("uid = ?", uid)
		} else {
			q = q.Where(
				"uid = ? OR (uid = '' AND conversation_id IN (SELECT id FROM conversations WHERE uid = ? AND deleted_at IS NULL))",
				uid, uid,
			)
		}
	}
	if in.ConversationID != nil {
		q = q.Where("conversation_id = ?", *in.ConversationID)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.AgentRun
	err = q.Order("created_at desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}
