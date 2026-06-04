package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	maxPlanClarificationOverlayRows = 14
	manualPlanClarificationOptionID = "manual"
)

func (m model) hasPlanClarification() bool {
	return m.planClarification.Active && len(m.planClarification.Questions) > 0
}

func (m model) openPlanClarification(ev runtimeEvent) model {
	questions := normalizePlanClarificationQuestions(ev.Questions)
	manual := newComposer()
	manual.minHeight = 1
	manual.maxHeight = 3
	manual.placeholder = "tell Qubit what to do..."
	manual.SetWidth(max(1, m.width-16))
	state := planClarificationState{
		Active:     true,
		RequestID:  ev.ID,
		SessionID:  ev.SessionID,
		RunID:      ev.RunID,
		Step:       ev.Step,
		ToolCallID: ev.ToolCallID,
		Questions:  questions,
		Manual:     manual,
	}
	if len(questions) > 0 && len(questions[0].Options) == 1 && questions[0].Options[0].Manual {
		state.OptionCursor = 0
	}
	m.planClarification = state
	m.status = "clarification needed"
	m.layout()
	return m
}

func normalizePlanClarificationQuestions(input []planClarificationQuestion) []planClarificationQuestion {
	questions := make([]planClarificationQuestion, 0, len(input))
	for i, question := range input {
		text := strings.TrimSpace(question.Question)
		if text == "" {
			continue
		}
		if strings.TrimSpace(question.ID) == "" {
			question.ID = fmt.Sprintf("question-%d", i+1)
		}
		question.Question = text
		question.Description = strings.TrimSpace(question.Description)
		options := make([]planClarificationOption, 0, len(question.Options)+1)
		for j, option := range question.Options {
			label := strings.TrimSpace(option.Label)
			if label == "" {
				continue
			}
			if strings.TrimSpace(option.ID) == "" {
				option.ID = fmt.Sprintf("option-%d", j+1)
			}
			option.Label = label
			option.Description = strings.TrimSpace(option.Description)
			options = append(options, option)
		}
		if len(options) == 0 || !options[len(options)-1].Manual {
			options = append(options, planClarificationOption{ID: manualPlanClarificationOptionID, Label: "None of the above — tell Qubit what to do instead", Manual: true})
		}
		question.Options = options
		questions = append(questions, question)
	}
	return questions
}

func (m model) updatePlanClarificationKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if !m.hasPlanClarification() {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.cancelPlanClarification()
	case "up", "ctrl+p":
		m.movePlanClarificationOption(-1)
		m.layout()
		return m, nil
	case "down", "ctrl+n":
		m.movePlanClarificationOption(1)
		m.layout()
		return m, nil
	case "enter":
		return m.answerPlanClarification()
	}
	if m.planClarificationManualSelected() || len(m.currentPlanClarificationQuestion().Options) == 0 {
		handled, cmd := m.planClarification.Manual.UpdateKey(msg)
		if handled {
			m.layout()
			return m, cmd
		}
	}
	return m, nil
}

func (m model) updatePlanClarificationPaste(text string) model {
	if !m.hasPlanClarification() || !m.planClarificationManualSelected() {
		return m
	}
	m.planClarification.Manual.InsertString(text)
	m.layout()
	return m
}

func (m *model) movePlanClarificationOption(delta int) {
	question := m.currentPlanClarificationQuestion()
	if len(question.Options) == 0 {
		m.planClarification.OptionCursor = 0
		return
	}
	m.planClarification.OptionCursor = (m.planClarification.OptionCursor + delta + len(question.Options)) % len(question.Options)
}

func (m model) currentPlanClarificationQuestion() planClarificationQuestion {
	if !m.hasPlanClarification() {
		return planClarificationQuestion{}
	}
	idx := clampInt(m.planClarification.Index, 0, len(m.planClarification.Questions)-1)
	return m.planClarification.Questions[idx]
}

func (m model) currentPlanClarificationOption() planClarificationOption {
	question := m.currentPlanClarificationQuestion()
	if len(question.Options) == 0 {
		return planClarificationOption{ID: manualPlanClarificationOptionID, Label: "Manual answer", Manual: true}
	}
	idx := clampInt(m.planClarification.OptionCursor, 0, len(question.Options)-1)
	return question.Options[idx]
}

func (m model) planClarificationManualSelected() bool {
	return m.currentPlanClarificationOption().Manual
}

func (m model) answerPlanClarification() (tea.Model, tea.Cmd) {
	question := m.currentPlanClarificationQuestion()
	option := m.currentPlanClarificationOption()
	answerText := option.Label
	if option.Manual {
		answerText = strings.TrimSpace(normalizeInputNewlines(m.planClarification.Manual.Value()))
		if answerText == "" {
			m.status = "enter a manual answer"
			return m, nil
		}
	}
	answer := planClarificationAnswer{
		QuestionID:          question.ID,
		Question:            question.Question,
		SelectedOptionID:    option.ID,
		SelectedOptionLabel: option.Label,
		Manual:              option.Manual,
		Answer:              answerText,
	}
	m.planClarification.Answers = append(m.planClarification.Answers, answer)
	m.planClarification.Index++
	if m.planClarification.Index >= len(m.planClarification.Questions) {
		return m.finishPlanClarification(false)
	}
	m.planClarification.OptionCursor = 0
	m.planClarification.Manual.Reset()
	if q := m.currentPlanClarificationQuestion(); len(q.Options) == 1 && q.Options[0].Manual {
		m.planClarification.OptionCursor = 0
	}
	m.status = "clarification needed"
	m.layout()
	return m, nil
}

func (m model) cancelPlanClarification() (tea.Model, tea.Cmd) {
	return m.finishPlanClarification(true)
}

func (m model) finishPlanClarification(cancelled bool) (tea.Model, tea.Cmd) {
	state := m.planClarification
	m.planClarification = planClarificationState{}
	m.status = "thinking"
	m.layout()
	payload := map[string]any{"type": "plan.clarification.response", "id": state.RequestID}
	if cancelled {
		payload["cancelled"] = true
	} else {
		payload["answers"] = state.Answers
	}
	return m, tea.Batch(sendRuntime(m.runtime, payload), waitRuntimeEvent(m.runtime))
}

func (m model) renderPlanClarificationOverlay(maxHeight int) string {
	if !m.hasPlanClarification() || maxHeight <= 0 {
		return ""
	}
	panelWidth := min(max(44, m.width-8), 96)
	contentWidth := max(20, panelWidth-6)
	question := m.currentPlanClarificationQuestion()
	var b strings.Builder
	icon := lipgloss.NewStyle().Foreground(accent).Render("✦")
	title := fmt.Sprintf("clarification · %d/%d", m.planClarification.Index+1, len(m.planClarification.Questions))
	b.WriteString(icon + " " + lipgloss.NewStyle().Foreground(accent).Bold(true).Render(title))
	b.WriteString("\n")
	b.WriteString(wrap(question.Question, contentWidth))
	if question.Description != "" {
		b.WriteString("\n")
		b.WriteString(mutedSt.Render(wrap(question.Description, contentWidth)))
	}
	if len(question.Options) > 0 {
		b.WriteString("\n")
		optionRows := max(1, maxHeight-renderedLineCount(b.String())-4-m.planClarificationManualHeight())
		window := visibleListWindow(len(question.Options), m.planClarification.OptionCursor, optionRows)
		if window.HasAbove {
			b.WriteString(mutedSt.Render(fmt.Sprintf("  more above (%d)", window.Start)))
			b.WriteString("\n")
		}
		for i := window.Start; i < window.End; i++ {
			option := question.Options[i]
			marker := mutedSt.Render("  ")
			labelStyle := lipgloss.NewStyle().Foreground(cyan).Bold(true)
			explanationStyle := mutedSt
			if option.Manual {
				labelStyle = labelStyle.Italic(true)
			}
			if i == m.planClarification.OptionCursor {
				marker = selectSt.Render("› ")
				labelStyle = selectSt.Bold(true)
			}
			optionPrefix := fmt.Sprintf("%d. ", i+1)
			labelWidth := max(8, contentWidth-lipgloss.Width(optionPrefix)-2)
			labelLines := strings.Split(wrap(option.Label, labelWidth), "\n")
			for j, line := range labelLines {
				lineMarker := marker
				prefix := optionPrefix
				if j > 0 {
					lineMarker = mutedSt.Render("  ")
					prefix = strings.Repeat(" ", lipgloss.Width(optionPrefix))
				}
				b.WriteString(lineMarker + labelStyle.Render(prefix+line))
				b.WriteString("\n")
			}
			if option.Description != "" {
				explanationWidth := max(8, contentWidth-4)
				for _, line := range strings.Split(wrap(option.Description, explanationWidth), "\n") {
					b.WriteString(mutedSt.Render("  ") + explanationStyle.Render("  "+line))
					b.WriteString("\n")
				}
			}
			if i < window.End-1 || window.HasBelow {
				b.WriteString("\n")
			}
		}
		if window.HasBelow {
			b.WriteString(mutedSt.Render(fmt.Sprintf("  more below (%d)", len(question.Options)-window.End)))
		}
	}
	if m.planClarificationManualSelected() {
		b.WriteString("\n")
		manualWidth := max(1, contentWidth-lipgloss.Width(idleInputPrompt()))
		m.planClarification.Manual.SetWidth(manualWidth)
		manual := m.planClarification.Manual.ViewStyled(idleInputPrompt(), m.inputCursorPulse, lipgloss.Style{})
		b.WriteString(renderFixedHeight(manual, m.planClarification.Manual.Height()))
	}
	body := renderAccentBorderedPanel(b.String(), panelWidth)
	return inputStyle.Width(m.width).Render(body)
}

func (m model) planClarificationManualHeight() int {
	if !m.planClarificationManualSelected() {
		return 0
	}
	return m.planClarification.Manual.Height() + 1
}
