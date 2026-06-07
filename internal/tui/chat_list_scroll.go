package tui

func (m *model) chatYOffset() int {
	m.syncLegacyViewportOffset()
	return m.chatList.YOffset
}

func (m *model) usingLegacyViewportContent() bool {
	if m.viewport.TotalLineCount() == 0 {
		return false
	}
	if m.chatList.TotalHeight == 0 {
		return true
	}
	return m.viewport.TotalLineCount() > m.chatList.TotalHeight && m.chatList.TotalHeight <= m.chatList.Height
}

func (m *model) syncLegacyViewportOffset() {
	if m.usingLegacyViewportContent() {
		m.chatList.YOffset = m.viewport.YOffset()
		m.chatList.TotalHeight = m.viewport.TotalLineCount()
	}
}

func (m *model) setChatYOffset(y int) {
	m.ensureChatList()
	if m.usingLegacyViewportContent() {
		m.viewport.SetYOffset(y)
		m.chatList.YOffset = m.viewport.YOffset()
		return
	}
	m.chatList.YOffset = clampInt(y, 0, max(0, m.chatList.TotalHeight-m.chatList.Height))
	m.viewport.SetYOffset(m.chatList.YOffset)
}

func (m *model) chatTotalLineCount() int {
	m.ensureChatList()
	if m.usingLegacyViewportContent() {
		return m.viewport.TotalLineCount()
	}
	return m.chatList.TotalHeight
}

func (m *model) chatAtBottom() bool {
	m.ensureChatList()
	if m.usingLegacyViewportContent() {
		return m.viewport.AtBottom()
	}
	return m.chatList.YOffset >= max(0, m.chatList.TotalHeight-m.chatList.Height)
}

func (m *model) chatScrollUp(n int) {
	m.ensureChatList()
	m.autoScroll = false
	if m.usingLegacyViewportContent() {
		m.viewport.ScrollUp(max(1, n))
		m.chatList.YOffset = m.viewport.YOffset()
		return
	}
	m.chatList.YOffset = clampInt(m.chatList.YOffset-max(1, n), 0, max(0, m.chatList.TotalHeight-m.chatList.Height))
	m.viewport.SetYOffset(m.chatList.YOffset)
}

func (m *model) chatScrollDown(n int) {
	m.ensureChatList()
	if m.usingLegacyViewportContent() {
		m.viewport.SetYOffset(clampInt(m.viewport.YOffset()+max(1, n), 0, max(0, m.viewport.TotalLineCount()-m.viewport.Height())))
		m.chatList.YOffset = m.viewport.YOffset()
		m.autoScroll = m.viewport.AtBottom()
		return
	}
	m.chatList.YOffset = clampInt(m.chatList.YOffset+max(1, n), 0, max(0, m.chatList.TotalHeight-m.chatList.Height))
	m.viewport.SetYOffset(m.chatList.YOffset)
	m.autoScroll = m.chatAtBottom()
}

func (m *model) chatPageUp() {
	m.ensureChatList()
	m.autoScroll = false
	m.chatScrollUp(max(1, m.chatList.Height))
}

func (m *model) chatPageDown() {
	m.ensureChatList()
	m.chatScrollDown(max(1, m.chatList.Height))
}

func (m *model) chatGotoBottom() {
	m.ensureChatList()
	m.chatList.YOffset = max(0, m.chatList.TotalHeight-m.chatList.Height)
	m.autoScroll = true
}

func (m *model) chatGotoTop() {
	m.ensureChatList()
	m.chatList.YOffset = 0
	m.autoScroll = false
}
