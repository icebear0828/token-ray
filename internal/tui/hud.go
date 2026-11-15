package tui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"ai-flight-dashboard/internal/alert"
	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	calc         *calculator.Calculator
	watch        *watcher.Watcher
	database     *db.DB
	budget       *alert.BudgetAlert
	usages       []watcher.TokenUsage
	totalCost    float64
	periods      []model.PeriodCost
	budgetStatus *alert.BudgetStatus
	quitting     bool
	eventCount   int // total events seen (usages slice is capped)
}

const maxLiveEvents = 50 // keep last N events in memory for display

func NewModel(c *calculator.Calculator, w *watcher.Watcher, d *db.DB, ba *alert.BudgetAlert) Model {
	return Model{
		calc:     c,
		watch:    w,
		database: d,
		budget:   ba,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.waitForUsage(), m.refreshPeriods())
}

type usageMsg watcher.TokenUsage
type periodsMsg []model.PeriodCost
type tickMsg struct{}

func (m Model) waitForUsage() tea.Cmd {
	return func() tea.Msg {
		u := <-m.watch.UsageChan
		return usageMsg(u)
	}
}

func (m Model) refreshPeriods() tea.Cmd {
	return func() tea.Msg {
		if m.database == nil {
			return periodsMsg(nil)
		}
		periods, err := dashboard.BuildPeriodCosts(m.database, m.watch.DeviceID, "")
		if err != nil {
			log.Printf("TUI DB query error: %v", err)
		}
		return periodsMsg(periods)
	}
}

func (m Model) tickRefresh() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case usageMsg:
		u := watcher.TokenUsage(msg)
		m.usages = append(m.usages, u)
		// Cap memory: keep only last N events
		if len(m.usages) > maxLiveEvents {
			m.usages = m.usages[len(m.usages)-maxLiveEvents:]
		}
		m.eventCount++
		cost, _ := m.calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)
		m.totalCost += cost

		return m, tea.Batch(m.waitForUsage(), m.refreshPeriods())
	case periodsMsg:
		m.periods = []model.PeriodCost(msg)
		// Refresh budget status alongside periods
		if m.budget != nil {
			s := m.budget.Check()
			m.budgetStatus = &s
		}
		return m, m.tickRefresh()
	case tickMsg:
		return m, m.refreshPeriods()
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Shutting down AI Flight Dashboard...\n"
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	costStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Width(8)
	tableHeader := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))

	var b strings.Builder
	b.WriteString(header.Render("✈️  AI Flight Dashboard") + "\n\n")

	// Time-window cost table
	if len(m.periods) > 0 {
		b.WriteString(tableHeader.Render(" Period   │ Cost (USD)") + "\n")
		b.WriteString(dimStyle.Render("──────────┼───────────") + "\n")
		for _, p := range m.periods {
			costStr := costStyle.Render(fmt.Sprintf("$%.4f", p.Cost))
			b.WriteString(fmt.Sprintf(" %s│ %s\n", labelStyle.Render(p.Label), costStr))
		}
		b.WriteString("\n")
	}

	// Live event counter
	b.WriteString(fmt.Sprintf(" Live Events: %d", m.eventCount))
	if len(m.usages) > 0 {
		last := m.usages[len(m.usages)-1]
		b.WriteString(dimStyle.Render(fmt.Sprintf(" │ %s %s │ In:%d CaR:%d CaW:%d Out:%d",
			last.Source, last.Model, last.InputTokens, last.CachedTokens, last.CacheCreationTokens, last.OutputTokens)))
	}
	b.WriteString("\n")

	// Budget alert bar
	if m.budgetStatus != nil {
		var budgetColor string
		var icon string
		switch m.budgetStatus.Level {
		case alert.LevelGreen:
			budgetColor = "42"
			icon = "🟢"
		case alert.LevelYellow:
			budgetColor = "220"
			icon = "🟡"
		case alert.LevelRed:
			budgetColor = "196"
			icon = "🔴"
		}
		budgetStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(budgetColor))
		if m.budgetStatus.Exceeded {
			b.WriteString(budgetStyle.Render(fmt.Sprintf(" ❌ BUDGET EXCEEDED! Spent: $%.2f", m.budgetStatus.Spent)) + "\n")
		} else {
			b.WriteString(fmt.Sprintf(" %s Budget: %s  Remaining: %s\n",
				icon,
				budgetStyle.Render(fmt.Sprintf("$%.2f (%.0f%%)", m.budgetStatus.Spent, m.budgetStatus.Percent)),
				costStyle.Render(fmt.Sprintf("$%.2f", m.budgetStatus.Remaining)),
			))
		}
	}

	b.WriteString(dimStyle.Render("\n [q] quit │ refreshes every 1s") + "\n")
	return b.String()
}
