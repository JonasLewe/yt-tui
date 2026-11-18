package ui

import (
	"fmt"
	"sort"
	"strings"
	"yt-tui/internal/config"
	"yt-tui/internal/ytcli"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type issuesCacheKey struct {
	serverURL, projectCode, query, agileID, sprintID, contextKey string
}

type projectCache struct {
	issues       []ytcli.Issue
	skip         int
	loadedAll    bool
	limitReached bool
}

type issueField struct {
	title string
	width int
	value func(ytcli.Issue) string
}

type issuesModel struct {
	client      *ytcli.Client
	cfg         *config.Config
	projectCode string // empty if general search
	query       string
	agileID     string
	sprintID    string
	boardName   string
	sprintName  string
	issues      []ytcli.Issue
	table       table.Model
	searchInput textinput.Model
	searchMode  bool
	filterMode  bool
	filterPanel issuesFilterPanel
	filterError string
	loading     bool
	err         error
	spinner     spinner.Model
	width       int
	height      int
	skip        int
	pageSize    int
	maxIssues   int
	loadedAll   bool
	cache       map[issuesCacheKey]projectCache
	serverURL   string
	requestID   uint64
	serverQuery string
	issueListFilters
	filtersByView       map[issueFilterView]issueListFilters
	contextKey          string
	tags                []ytcli.Tag
	tagsLoaded          bool
	tagsLoading         bool
	tagsRequestID       uint64
	tagsErr             error
	typeInfoLoading     bool
	typeInfo            ytcli.IssueTypeInfo
	typeInfoErr         error
	typeInfoCache       map[issueTypesKey]ytcli.IssueTypeInfo
	limitReached        bool
	fields              []issueField
	visibleIssueIDs     []string
	lastSelectedIssueID string
	sortMode            bool
	sortActiveSection   int // 0: Column list, 1: Direction list
	sortColCursor       int
	sortDirCursor       int
	tempSortCol         string
	tempSortDir         string
	actionMode          bool
	actionCursor        int
	loadingText         string
}

func newIssuesModel(client *ytcli.Client, cfg *config.Config) issuesModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet))

	pageSize := config.DefaultPageSize
	maxIssues := config.DefaultMaxIssues
	var fieldNames []string

	if cfg != nil {
		if cfg.PageSize > 0 {
			pageSize = cfg.PageSize
		}
		maxIssues = cfg.MaxIssues
		fieldNames = cfg.Fields
	}

	if len(fieldNames) == 0 {
		fieldNames = config.DefaultFields
	}

	var fields []issueField
	var columns []table.Column

	for _, name := range fieldNames {
		title := name
		width := 15
		var valueFn func(ytcli.Issue) string

		switch strings.ToLower(name) {
		case "id":
			title = "ID"
			width = 12
			valueFn = func(i ytcli.Issue) string { return i.IDReadable }
		case "summary":
			title = "Summary"
			width = 55
			valueFn = func(i ytcli.Issue) string { return i.Summary }
		case "state":
			title = "State"
			width = 15
			valueFn = func(i ytcli.Issue) string { return i.State() }
		case "priority":
			title = "Priority"
			width = 12
			valueFn = func(i ytcli.Issue) string { return i.Priority() }
		case "assignee":
			title = "Assignee"
			width = 20
			valueFn = func(i ytcli.Issue) string { return i.Assignee() }
		case "type":
			title = "Type"
			width = 12
			valueFn = func(i ytcli.Issue) string { return issueTypeLabel(i, cfg, "") }
		case "updated":
			title = "Updated"
			width = 20
			valueFn = func(i ytcli.Issue) string { return i.UpdatedTime() }
		case "updater":
			title = "Updater"
			width = 20
			valueFn = func(i ytcli.Issue) string { return i.UpdaterName() }
		case "created":
			title = "Created"
			width = 20
			valueFn = func(i ytcli.Issue) string { return i.CreatedTime() }
		case "creator", "reporter":
			title = "Creator"
			width = 20
			valueFn = func(i ytcli.Issue) string {
				if i.Reporter != nil {
					return i.Reporter.DisplayName()
				}
				return "N/A"
			}
		default:
			// Custom field
			valueFn = func(i ytcli.Issue) string { return i.ExtractStringField(name) }
		}

		fields = append(fields, issueField{
			title: title,
			width: width,
			value: valueFn,
		})

		columns = append(columns, table.Column{
			Title: title,
			Width: width,
		})
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
	)

	ts := table.DefaultStyles()
	ts.Header = ts.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(ColorOverlay)).
		BorderBottom(true).
		Bold(true)
	ts.Selected = ts.Selected.
		Foreground(lipgloss.Color(ColorBg)).
		Background(lipgloss.Color(ColorCyan)).
		Bold(true)
	t.SetStyles(ts)

	ti := textinput.New()
	ti.Placeholder = "Filter by ID or summary..."
	ti.Prompt = " 🔍 / "
	ti.Focus()

	m := issuesModel{
		client:        client,
		cfg:           cfg,
		table:         t,
		searchInput:   ti,
		loading:       true,
		spinner:       s,
		pageSize:      pageSize,
		maxIssues:     maxIssues,
		skip:          0,
		cache:         make(map[issuesCacheKey]projectCache),
		typeInfoCache: make(map[issueTypesKey]ytcli.IssueTypeInfo),
		filtersByView: make(map[issueFilterView]issueListFilters),
		fields:        fields,
	}
	m.updateTableColumns()
	return m
}

type issuesDataMsg struct {
	context     issuesCacheKey
	requestID   uint64
	skip, limit int
	issues      []ytcli.Issue
	err         error
}

func (m *issuesModel) cacheKey() issuesCacheKey {
	return issuesCacheKey{m.serverURL, m.projectCode, m.serverQuery, m.agileID, m.sprintID, m.contextKey}
}

func (m issuesModel) loadIssuesCmd() tea.Cmd {
	msg := issuesDataMsg{context: m.cacheKey(), requestID: m.requestID, skip: m.skip, limit: m.pageSize}
	if m.maxIssues > 0 && m.sprintID == "" {
		msg.limit = min(msg.limit, m.maxIssues-len(m.issues))
	}
	if msg.limit <= 0 {
		return nil
	}
	if m.client == nil {
		return func() tea.Msg {
			msg.err = fmt.Errorf("YouTrack client is unavailable")
			return msg
		}
	}
	// Requests use a snapshot so switching accounts cannot change an in-flight request.
	client := *m.client
	return func() tea.Msg {
		if msg.context.sprintID != "" {
			msg.issues, msg.err = client.ListSprintIssues(msg.context.agileID, msg.context.sprintID)
		} else {
			msg.issues, msg.err = client.ListIssues(msg.context.projectCode, msg.context.query, msg.limit, msg.skip)
		}
		return msg
	}
}

func (m *issuesModel) isVisible(issue ytcli.Issue) bool {
	return !excludedValue(m.excludedStates, issue.State()) && !excludedValue(m.excludedPriorities, issue.Priority())
}

func compareIDs(idA, idB string) int {
	partsA := strings.SplitN(idA, "-", 2)
	partsB := strings.SplitN(idB, "-", 2)
	if len(partsA) == 2 && len(partsB) == 2 {
		if partsA[0] != partsB[0] {
			return strings.Compare(partsA[0], partsB[0])
		}
		var numA, numB int
		_, errA := fmt.Sscan(partsA[1], &numA)
		_, errB := fmt.Sscan(partsB[1], &numB)
		if errA == nil && errB == nil {
			if numA < numB {
				return -1
			}
			if numA > numB {
				return 1
			}
			return 0
		}
	}
	return strings.Compare(idA, idB)
}

func priorityIndex(p string, priorities []string) int {
	for i, pr := range priorities {
		if strings.EqualFold(pr, p) {
			return i
		}
	}
	return -1
}

func stateIndex(s string, states []string) int {
	for i, st := range states {
		if strings.EqualFold(st, s) {
			return i
		}
	}
	return -1
}

func (m *issuesModel) getAvailableStates() []string {
	if m.cfg == nil {
		return config.DefaultStates
	}
	projects := make(map[string]bool)
	projCode := m.effectiveProjectCode()
	if projCode != "" {
		projects[projCode] = true
	}
	for _, issue := range m.issues {
		if issue.Project != nil {
			code := issue.Project.ShortName
			if code == "" {
				code = issue.Project.ID
			}
			if code != "" {
				projects[code] = true
			}
		}
	}

	if len(projects) == 0 {
		return m.cfg.GetCustomStates("")
	}

	seen := make(map[string]bool)
	var merged []string

	if projCode != "" {
		primaryStates := m.cfg.GetCustomStates(projCode)
		for _, s := range primaryStates {
			if !seen[s] {
				seen[s] = true
				merged = append(merged, s)
			}
		}
	}

	var codes []string
	for code := range projects {
		if code != projCode {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)

	for _, code := range codes {
		states := m.cfg.GetCustomStates(code)
		for _, s := range states {
			if !seen[s] {
				seen[s] = true
				merged = append(merged, s)
			}
		}
	}

	return merged
}

func (m *issuesModel) getAvailablePriorities() []string {
	if m.cfg == nil {
		return config.DefaultPriorities
	}
	projects := make(map[string]bool)
	projCode := m.effectiveProjectCode()
	if projCode != "" {
		projects[projCode] = true
	}
	for _, issue := range m.issues {
		if issue.Project != nil {
			code := issue.Project.ShortName
			if code == "" {
				code = issue.Project.ID
			}
			if code != "" {
				projects[code] = true
			}
		}
	}

	if len(projects) == 0 {
		return m.cfg.GetCustomPriorities("")
	}

	seen := make(map[string]bool)
	var merged []string

	if projCode != "" {
		primaryPriorities := m.cfg.GetCustomPriorities(projCode)
		for _, p := range primaryPriorities {
			if !seen[p] {
				seen[p] = true
				merged = append(merged, p)
			}
		}
	}

	var codes []string
	for code := range projects {
		if code != projCode {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)

	for _, code := range codes {
		priorities := m.cfg.GetCustomPriorities(code)
		for _, p := range priorities {
			if !seen[p] {
				seen[p] = true
				merged = append(merged, p)
			}
		}
	}

	return merged
}

func (m *issuesModel) compareIssues(a, b ytcli.Issue, fieldTitle string) int {
	var valA, valB string
	var valueFn func(ytcli.Issue) string
	for _, f := range m.fields {
		if strings.EqualFold(f.title, fieldTitle) {
			valueFn = f.value
			break
		}
	}
	if valueFn == nil {
		return 0
	}
	valA = valueFn(a)
	valB = valueFn(b)
	if strings.EqualFold(fieldTitle, "Type") {
		valA = issueTypeLabel(a, m.cfg, m.projectCode)
		valB = issueTypeLabel(b, m.cfg, m.projectCode)
	}

	titleLower := strings.ToLower(fieldTitle)
	if titleLower == "id" {
		return compareIDs(valA, valB)
	}

	if titleLower == "priority" && m.cfg != nil {
		projectCode := ""
		if a.Project != nil {
			projectCode = a.Project.ShortName
			if projectCode == "" {
				projectCode = a.Project.ID
			}
		}
		if projectCode == "" {
			projectCode = m.effectiveProjectCode()
		}
		priorities := m.cfg.GetCustomPriorities(projectCode)
		idxA := priorityIndex(valA, priorities)
		idxB := priorityIndex(valB, priorities)
		if idxA != -1 && idxB != -1 {
			if idxA < idxB {
				return -1
			}
			if idxA > idxB {
				return 1
			}
			return 0
		}
	}

	if titleLower == "state" && m.cfg != nil {
		projectCode := ""
		if a.Project != nil {
			projectCode = a.Project.ShortName
			if projectCode == "" {
				projectCode = a.Project.ID
			}
		}
		if projectCode == "" {
			projectCode = m.effectiveProjectCode()
		}
		states := m.cfg.GetCustomStates(projectCode)
		idxA := stateIndex(valA, states)
		idxB := stateIndex(valB, states)
		if idxA != -1 && idxB != -1 {
			if idxA < idxB {
				return -1
			}
			if idxA > idxB {
				return 1
			}
			return 0
		}
	}

	// Default case-insensitive string comparison
	return strings.Compare(strings.ToLower(valA), strings.ToLower(valB))
}

func (m *issuesModel) sortIssues() {
	if m.cfg == nil || m.cfg.SortColumn == "" {
		return
	}
	sort.SliceStable(m.issues, func(i, j int) bool {
		cmp := m.compareIssues(m.issues[i], m.issues[j], m.cfg.SortColumn)
		if strings.EqualFold(m.cfg.SortDirection, "desc") {
			return cmp > 0
		}
		return cmp < 0
	})
}

func (m *issuesModel) updateTableColumns() {
	var columns []table.Column
	widths := issueColumnWidths(m.fields, m.width)
	for i, f := range m.fields {
		title := f.title
		if m.cfg != nil && strings.EqualFold(f.title, m.cfg.SortColumn) {
			if strings.EqualFold(m.cfg.SortDirection, "desc") {
				title += " ▼"
			} else {
				title += " ▲"
			}
		}
		columns = append(columns, table.Column{
			Title: title,
			Width: widths[i],
		})
	}
	m.table.SetColumns(columns)
	if m.width > 0 {
		m.table.SetWidth(m.width)
	}
}

func (m *issuesModel) updateTableRows() {
	m.sortIssues()
	rows := []table.Row{}
	m.visibleIssueIDs = []string{}
	filterPhrase := strings.ToLower(m.searchInput.Value())

	for _, issue := range m.issues {
		if m.isVisible(issue) {
			if filterPhrase == "" ||
				strings.Contains(strings.ToLower(issue.Summary), filterPhrase) ||
				strings.Contains(strings.ToLower(issue.IDReadable), filterPhrase) {

				row := table.Row{}
				for _, f := range m.fields {
					value := f.value(issue)
					if f.title == "Type" {
						value = issueTypeLabel(issue, m.cfg, m.projectCode)
					}
					row = append(row, value)
				}
				rows = append(rows, row)
				m.visibleIssueIDs = append(m.visibleIssueIDs, issue.IDReadable)
			}
		}
	}
	m.table.SetRows(rows)
}

func (m *issuesModel) updateTableHeight() {
	tableHeight := m.height - 7 - strings.Count(m.filterSummary(), "\n") - strings.Count(m.issueHelp(), "\n")
	if m.searchMode {
		tableHeight -= 4
	}
	if tableHeight < 2 {
		tableHeight = 2 // Bubbles subtracts the two header rows from this height.
	}
	m.table.SetHeight(tableHeight)
}

func (m *issuesModel) invalidateCache(projectCode string) {
	// Global actions may affect any project. A stale typed view must never hide
	// a changed issue when the user returns from another context.
	if projectCode == "" {
		clear(m.cache)
		return
	}
	aliases := map[string]bool{projectCode: true}
	for _, info := range m.typeInfoCache {
		if info.Project.ID == projectCode || info.Project.ShortName == projectCode {
			aliases[info.Project.ID], aliases[info.Project.ShortName] = true, true
		}
	}
	for key := range m.cache {
		if aliases[key.projectCode] || key.projectCode == "" {
			delete(m.cache, key)
		}
	}
}

func (m *issuesModel) restoreCursor() {
	if m.lastSelectedIssueID != "" {
		for i, id := range m.visibleIssueIDs {
			if id == m.lastSelectedIssueID {
				m.table.SetCursor(i)
				return
			}
		}
	}
	if len(m.table.Rows()) > 0 {
		m.table.SetCursor(0)
	}
}

func (m *issuesModel) effectiveProjectCode() string {
	if m.projectCode != "" {
		return m.projectCode
	}
	if len(m.issues) == 0 {
		return ""
	}
	var singleProject string
	for _, issue := range m.issues {
		if issue.Project != nil {
			code := issue.Project.ShortName
			if code == "" {
				code = issue.Project.ID
			}
			if code != "" {
				if singleProject == "" {
					singleProject = code
				} else if singleProject != code {
					return ""
				}
			}
		}
	}
	return singleProject
}

func (m *issuesModel) selectedIssueProjectCode() string {
	cursor := m.table.Cursor()
	if cursor >= 0 && cursor < len(m.visibleIssueIDs) {
		selectedID := m.visibleIssueIDs[cursor]
		for _, issue := range m.issues {
			if issue.IDReadable == selectedID && issue.Project != nil {
				code := issue.Project.ShortName
				if code == "" {
					code = issue.Project.ID
				}
				return code
			}
		}
	}
	return m.effectiveProjectCode()
}

func (m *issuesModel) actionList() []config.ActionConfig {
	if m.cfg == nil {
		return nil
	}
	return m.cfg.GetActions(m.selectedIssueProjectCode())
}

func (m *issuesModel) initProject(projectCode string, isBack bool) tea.Cmd {
	return m.initContext(projectCode, "", isBack)
}

func (m *issuesModel) initContext(projectCode string, query string, isBack bool) tea.Cmd {
	m.rememberFilters()
	m.projectCode, m.query = projectCode, query
	m.issueListFilters = m.filtersByView[issueFilterView{projectCode, query}]
	m.searchInput.SetValue(m.issueListFilters.text)
	m.searchInput.Blur()
	m.searchMode = false
	m.agileID, m.sprintID, m.boardName, m.sprintName = "", "", "", ""
	m.filterMode, m.sortMode, m.actionMode = false, false, false
	if strings.HasPrefix(query, "sprint:") {
		parts := strings.SplitN(query, ":", 5)
		if len(parts) == 5 {
			m.agileID, m.sprintID, m.boardName, m.sprintName = parts[1], parts[2], parts[3], parts[4]
		}
	}
	if !isBack {
		m.lastSelectedIssueID = ""
	}
	return m.startContext(false)
}

func (m *issuesModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startContext(false))
}

func (m issuesModel) Update(msg tea.Msg) (issuesModel, tea.Cmd) {
	m.updateTableHeight()
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.updateTableColumns()
		m.updateTableHeight()
		return m, nil

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case issueTypesMsg:
		if msg.requestID != m.requestID || msg.key != m.typeKey() || !m.sameServer() {
			return m, nil
		}
		m.typeInfo, m.typeInfoErr = msg.info, msg.err
		m.typeInfoLoading = false
		if msg.err == nil {
			m.typeInfoCache[msg.key] = msg.info
		}
		if m.filterMode && len(m.filterPanel.sections) >= 3 {
			m.filterPanel.sections[2].Options = m.availableTypes()
		}
		cmd = m.restoreOrLoadIssues()
		return m, cmd

	case issueTagsMsg:
		if msg.contextKey != m.contextKey || msg.requestID != m.requestID || msg.metadataID != m.tagsRequestID || !m.sameServer() {
			return m, nil
		}
		m.tagsLoading, m.tagsLoaded = false, msg.err == nil
		m.tags, m.tagsErr = msg.tags, msg.err
		if m.filterMode && len(m.filterPanel.sections) > 3 {
			m.filterPanel.sections[3].Options = m.availableTags()
			m.filterPanel.clamp()
		}
		return m, nil

	case issuesDataMsg:
		if msg.context != m.cacheKey() || msg.requestID != m.requestID || msg.skip != m.skip || !m.sameServer() {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.skip == 0 {
			m.issues = msg.issues
		} else {
			m.issues = append(m.issues, msg.issues...)
		}
		m.limitReached = m.maxIssues > 0 && len(m.issues) >= m.maxIssues && m.sprintID == ""
		if m.limitReached && len(m.issues) > m.maxIssues {
			m.issues = m.issues[:m.maxIssues]
		}
		m.skip = msg.skip + len(msg.issues)
		m.loadedAll = len(msg.issues) < msg.limit || m.limitReached || m.sprintID != ""
		m.cache[m.cacheKey()] = projectCache{
			issues: append([]ytcli.Issue(nil), m.issues...), skip: m.skip,
			loadedAll: m.loadedAll, limitReached: m.limitReached,
		}
		m.updateTableRows()
		if msg.skip == 0 {
			m.restoreCursor()
		}
		if !m.loadedAll {
			return m, m.loadIssuesCmd()
		}
		return m, nil

	case actionFinishedMsg:
		m.loading = false
		m.loadingText = ""
		m.actionMode = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.invalidateCache(m.projectCode)
		cmd = m.startContext(false)
		return m, cmd

	case tea.KeyMsg:
		if m.actionMode && m.cfg != nil {
			switch msg.String() {
			case "esc", " ":
				m.actionMode = false
				return m, nil
			case "up", "k":
				m.actionCursor--
				if m.actionCursor < 0 {
					m.actionCursor = len(m.actionList()) - 1
				}
				return m, nil
			case "down", "j":
				m.actionCursor++
				if m.actionCursor >= len(m.actionList()) {
					m.actionCursor = 0
				}
				return m, nil
			case "enter":
				actions := m.actionList()
				if len(actions) > 0 && len(m.table.Rows()) > 0 {
					cursor := m.table.Cursor()
					if cursor >= 0 && cursor < len(m.visibleIssueIDs) {
						issueID := m.visibleIssueIDs[cursor]
						act := actions[m.actionCursor]
						m.loading = true
						m.loadingText = "Running action..."
						client := m.client
						return m, func() tea.Msg {
							err := executeAction(client, issueID, act)
							return actionFinishedMsg{err: err}
						}
					}
				}
				m.actionMode = false
				return m, nil
			default:
				// Check shortcuts
				for _, act := range m.actionList() {
					if msg.String() == act.Shortcut {
						if len(m.table.Rows()) > 0 {
							cursor := m.table.Cursor()
							if cursor >= 0 && cursor < len(m.visibleIssueIDs) {
								issueID := m.visibleIssueIDs[cursor]
								m.loading = true
								m.loadingText = "Running action..."
								client := m.client
								return m, func() tea.Msg {
									err := executeAction(client, issueID, act)
									return actionFinishedMsg{err: err}
								}
							}
						}
						m.actionMode = false
						return m, nil
					}
				}
			}
			return m, nil
		}

		if m.sortMode {
			numCols := len(m.fields)
			switch msg.String() {
			case "esc":
				m.sortMode = false
				return m, nil
			case "q":
				return m, tea.Quit
			case "left", "h":
				m.sortActiveSection = 0
				return m, nil
			case "right", "l":
				m.sortActiveSection = 1
				return m, nil
			case "up", "k":
				if m.sortActiveSection == 0 {
					m.sortColCursor--
					if m.sortColCursor < 0 {
						m.sortColCursor = numCols - 1
					}
				} else {
					m.sortDirCursor--
					if m.sortDirCursor < 0 {
						m.sortDirCursor = 1
					}
				}
				return m, nil
			case "down", "j":
				if m.sortActiveSection == 0 {
					m.sortColCursor++
					if m.sortColCursor >= numCols {
						m.sortColCursor = 0
					}
				} else {
					m.sortDirCursor++
					if m.sortDirCursor >= 2 {
						m.sortDirCursor = 0
					}
				}
				return m, nil
			case " ":
				if m.sortActiveSection == 0 {
					if m.sortColCursor >= 0 && m.sortColCursor < numCols {
						m.tempSortCol = m.fields[m.sortColCursor].title
					}
				} else {
					if m.sortDirCursor == 0 {
						m.tempSortDir = "asc"
					} else {
						m.tempSortDir = "desc"
					}
				}
				return m, nil
			case "enter":
				if m.sortActiveSection == 0 {
					if m.sortColCursor >= 0 && m.sortColCursor < numCols {
						m.tempSortCol = m.fields[m.sortColCursor].title
					}
				} else {
					if m.sortDirCursor == 0 {
						m.tempSortDir = "asc"
					} else {
						m.tempSortDir = "desc"
					}
				}
				if m.cfg != nil {
					m.cfg.SortColumn = m.tempSortCol
					// Empty direction inherits ascending order. Choosing a column
					// must not turn that inherited direction into an override.
					if strings.EqualFold(m.tempSortDir, "desc") != strings.EqualFold(m.cfg.SortDirection, "desc") {
						m.cfg.SortDirection = m.tempSortDir
					}
					_ = config.SaveConfig(m.cfg)
					m.updateTableColumns()
					m.updateTableRows()
				}
				m.sortMode = false
				return m, nil
			}
			return m, nil
		}

		if msg.String() == "ctrl+f" {
			cmd = m.clearFilters()
			return m, cmd
		}

		if m.filterMode {
			if m.filterPanel.searchMode {
				cmd = m.filterPanel.updateSearch(msg)
				return m, cmd
			}
			switch msg.String() {
			case "/":
				cmd = m.filterPanel.startSearch()
				return m, cmd
			case "esc":
				m.filterMode = false
				m.filterError = ""
			case " ":
				m.filterPanel.toggle()
			case "enter":
				cmd = m.saveFilters()
				return m, cmd
			default:
				m.filterPanel.move(msg.String())
			}
			return m, nil
		}

		if m.searchMode {
			switch msg.String() {
			case "enter", "esc":
				m.searchMode = false
				m.searchInput.Blur()
				m.updateTableHeight()
				return m, nil
			}
			oldValue := m.searchInput.Value()
			m.searchInput, cmd = m.searchInput.Update(msg)
			if m.searchInput.Value() != oldValue {
				m.updateTableRows()
				m.table.SetCursor(0)
			}
			return m, cmd
		}

		if m.loading && msg.String() != "f" && msg.String() != "F" && msg.String() != "r" && msg.String() != "esc" && msg.String() != "backspace" {
			return m, nil
		}

		switch msg.String() {
		case " ":
			if len(m.table.Rows()) > 0 {
				cursor := m.table.Cursor()
				if cursor >= 0 && cursor < len(m.visibleIssueIDs) && len(m.actionList()) > 0 {
					m.actionMode = true
					m.actionCursor = 0
				}
			}
			return m, nil
		case "esc", "backspace":
			return m, func() tea.Msg {
				return popStateMsg{}
			}
		case "/":
			m.searchMode = true
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			m.updateTableRows()
			m.updateTableHeight()
			return m, nil
		case "K":
			if m.projectCode != "" && m.projectCode != "ME" {
				projectCode := m.projectCode
				return m, func() tea.Msg {
					return pushStateMsg{state: stateArticles, data: projectCode}
				}
			}
		case "f", "F":
			cmd = m.openFilters()
			return m, cmd
		case "s":
			if m.cfg != nil {
				m.sortMode = true
				m.sortActiveSection = 0
				m.tempSortCol = m.cfg.SortColumn
				if m.tempSortCol == "" && len(m.fields) > 0 {
					m.tempSortCol = m.fields[0].title
				}
				m.tempSortDir = m.cfg.SortDirection
				if m.tempSortDir == "" {
					m.tempSortDir = "asc"
				}

				m.sortColCursor = 0
				for idx, f := range m.fields {
					if strings.EqualFold(f.title, m.tempSortCol) {
						m.sortColCursor = idx
						break
					}
				}
				if strings.EqualFold(m.tempSortDir, "desc") {
					m.sortDirCursor = 1
				} else {
					m.sortDirCursor = 0
				}
			}
			return m, nil
		case "enter":
			if len(m.table.Rows()) > 0 {
				cursor := m.table.Cursor()
				if cursor >= 0 && cursor < len(m.visibleIssueIDs) {
					issueKey := m.visibleIssueIDs[cursor]
					m.lastSelectedIssueID = issueKey
					return m, func() tea.Msg {
						return pushStateMsg{state: stateDetail, data: issueKey}
					}
				}
			}
		case "n":
			return m, func() tea.Msg {
				return pushStateMsg{state: stateForm, data: m.effectiveProjectCode()}
			}
		case "r":
			cmd = m.startContext(true)
			return m, cmd
		}

		// Let the table handle navigation keys
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m issuesModel) View() string {
	return boundIssueView(m.renderView(), m.width, m.height)
}

func (m issuesModel) renderView() string {
	if m.filterMode {
		return m.filterPanelView()
	}
	if m.err != nil {
		return m.filterSummary() + "\n" + StyleErrorMessage.Render(fmt.Sprintf("Error: %v", m.err)) + "\n[f] Filters  [Ctrl+f] Clear filters  [r] Retry  [Esc] Back"
	}

	if m.loading {
		loadingMsg := " Loading issues..."
		if m.loadingText != "" {
			loadingMsg = " " + m.loadingText
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			lipgloss.JoinHorizontal(lipgloss.Center, m.spinner.View(), loadingMsg))
	}

	if m.sortMode && m.cfg != nil {
		// Build columns list
		var colsList strings.Builder
		colsList.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet)).Bold(true).Render(" SORT COLUMN") + "\n\n")
		for i, f := range m.fields {
			selected := "( )"
			if strings.EqualFold(f.title, m.tempSortCol) {
				selected = "(*)"
			}
			item := fmt.Sprintf(" %s %s ", selected, f.title)
			if m.sortActiveSection == 0 && i == m.sortColCursor {
				colsList.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBg)).Background(lipgloss.Color(ColorCyan)).Bold(true).Render(item) + "\n")
			} else {
				colsList.WriteString(item + "\n")
			}
		}

		// Build direction list
		var dirList strings.Builder
		dirList.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet)).Bold(true).Render(" SORT DIRECTION") + "\n\n")

		dirs := []string{"asc", "desc"}
		labels := []string{"Ascending", "Descending"}
		for i, d := range dirs {
			selected := "( )"
			if strings.EqualFold(d, m.tempSortDir) {
				selected = "(*)"
			}
			item := fmt.Sprintf(" %s %s ", selected, labels[i])
			if m.sortActiveSection == 1 && i == m.sortDirCursor {
				dirList.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBg)).Background(lipgloss.Color(ColorCyan)).Bold(true).Render(item) + "\n")
			} else {
				dirList.WriteString(item + "\n")
			}
		}

		// Calculate column width
		colWidth := (m.width - 8) / 2
		if colWidth < 20 {
			colWidth = 20
		}

		columns := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(colWidth).Render(colsList.String()),
			lipgloss.NewStyle().Width(colWidth).Render(dirList.String()),
		)

		boxTitle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorCyan)).Bold(true).Render(" Sort Tasks List ")
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorViolet)).
			Padding(1, 2).
			Width(m.width - 4).
			Render(lipgloss.JoinVertical(lipgloss.Left,
				boxTitle,
				"",
				columns,
			))

		titleText := " Issues Sort "
		title := StyleTitle.Render(titleText)

		help := StyleHelp.Render(" [↑↓←→] Navigate  [Space] Select  [Enter] Save  [Esc] Cancel  " + quitHelp() + " ")

		return lipgloss.JoinVertical(lipgloss.Left, title, box, "", help)
	}

	var statusSuffix string
	if len(m.issues) > 0 {
		visibleCount := len(m.table.Rows())
		if visibleCount != len(m.issues) {
			if !m.loadedAll {
				statusSuffix = fmt.Sprintf(" (Showing %d of %d loaded, loading more...)", visibleCount, len(m.issues))
			} else {
				statusSuffix = fmt.Sprintf(" (Showing %d of %d loaded)", visibleCount, len(m.issues))
			}
		} else {
			if !m.loadedAll {
				statusSuffix = fmt.Sprintf(" (Loaded %d, loading more...)", len(m.issues))
			} else {
				statusSuffix = fmt.Sprintf(" (Loaded %d)", len(m.issues))
			}
		}
	}

	if m.limitReached {
		statusSuffix += fmt.Sprintf(" · limit %d reached", m.maxIssues)
	}

	var titleText string
	if m.projectCode != "" {
		titleText = fmt.Sprintf(" Issues in Project: %s%s ", m.projectCode, statusSuffix)
	} else if m.sprintID != "" {
		titleText = fmt.Sprintf(" Issues on Board: %s (Sprint: %s)%s ", m.boardName, m.sprintName, statusSuffix)
	} else if m.query != "" {
		displayQuery := m.query
		if strings.HasPrefix(displayQuery, "Board ") || strings.HasPrefix(displayQuery, "Board:") {
			queryBody := displayQuery
			if strings.HasPrefix(queryBody, "Board ") {
				queryBody = strings.TrimPrefix(queryBody, "Board ")
			} else {
				queryBody = strings.TrimPrefix(queryBody, "Board:")
			}
			queryBody = strings.TrimSpace(queryBody)

			parts := strings.SplitN(queryBody, ":", 2)
			boardName := strings.TrimSpace(parts[0])
			if strings.HasPrefix(boardName, "{") && strings.HasSuffix(boardName, "}") {
				boardName = boardName[1 : len(boardName)-1]
			}

			if len(parts) > 1 {
				sprintName := strings.TrimSpace(parts[1])
				if strings.HasPrefix(sprintName, "{") && strings.HasSuffix(sprintName, "}") {
					sprintName = sprintName[1 : len(sprintName)-1]
				}
				titleText = fmt.Sprintf(" Issues on Board: %s (Sprint: %s)%s ", boardName, sprintName, statusSuffix)
			} else {
				titleText = fmt.Sprintf(" Issues on Board: %s%s ", boardName, statusSuffix)
			}
		} else if displayQuery == "reporter: me" {
			titleText = fmt.Sprintf(" Issues Created by Me%s ", statusSuffix)
		} else {
			titleText = fmt.Sprintf(" Issues matching: %s%s ", displayQuery, statusSuffix)
		}
	} else {
		titleText = fmt.Sprintf(" Issues%s ", statusSuffix)
	}
	titleTextStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorViolet)).Bold(true).Render(titleText)
	title := lipgloss.NewStyle().MarginBottom(1).Render(titleTextStyled)

	m.updateTableColumns()
	m.updateTableHeight()

	var searchBar string
	if m.searchMode {
		searchBar = "\n" + lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorCyan)).
			Width(m.width-4).
			Render(m.searchInput.View())
	}

	tableStr := m.table.View()
	help := m.issueHelp()

	view := lipgloss.JoinVertical(lipgloss.Left, title, m.filterSummary(), tableStr, searchBar, "", help)

	if m.actionMode {
		actions := m.actionList()
		var lines []string
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorViolet)).Render("Select Action (or press shortcut):"))
		lines = append(lines, "")
		for idx, act := range actions {
			displayStr := fmt.Sprintf("[%s] %s", act.Shortcut, act.Name)
			if idx == m.actionCursor {
				lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(ColorCyan)).Bold(true).Render("> "+displayStr))
			} else {
				lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(ColorText)).Render("  "+displayStr))
			}
		}
		popupContent := lipgloss.JoinVertical(lipgloss.Left, lines...)
		popup := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorViolet)).
			Background(lipgloss.Color(ColorSurface)).
			Padding(1, 2).
			Render(popupContent)

		popupWidth := lipgloss.Width(popup)
		x := (m.width - 4) - popupWidth
		if x < 0 {
			x = 0
		}
		// Overlay starting at row 2 (just under title), col x
		view = overlayLines(view, popup, x, 2)
	}

	return view
}
