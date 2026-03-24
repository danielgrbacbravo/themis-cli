package login

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Prefill struct {
	Username     string
	Password     string
	SaveUsername bool
	SavePassword bool
}

type SubmitRequest struct {
	Username     string
	Password     string
	TOTP         string
	SaveUsername bool
	SavePassword bool
}

type SubmitResult struct {
	UserFullName string
	UserEmail    string
}

type SubmitFunc func(req SubmitRequest) (SubmitResult, error)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "75"})
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "243", Dark: "247"})
	focusStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "39", Dark: "81"})
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"})
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "246"})
)

func Run(prefill Prefill, submit SubmitFunc) (SubmitResult, error) {
	m := newModel(prefill, submit)
	program := tea.NewProgram(m, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return SubmitResult{}, err
	}
	m, ok := final.(model)
	if !ok {
		return SubmitResult{}, fmt.Errorf("unexpected login model type")
	}
	if m.canceled {
		return SubmitResult{}, errors.New("login canceled")
	}
	if m.submitErr != nil {
		return SubmitResult{}, m.submitErr
	}
	return m.submitResult, nil
}

type submitFinishedMsg struct {
	result SubmitResult
	err    error
}

type model struct {
	usernameInput textinput.Model
	passwordInput textinput.Model
	totpInput     textinput.Model
	saveUsername  bool
	savePassword  bool
	focusIndex    int
	submitting    bool
	submitResult  SubmitResult
	submitErr     error
	statusLine    string
	canceled      bool
	submit        SubmitFunc
}

func newModel(prefill Prefill, submit SubmitFunc) model {
	username := textinput.New()
	username.Placeholder = "s1234567"
	username.SetValue(strings.TrimSpace(prefill.Username))
	username.Focus()

	password := textinput.New()
	password.Placeholder = "password"
	password.EchoMode = textinput.EchoPassword
	password.EchoCharacter = '*'
	password.SetValue(prefill.Password)

	totp := textinput.New()
	totp.Placeholder = "123456"
	totp.CharLimit = 12

	if prefill.SavePassword && !prefill.SaveUsername {
		prefill.SaveUsername = true
	}

	return model{
		usernameInput: username,
		passwordInput: password,
		totpInput:     totp,
		saveUsername:  prefill.SaveUsername,
		savePassword:  prefill.SavePassword,
		focusIndex:    0,
		submit:        submit,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.submitting {
		switch msg := msg.(type) {
		case submitFinishedMsg:
			m.submitting = false
			m.submitResult = msg.result
			m.submitErr = msg.err
			if msg.err != nil {
				m.statusLine = errorStyle.Render(msg.err.Error())
				return m, nil
			}
			m.statusLine = successStyle.Render(fmt.Sprintf("Login successful: %s (%s)", msg.result.UserFullName, msg.result.UserEmail))
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.canceled = true
			return m, tea.Quit
		case "tab", "shift+tab", "up", "down":
			m.moveFocus(msg.String())
			return m, nil
		case "enter":
			if m.focusIndex == 5 {
				return m.startSubmit()
			}
			m.moveFocus("tab")
			return m, nil
		case " ":
			if m.focusIndex == 3 {
				m.saveUsername = !m.saveUsername
				if !m.saveUsername {
					m.savePassword = false
				}
				return m, nil
			}
			if m.focusIndex == 4 {
				m.savePassword = !m.savePassword
				if m.savePassword {
					m.saveUsername = true
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.usernameInput, cmd = m.usernameInput.Update(msg)
	case 1:
		m.passwordInput, cmd = m.passwordInput.Update(msg)
	case 2:
		m.totpInput, cmd = m.totpInput.Update(msg)
	}
	return m, cmd
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Themis Login"))
	b.WriteString("\n\n")
	b.WriteString(m.renderField("Username", m.usernameInput.View(), 0))
	b.WriteString("\n")
	b.WriteString(m.renderField("Password", m.passwordInput.View(), 1))
	b.WriteString("\n")
	b.WriteString(m.renderField("TOTP", m.totpInput.View(), 2))
	b.WriteString("\n")
	b.WriteString(m.renderToggle("Save username", m.saveUsername, 3))
	b.WriteString("\n")
	b.WriteString(m.renderToggle("Save password", m.savePassword, 4))
	b.WriteString("\n\n")

	submitLabel := "[ Submit ]"
	if m.submitting {
		submitLabel = "[ Submitting... ]"
	}
	if m.focusIndex == 5 {
		submitLabel = focusStyle.Render(submitLabel)
	}
	b.WriteString(submitLabel)
	b.WriteString("\n\n")

	if strings.TrimSpace(m.statusLine) != "" {
		b.WriteString(m.statusLine)
		b.WriteString("\n")
	} else {
		b.WriteString(mutedStyle.Render("Tab/Shift+Tab move focus, Space toggles options, Enter submits, Esc cancels"))
		b.WriteString("\n")
	}
	return b.String()
}

func (m *model) moveFocus(key string) {
	m.usernameInput.Blur()
	m.passwordInput.Blur()
	m.totpInput.Blur()

	switch key {
	case "up", "shift+tab":
		m.focusIndex--
	default:
		m.focusIndex++
	}

	if m.focusIndex > 5 {
		m.focusIndex = 0
	}
	if m.focusIndex < 0 {
		m.focusIndex = 5
	}

	switch m.focusIndex {
	case 0:
		m.usernameInput.Focus()
	case 1:
		m.passwordInput.Focus()
	case 2:
		m.totpInput.Focus()
	}
}

func (m model) renderField(label string, value string, idx int) string {
	prefix := labelStyle.Render(label + ": ")
	if m.focusIndex == idx {
		prefix = focusStyle.Render(label + ": ")
	}
	return prefix + value
}

func (m model) renderToggle(label string, value bool, idx int) string {
	checked := "[ ]"
	if value {
		checked = "[x]"
	}
	line := fmt.Sprintf("%s %s", checked, label)
	if m.focusIndex == idx {
		return focusStyle.Render(line)
	}
	return labelStyle.Render(line)
}

func (m model) startSubmit() (tea.Model, tea.Cmd) {
	username := strings.TrimSpace(m.usernameInput.Value())
	password := m.passwordInput.Value()
	totp := strings.TrimSpace(m.totpInput.Value())
	if username == "" || password == "" || totp == "" {
		m.statusLine = errorStyle.Render("Username, password, and TOTP are required.")
		return m, nil
	}

	req := SubmitRequest{
		Username:     username,
		Password:     password,
		TOTP:         totp,
		SaveUsername: m.saveUsername,
		SavePassword: m.savePassword,
	}
	m.submitting = true
	m.statusLine = ""

	return m, func() tea.Msg {
		result, err := m.submit(req)
		return submitFinishedMsg{result: result, err: err}
	}
}
