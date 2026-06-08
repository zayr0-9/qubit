package tui

import (
	"strings"
	"time"
)

func (m *model) applySessionList(ev runtimeEvent) {
	wasStreaming := m.streaming
	m.sessions = mergeSessionActivity(m.sessions, ev.Sessions)
	if m.session == "" && ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if !m.autoNewSessionOnChat {
		if ev.SessionTitle != "" && ev.SessionID == m.session {
			m.title = ev.SessionTitle
		} else if !m.busy {
			m.title = m.currentSessionTitle()
		}
	}
	if wasStreaming {
		m.streaming = true
		m.status = "responding"
	} else if !(m.busy && m.activeRunID != "") {
		m.status = "ready"
		m.busy = false
		m.lastRunStartedSession = ""
		m.activeRunID = ""
		m.clearActiveRunStartedAt()
	}
	m.ensureSessionCursorInBounds()
}

func (m *model) touchLocalSessionActivity(sessionID string, title string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	for i := range m.sessions {
		if m.sessions[i].ID != sessionID {
			continue
		}
		if shouldReplaceSessionTitle(m.sessions[i].Title) && strings.TrimSpace(title) != "" {
			m.sessions[i].Title = title
		}
		if m.sessions[i].CreatedAt == "" {
			m.sessions[i].CreatedAt = now
		}
		m.sessions[i].UpdatedAt = now
		return
	}
	m.sessions = append(m.sessions, sessionInfo{ID: sessionID, Title: fallback(title, "New chat"), CreatedAt: now, UpdatedAt: now})
}

func (m model) latestUserMessageTitle() string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == "user" && strings.TrimSpace(m.messages[i].Content) != "" {
			return titleFromInput(m.messages[i].Content)
		}
	}
	return fallback(m.title, m.currentSessionTitle())
}

func mergeSessionActivity(local []sessionInfo, incoming []sessionInfo) []sessionInfo {
	if len(local) == 0 {
		filtered := incoming[:0]
		for _, session := range incoming {
			if !session.Hidden {
				filtered = append(filtered, session)
			}
		}
		return filtered
	}
	localByID := make(map[string]sessionInfo, len(local))
	for _, session := range local {
		if session.ID != "" {
			localByID[session.ID] = session
		}
	}
	merged := make([]sessionInfo, 0, max(len(local), len(incoming)))
	seen := make(map[string]bool, len(incoming))
	for _, session := range incoming {
		if session.Hidden {
			continue
		}
		seen[session.ID] = true
		localSession, hasLocal := localByID[session.ID]
		if hasLocal && session.FavouritedAt == "" {
			session.FavouritedAt = localSession.FavouritedAt
		}
		if hasLocal && sessionRecentTimestamp(localSession) > sessionRecentTimestamp(session) {
			if localSession.UpdatedAt != "" {
				session.UpdatedAt = localSession.UpdatedAt
			}
			if session.CreatedAt == "" {
				session.CreatedAt = localSession.CreatedAt
			}
			if shouldReplaceSessionTitle(session.Title) && localSession.Title != "" {
				session.Title = localSession.Title
			}
		}
		merged = append(merged, session)
	}
	for _, session := range local {
		if session.ID != "" && !session.Hidden && !seen[session.ID] {
			merged = append(merged, session)
		}
	}
	return merged
}

func (m *model) applySessionFavourited(ev runtimeEvent) {
	m.busy = false
	m.status = "session favourited"
	session := ev.Session
	if session == nil && ev.SessionID != "" {
		fallbackSession := sessionInfo{ID: ev.SessionID, Title: ev.SessionTitle, FavouritedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}
		session = &fallbackSession
	}
	if session != nil {
		if session.FavouritedAt == "" {
			session.FavouritedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		}
		m.upsertSessionInfo(*session)
		if session.ID == m.session && session.Title != "" {
			m.title = session.Title
		}
	}
}

func (m *model) upsertSessionInfo(session sessionInfo) {
	if session.ID == "" {
		return
	}
	for i := range m.sessions {
		if m.sessions[i].ID == session.ID {
			m.sessions[i] = session
			return
		}
	}
	m.sessions = append(m.sessions, session)
}

func shouldReplaceSessionTitle(title string) bool {
	switch strings.TrimSpace(title) {
	case "", "New chat", "Default chat", "Untitled chat":
		return true
	default:
		return false
	}
}

func (m *model) applySessionDeleted(ev runtimeEvent) {
	m.busy = false
	m.status = "ready"
	deletedID := ev.SessionID
	if deletedID == "" {
		deletedID = m.pendingDeleteSession.ID
	}
	filtered := m.sessions[:0]
	for _, session := range m.sessions {
		if session.ID != deletedID {
			filtered = append(filtered, session)
		}
	}
	m.sessions = filtered
	m.pendingDeleteSession = sessionInfo{}
	if m.session == deletedID {
		m.session = ""
		m.title = ""
		m.messages = []chatMessage{{Role: "assistant", Content: "Session deleted. Choose another session or start a new chat."}}
		m.autoNewSessionOnChat = true
		m.refreshViewport()
	}
	m.ensureSessionCursorInBounds()
}

func (m *model) applySessionMessages(ev runtimeEvent) {
	if ev.SessionID != "" && ev.SessionID != m.session {
		return
	}
	if m.activeRunID != "" || m.streaming {
		if ev.ID == "" || ev.ID != m.transcriptLoadRunID || (m.transcriptLoadSession != "" && ev.SessionID != "" && ev.SessionID != m.transcriptLoadSession) {
			return
		}
	}
	m.clearFakeStream()
	m.activeRunID = ""
	m.transcriptLoadRunID = ""
	m.transcriptLoadSession = ""
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if ev.SessionTitle != "" {
		m.title = ev.SessionTitle
	}
	m.busy = false
	m.status = "ready"
	m.err = ""
	m.autoScroll = true
	if len(ev.Messages) == 0 {
		m.messages = []chatMessage{{Role: "assistant", Content: "No messages in this session yet."}}
		m.lastCodexUsage = nil
	} else {
		m.messages = ev.Messages
		m.markCompactionMessages()
		m.lastCodexUsage = latestCodexUsageFromMessages(m.messages)
	}
	m.layout()
	m.refreshViewport()
}

func (m model) currentSessionTitle() string {
	for _, session := range m.sessions {
		if session.ID == m.session {
			return session.Title
		}
	}
	return ""
}

func (m *model) applyChatCompacted(ev runtimeEvent) {
	m.clearFakeStream()
	m.busy = false
	m.compacting = false
	m.activeRunID = ""
	m.clearActiveRunStartedAt()
	m.transcriptLoadRunID = ""
	m.transcriptLoadSession = ""
	if ev.SourceSessionID != "" {
		m.lastCompactedSource = ev.SourceSessionID
	}
	if ev.SessionID != "" {
		m.session = ev.SessionID
	}
	if ev.SessionTitle != "" {
		m.title = ev.SessionTitle
	}
	if ev.Session != nil {
		m.upsertSessionInfo(*ev.Session)
	}
	m.autoNewSessionOnChat = false
	m.autoScroll = true
	m.err = ""
	m.status = "compacted"
	m.pendingCompactInput = ""
	if len(ev.Messages) > 0 {
		m.messages = ev.Messages
	} else if ev.Marker != "" {
		m.messages = []chatMessage{{Role: "assistant", Content: ev.Marker, MessageKind: messageKindCompaction}}
	} else {
		m.messages = []chatMessage{{Role: "assistant", Content: ">summarised session", MessageKind: messageKindCompaction}}
	}
	m.markCompactionMessages()
	m.lastCodexUsage = nil
	m.layout()
	m.refreshViewport()
}

func (m *model) markCompactionMessages() {
	for i := range m.messages {
		if strings.HasPrefix(strings.TrimSpace(m.messages[i].Content), ">summarised session") {
			m.messages[i].MessageKind = messageKindCompaction
		}
	}
}
