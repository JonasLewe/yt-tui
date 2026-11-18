# 🎛️ YouTrack Terminal User Interface (TUI)

[![CI](https://github.com/JonasLewe/yt-tui/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/JonasLewe/yt-tui/actions/workflows/test.yml)

This is [JonasLewe's fork](https://github.com/JonasLewe/yt-tui) of [nospor/yt-tui](https://github.com/nospor/yt-tui). Its `main` branch is the maintained default and includes project type/tag filters, configuration defaults with personal overrides, the assignee and external-editor fixes, Knowledge Base browsing/editing, numbered panels, and optional integration with [yt-tui.nvim](https://github.com/JonasLewe/yt-tui.nvim). The application also runs independently of Neovim.

A sleek, keyboard-driven terminal dashboard for JetBrains YouTrack, written in Go using the **Bubble Tea** framework. It communicates directly with the YouTrack REST API to provide high-performance, asynchronous, and interactive issue management without requiring any external Python CLI.

Styled out-of-the-box with a vibrant **Catppuccin Mocha** color palette, `yt-tui` keeps you in your terminal flow while tracking your project tasks.

<p float="left">
  <img src="assets/images/dashboard.png" width="300" />
  <img src="assets/images/taskslist.png" width="300" />
</p>
<p float="left">
  <img src="assets/images/taskdetails.png" width="300" />
  <img src="assets/images/tasknew.png" width="300" />
</p>
<p float="left">
  <img src="assets/images/listfiltering.png" width="300" />
  <img src="assets/images/listsorting.png" width="300" />
</p>

---

## ✨ Features

- **🎛️ Interactive Dashboard (Home)**: A split-pane interface showing **My Open Issues** (unresolved issues assigned to you) alongside your **Projects**. Seamlessly toggle focus with `Tab`.
- **📂 Global Project Browser**: View and browse all accessible YouTrack projects in a clean tabular view.
- **⚡ Asynchronous Paginated Loading**: Loads issues in the background without blocking the UI, giving you an instantly responsive view even on large codebases.
- **🔍 Filter & Search**: Filter project issues by their actual YouTrack types and tags, including multiple selections. Type and tag filters run on the server before pagination and the issue limit. Find tags by name inside the filter panel, or search loaded issues locally by readable ID or summary text.
- **📝 Comprehensive Issue Detail**: Renders descriptions (with a toggle `m` to switch between plain text and formatted markdown, saved in configuration), assignees, priorities, states, tags, and custom fields (including **Boards** and **Estimation**). Supports dedicated scrollable viewports for description, comments, linked issues, and attachments.
- **📚 Project Knowledge Base**: Browse a project's article hierarchy, filter article titles and IDs, and read articles in a scrollable Markdown or plain-text view. Create and edit articles using the same form controls and external editor as issues. Press `K` on a project or in its issues list.
- **🔄 Complete Issue Lifecycle**:
  - **Create & Clone**: Instantly spawn new issues or clone existing ones (pre-populating description, type, priority, and assignee details).
  - **State Transitions**: Transition states (e.g. `Open` ➔ `In Progress` ➔ `Fixed`) dynamically.
  - **Assigning**: Quickly assign tickets with a **live autocomplete dropdown** that filters project members as you type. Navigate suggestions with `↑`/`↓` and select with `Tab`/`Enter`. Still supports free-text entry: type `me` to self-assign, `unassigned`/`-` to unassign.
  - **Commenting**: Write and submit markdown comments directly from the detail view.
  - **Linked Issues**: Displays grouped relations (e.g., parent/subtask, depends on, etc.) with quick jump navigation on `Enter`.
  - **Task Attachments**: Lists issue files and attachments with their sizes; download and open them directly via `xdg-open` on `Enter`.
- **🛡️ Native REST API Integration**: Directly interacts with YouTrack REST API endpoints, completely avoiding Python keyring lockouts, credential corruption, or subprocess TTY issues.

---

## 🚀 Prerequisites

If you choose to build the project from source, you will need:
1. **Go Compiler**: Go 1.24.2 or newer, matching [go.mod](go.mod).

---

## 🛠️ Installation & Building

### 📦 Binary Availability

This fork has not published its own binary release yet. Build from `main` below to get the features described in this README. The original project's [upstream releases](https://github.com/nospor/yt-tui/releases) remain available, but do not include this fork's additions. See [Releases](#releases) for the fork's delivery workflow and current status.

### 🛠️ Build This Fork from Source

Build a binary whose `--version` output identifies the source commit:

```bash
git clone https://github.com/JonasLewe/yt-tui.git &&
cd yt-tui &&
git switch main &&
go build -trimpath \
  -ldflags "-s -w -X main.version=dev-$(git rev-parse --short=8 HEAD)" \
  -o yt-tui . &&
./yt-tui --version
```

Run `./yt-tui` to launch it. On Linux/macOS, you can install it into a user-writable directory on your `PATH`, for example:

```bash
mkdir -p "$HOME/.local/bin" &&
cp yt-tui "$HOME/.local/bin/yt-tui"
```

Ensure `$HOME/.local/bin` is on your `PATH` before launching `yt-tui` from another directory or from Neovim.

### Updating an Existing Installation

Pulling source changes does not replace an installed binary. Fully exit running TUI sessions with `Q` before updating, so an older process cannot save its previous configuration format. In your existing checkout, use `main` and build into the executable path used by your shell.

```bash
git switch main &&
git pull --ff-only &&
yt_tui_executable="$(command -v yt-tui)" &&
test -n "$yt_tui_executable" &&
go build -trimpath \
  -ldflags "-s -w -X main.version=dev-$(git rev-parse --short=8 HEAD)" \
  -o "$yt_tui_executable" . &&
"$yt_tui_executable" --version
```

The target must be writable by your user. In Neovim, check its actual executable path with `:lua print(vim.fn.exepath('yt-tui'))`; if this differs from your shell, use that path for `-o` instead. Then run `:YtTuiRestart` so the integration launches the new binary. `:YtTui`, toggling the window, or hiding it with `q` resumes the existing process.

---

## 🏃 Usage

You can launch `yt-tui` without any parameters to open the welcome screen or the dashboard:

```bash
yt-tui
```

### Direct Issue Link Loading
You can also launch `yt-tui` directly with a YouTrack issue URL as an argument:

```bash
yt-tui https://youtrack.example.com/issue/PROJ-123
# Or project-specific/plural issue URLs:
yt-tui https://youtrack.example.com/projects/PROJ/issues/PROJ-123
yt-tui https://youtrack.example.com/issues/PROJ-123
```

When you pass a URL:
1. `yt-tui` parses the URL to extract the base YouTrack server and the issue ID (e.g., `PROJ-123`).
2. It searches your configured servers in `config.json` (either the `url` field or the `servers` list).
3. If it finds a matching server, it automatically connects to that server and opens the issue detail view directly.
4. If no configured server matches the URL, it will display the welcome/login screen with the YouTrack Base URL pre-filled so you can enter your API token.

---

## ⚙️ Configuration

`yt-tui` loads personal settings and credentials from `~/.config/yt-tui/config.json`. Each startup begins with the defaults shipped by the installed version and applies the explicit values in this file. A first run creates a minimal versioned file rather than copying every default into it.

### Defaults and Personal Overrides

Keep only the settings you want to override. Missing settings inherit the current defaults, including new default columns such as **Type**. Explicit scalar and array values replace the corresponding default; project maps retain inherited entries unless you override those entries. The app saves the settings you change without turning the remaining defaults into personal overrides. Unknown top-level settings are preserved.

For example, this file uses the standard columns and only changes the sort order:

```json
{
  "config_version": 1,
  "url": "https://youtrack.example.com",
  "token": "$YOUTRACK_TOKEN",
  "sort_column": "Assignee",
  "sort_direction": "asc"
}
```

An explicit, nonempty `fields` list defines your complete layout. For example, adding `"fields": ["ID", "Summary", "Assignee"]` deliberately hides Type and the other columns. This list remains unchanged when a later release adds default columns. To use the latest standard layout again, remove `fields` from the file and restart the TUI. Removing another override similarly restores its current default. A known setting with `null` also inherits its default; meaningful explicit values such as `false`, `max_issues: 0`, and an empty project filter list are retained.

### Existing Configurations

Files without `config_version`, or with version `0`, migrate once to version `1`. Before any migration rewrite, the app creates an exact copy named `config.json.before-v1-*` beside the configuration. The original is retained if backing up or writing the migration fails. Both the backup and the new configuration are readable and writable only by your user.

The migration removes recognized historical generated defaults so they can follow future releases. This includes the original five-column layout (`ID`, `Summary`, `State`, `Priority`, `Assignee`) and the current six-column layout including `Type`, default page limits, built-in type/state/priority/work-type lists, unrestricted built-in State/Priority filters, and the original Markdown/theme/browser/activity defaults. Other values, credentials, unknown keys, and customized project entries remain intact.

Older files do not record whether a value equal to an old default was intentionally chosen. The migration treats those recognized values as inherited; the backup preserves the original file. In version `1`, an explicitly written value remains a personal override even when it equals the current default. To pin a recognized legacy value before migrating, add `"config_version": 1` yourself; the migration will not remove it.

### Environment Variables & `.env` Files (Optional)
You can resolve values in `config.json` dynamically from environment variables by prefixing the value with `$`. For example:

```json
{
  "config_version": 1,
  "url": "youtrack url",
  "token": "$SOMETOKEN"
}
```

When `yt-tui` encounters a value starting with `$`, it resolves it using:
1. **System Environment Variables**: (e.g., `os.Getenv("SOMETOKEN")`).
2. **Local/Config `.env` Files**: It checks for the variable definition in a local `.env` file in the current working directory, falling back to `~/.config/yt-tui/.env`.

If you do not specify a `url` or `token` in `config.json` at all, `yt-tui` will check standard fallback variables: `YOUTRACK_BASE_URL` and `YOUTRACK_TOKEN`.

If you have legacy credentials configured in `~/.config/youtrack-cli/.env`, they will be automatically migrated to your `config.json` on the first launch.

### Multi-Server Support (Optional)

If you manage issues across multiple YouTrack instances, you can configure an array of servers in your `config.json`. When the `servers` array is set, the application will display a list of these instances on startup for you to choose from.

```json
{
  "config_version": 1,
  "servers": [
    {
      "name": "Work YouTrack",
      "url": "https://work.youtrack.cloud",
      "token": "$WORK_TOKEN",
      "username_separator": "_"
    },
    {
      "name": "Personal YouTrack",
      "url": "https://personal.youtrack.cloud",
      "token": "perm:abc123yourtokenhere"
    }
  ]
}
```

If you select one of the configured servers, `yt-tui` will authenticate using those credentials. You can also select the `➕ Connect to another YouTrack...` option to manually input a new server's URL and token.

### Configuration Options

| Option                  | Type             | Default                                                                          | Description                                                                                                                                                                                                                                                                                                                                                                                              |
| ----------------------- | ---------------- | -------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `config_version` | Integer | `1` | Configuration format version. Managed by the app; legacy files migrate once with a backup. |
| `url`                   | String           | `""`                                                                             | The base URL of your YouTrack instance (e.g. `https://company.youtrack.cloud`).                                                                                                                                                                                                                                                                                                                          |
| `token`                 | String           | `""`                                                                             | Your permanent YouTrack API token.                                                                                                                                                                                                                                                                                                                                                                       |
| `servers`               | Array of Objects | `[]` (empty)                                                                     | List of YouTrack server configurations (each containing `name`, `url`, `token`, and optionally `vcs_base_url` or `username_separator`) to choose from on startup.                                                                                                                                                                                                                                        |
| `username_separator`    | String           | `""` (defaults to `.`)                                                           | The separator string used to merge first and last names when normalizing assignee inputs (e.g., `_` for `First_Last` or `.` for `first.last`). Can be set globally or per-server.                                                                                                                                                                                                                        |
| `page_size`             | Integer          | `20`                                                                             | A positive number of issues requested per query page. Larger page sizes fetch more records at once, while smaller sizes load background pages in quicker increments.                                                                                                                                                                                                                                            |
| `max_issues`            | Integer          | `500`                                                                            | The maximum number of issues to load for a project list. Set `0` for no limit; otherwise the app stops fetching pages when this cap is reached.                                                                                                                                                                                                              |
| `fields` | Array of Strings | `["ID", "Type", "Summary", "State", "Priority", "Assignee"]` | Columns in the issues list. Supports `ID`, `Type`, `Summary`, `State`, `Priority`, `Assignee`, `Updated`, `Updater`, `Created`, `Creator`/`Reporter`, and custom field names. Omit this setting to follow the current default layout. An explicit list is a complete personal layout; recognized legacy default lists migrate to inheritance. Missing type values appear as `—`. |
| `filtered_types` | Object | Unset (all types) | Saved type selections per project ShortName or ID, with an optional `default` fallback; for example `{"APP": ["Bug", "Story"], "ALL": []}`. A project entry containing `[]` means all types and overrides any default restriction. Values are actual YouTrack type names, selected through `F` in a project issues list. |
| `filtered_tags` | Object | Unset (no restriction) | Saved tag-name selections, first keyed by connection and account (`server URL#user ID`), then by project ShortName or ID. An optional `default` restriction applies only within that account. An explicit project `[]` clears the restriction. Managed through **Tags** in `F`; existing selections remain separate when switching accounts or servers. |
| `type_field_name` | Object | Unset (automatic) | Maps project ShortName or ID to its issue type custom field name, with an optional `default` fallback; for example `{"APP": "Ticket Type", "default": "Category"}`. Without an override, the app detects `Type` or `Issue Type`. |
| `custom_types`          | Object           | `{}` (empty)                                                                     | Custom list of issue type options configured per project (keyed by project ShortName or ID, e.g. `{"MTEL": ["Bug", "Task"], "default": ["Bug", "Feature"]}`) to populate the creation dropdown. Supports legacy array format as a fallback.                                                                                                                                                              |
| `custom_priorities`     | Object           | `{}` (empty)                                                                     | Custom list of issue priority options configured per project (keyed by project ShortName or ID, e.g. `{"MTEL": ["Minor", "Normal"], "default": ["Minor", "Major"]}`) to populate the creation dropdown. Supports legacy array format as a fallback.                                                                                                                                                      |
| `custom_states`         | Object           | `{}` (empty)                                                                     | Custom list of issue state transition options configured per project (keyed by project ShortName or ID, e.g. `{"MTEL": ["New", "In Dev"], "default": ["Open", "Closed"]}`) to populate the state selection modal. Supports legacy array format as a fallback.                                                                                                                                            |
| `filtered_states`       | Object           | `{}` (empty)                                                                     | Custom/filtered states to display on the tasks list (keyed by project ShortName or ID, e.g. `{"MTEL": ["Open", "In Progress"], "default": ["Open", "Closed"]}`). States not in this list will be filtered out. Supports legacy array format as a fallback.                                                                                                                                               |
| `filtered_priorities`   | Object           | `{}` (empty)                                                                     | Custom/filtered priorities to display on the tasks list (keyed by project ShortName or ID, e.g. `{"MTEL": ["Normal"], "default": ["Normal", "Major"]}`). Priorities not in this list will be filtered out. Supports legacy array format as a fallback.                                                                                                                                                   |
| `sort_column`           | String           | `""`                                                                             | The column by which the tasks list is sorted (e.g. `ID`, `Summary`, `State`, etc.).                                                                                                                                                                                                                                                                                                                      |
| `sort_direction`        | String           | `""`                                                                             | The sorting direction (`asc` or `desc`).                                                                                                                                                                                                                                                                                                                                                                 |
| `favorite_view` | String | `""` | Legacy saved-view bookmark. Retained for compatibility; the favorite UI is temporarily disabled. |
| `favorite_views` | Object | `{}` (empty) | Legacy saved-view bookmarks per server URL. Existing data is retained while the favorite UI is disabled. |
| `work_types`            | Array of Strings | `["Development", "Documentation", "Implementation", "Investigation", "Testing"]` | Custom list of work types for time tracking dropdown selection instead of the standard default list.                                                                                                                                                                                                                                                                                                     |
| `render_markdown`       | Boolean          | `true`                                                                           | Whether to format and render issue descriptions as markdown. Can be toggled inside the issue detail view by pressing `m`.                                                                                                                                                                                                                                                                                |
| `activity_sort_order`   | String           | `"asc"`                                                                          | Sort order for the issue activity stream in the Comments viewport (`asc` for oldest first, `desc` for newest first). Configured via the Activity filter panel (`F` in Comments viewport).                                                                                                                                                                                                                |
| `repo_options`          | Object           | `{}` (empty)                                                                     | Custom list of repository options per project (keyed by project ShortName or ID, e.g. `{"MTEL": ["repo1", "repo2"]}`) for updating the custom `Repo` field. Used as a fallback if options cannot be retrieved from YouTrack directly.                                                                                                                                                                    |
| `boards_options`        | Object           | `{}` (empty)                                                                     | Custom list of board/sprint options per project (keyed by project ShortName or ID) for updating the custom `Boards` field. Used as a fallback if options cannot be retrieved from YouTrack directly.                                                                                                                                                                                                     |
| `boards_field_name`     | Object           | `{}` (empty)                                                                     | Maps project ShortName or ID to the exact custom field name used for sprints/boards when your project uses a **custom field** (not agile sprint membership). For agile boards without a sync field (e.g. SRDS), leave this unset — sprint options and saves use the agile board API automatically. |
| `filepicker_sort_by`    | String           | `""`                                                                             | The criteria by which files in the file picker are sorted (`Name` or `Datetime`).                                                                                                                                                                                                                                                                                                                        |
| `filepicker_sort_order` | String           | `""`                                                                             | The sorting direction of the file picker (`asc` or `desc`).                                                                                                                                                                                                                                                                                                                                              |
| `filepicker_last_dir`   | String           | `""`                                                                             | The last directory visited by the file picker.                                                                                                                                                                                                                                                                                                                                                           |
| `actions`               | Object           | `{}` (empty)                                                                     | Custom templated action sequences triggered by `Space`, configured per project (keyed by project ShortName or ID, e.g. `{"MTEL": [...], "default": [...]}`). The `default` key is used when the current issue's project has no dedicated actions. Supports legacy array format as a fallback. See [Custom Quick Actions](#-custom-quick-actions) below.                                                  |
| `image_viewer`          | String           | `""` (empty)                                                                     | The command or executable to open image attachments (e.g. `sxiv` or `feh`). If empty or not an image file, it defaults to `xdg-open`. When set, pressing Enter on an image attachment downloads all image attachments in the current issue and loads them all into the viewer, automatically starting at the selected one (with support for `sxiv`/`nsxiv`/`imv`'s `-n` and `feh`'s `--start-at` flags). |
| `vcs_base_url`          | String           | `""` (empty)                                                                     | Base URL of your VCS (e.g. GitLab/GitHub instance) to resolve references like `group/project!mr` to links. Can also be set per-server in the `servers` list.                                                                                                                                                                                                                                             |
| `browser_command`       | String           | `"xdg-open"`                                                                     | The command or executable used to open web browser links (e.g. `google-chrome`).                                                                                                                                                                                                                                                                                                                         |
| `gitlab_command`        | String           | `""` (empty)                                                                     | The command or executable to open GitLab merge requests inside a popup TUI process (e.g. [gitlab-tui](https://github.com/nospor/gitlab-tui)). If empty, GitLab links are opened in the default browser.                                                                                                                                                                                                  |
| `theme`                 | String           | `"catppuccin"`                                                                   | The TUI color theme. Supported values: `catppuccin` (Catppuccin Mocha, default) and `teams` (adapted from `teams-tui-go` with green borders, cyan accents, and popups).                                                                                                                                                                                                                                  |



### ⚡ Custom Quick Actions

You can configure custom templates to quickly update issues with predefined command sequences. Pressing `Space` inside either the **Issues List** (on the selected issue) or **Issue Detail** view will open a popup listing these actions. You can navigate the list with arrow keys and hit `Enter` to apply, or directly hit the shortcut key (e.g. `1`-`9`) to apply the template instantly.

Each action template is configured under the `actions` key in `config.json`. Actions can be configured per project (keyed by project ShortName or ID) with a `default` entry used whenever the current issue's project has no dedicated actions. Below is an example structure:

```json
  "actions": {
    "default": [
      {
        "name": "In progress",
        "shortcut": "1",
        "commands": [
          {
            "type": "update_field",
            "field": "State",
            "value": "In Progress"
          }
        ]
      },
      {
        "name": "Assign to me & Add Comment",
        "shortcut": "2",
        "commands": [
          {
            "type": "assign",
            "value": "me"
          },
          {
            "type": "comment",
            "value": "Starting work on this task."
          }
        ]
      }
    ],
    "MTEL": [
      {
        "name": "Move to Closed",
        "shortcut": "3",
        "commands": [
          {
            "type": "update_field",
            "field": "State",
            "value": "Closed"
          }
        ]
      }
    ]
  }
```

The legacy array format (`"actions": [...]`) is still supported and is treated as the `default` project's actions.

Supported action command types:
- `update_field`: Updates any single-value custom field (e.g., `State`, `Repo`, `Priority`). Requires `field` and `value`.
- `comment`: Adds a comment to the issue. Requires `value` containing the comment text.
- `assign`: Assigns the issue to a user (use `"me"`, `"unassigned"`, or any valid username). Requires `value`.
- `update_board`: Replaces the issue's board/sprint assignments with the given value(s). Use `value` for a single sprint name or comma-separated names, or `values` for an array (e.g. `["Sprint 1", "Sprint 2"]`). An empty `value` clears all board assignments. Works with agile-board sprints and multi-value custom fields (auto-detected per project). For agile boards, only the board that contains the named sprint is updated; membership on other boards (for example a project overview board) is left unchanged.

Example board action:

```json
{
  "name": "Sprint 5",
  "shortcut": "5",
  "commands": [
    { "type": "update_board", "value": "Sprint 5" }
  ]
}
```

---

## ⌨️ Keyboard Controls & Navigation

### 🌐 Global Controls
* `q` / `Q`: Quit while browsing. In the Neovim integration described below, lowercase `q` hides the float and uppercase `Q` quits the process. Forms, search inputs, action shortcuts, and nested editors keep their normal text/key behavior.
* `Ctrl+C`: Force quit the application at any time.
* `?`: Toggle the help popup from any screen (except when welcome screen is shown or a text input/textarea is active), showing all keyboard shortcuts split by views.
* `S`: Open the global search popup from any screen (except when a text input or textarea is active).
  - Type or paste a phrase in the input field, then press `Enter` to search.
  - **Note**: YouTrack matches full words/phrases 
  - Once search completes, use `↑` / `↓` (or `k` / `j`, `Ctrl+P` / `Ctrl+N`) to navigate the results.
  - Press `Enter` to open the details view for the selected task.
  - Press `s` or `S` while navigating the list to focus back on the input field to search again.
  - Press `Esc` to close the popup.

### Neovim floating-window integration

A supporting Neovim launcher sets `YT_TUI_NVIM_HIDE=1` only for its terminal job.
While browsing, `q` asks that launcher to hide the window without stopping yt-tui;
`Q` fully quits. The current issue, pane, navigation history, and draft remain in
the running process when hidden. Close an input or popup normally before using
these browsing shortcuts. Inside an external editor, keys remain owned by that
editor. `Ctrl+C` remains the application's unconditional quit key.

This environment variable is a launcher integration marker, not a credential or
saved configuration setting. Do not set it globally in your shell: ordinary
terminals have no receiver for the hide request. Without it, both `q` and `Q`
quit as usual. The launcher listens for the fixed OSC sequence
`ESC ] 777 ; yt-tui ; hide BEL`; it must keep the process alive when handling it.
No issue data or executable code is sent in this request.

### 🚪 Welcome / Login View
* **Server Selection Mode** (when `servers` is configured):
  - `↑` / `↓` (or `k` / `j`): Navigate the server list.
  - `Enter`: Select and authenticate with the chosen YouTrack instance.
  - `q` / `Q` / `Ctrl+C`: Quit (lowercase `q` hides when launched by the Neovim integration).
* **Credentials Entry Mode**:
  - `Tab` / `Shift+Tab`: Switch focus between YouTrack Base URL and API Token fields.
  - `Enter`: Save credentials and authenticate.
  - `Esc`: Return to Server Selection Mode (if `servers` array is set).
  - `Ctrl+C`: Force quit the application.

### 🏠 Dashboard View (Home Screen)
* `0` / `1`: Jump directly to **[0] My Open Issues** or **[1] Projects**, matching the number embedded in each panel's top border.
* `Tab` / `Shift+Tab`: Switch active focus panel (**My Open Issues** vs **Projects**).
* `↑` / `↓` (or `k` / `j`): Scroll items inside the active focus panel.
* `Space` (when focusing My Open Issues): Open the custom quick Actions popup to quickly update the selected issue using a templated sequence (either select via list or hit shortcut number).
* `Enter`: Open the highlighted item:
  - Selecting an issue opens the **Issue Detail** view.
  - Selecting a project opens the **Issues List** filtered to that project (selecting **Issues created by me** displays all issues created/reported by you).
* `n`: Create a new issue (pre-selects the highlighted project if focused).
* `p`: View the full **Projects List** screen.
* `K` (when focusing a real project): Open that project's **Knowledge Base**.
* `b`: View the full **Agile Boards** screen.
* `r`: Reload and refresh all dashboard data.
* `q` / `Q`: Follow the global hide/quit behavior described above.

### 📂 Projects List View
* `↑` / `↓` (or `k` / `j`): Navigate projects table.
* `Enter`: View issues inside the selected project.
* `K`: Open the selected project's **Knowledge Base** (unavailable for **Issues created by me**).
* `n`: Create a new issue in the selected project.
* `r`: Refresh projects list.
* `Esc` / `Backspace`: Go back to the dashboard.
* **Note**: A special project called **Issues created by me** is pinned at the top of the list (separated from real projects by a blank line). Entering this project displays all issues created/reported by you (`reporter: me`).

### 🎛️ Agile Boards View
* `↑` / `↓` (or `k` / `j`): Navigate agile boards table.
* `Enter`: View issues belonging to the selected board.
* `r`: Refresh agile boards list.
* `Esc` / `Backspace`: Go back to the dashboard.

### 📚 Knowledge Base

Press uppercase `K` on a project in the dashboard or Projects List, or while viewing that project's issues. The article list loads all available pages and shows nested articles beneath their parents. Articles whose parents are not visible remain accessible.

* `↑` / `↓` or `k` / `j`: Move between articles.
* `PgUp` / `PgDn`, `Home` / `End`: Move through the list.
* `/`: Filter by article title or ID across all loaded articles. Matching nested articles are included independently of their parents. This filter does not search article body text.
* `Enter` or `Esc` while typing: Finish entering the filter.
* `Enter` on an article: Open the article reader.
* `n`: Create an article in the current project. The form's **Parent** dropdown offers **None** or the selected article, using the same `←` / `→` or `h` / `l` controls as other dropdowns. **None** creates a top-level article.
* `Esc` / `Backspace`: Clear an active filter; when no filter is active, return to the previous screen.
* `r`: Reload the project's articles.

In the **article reader**:

* `↑` / `↓` or `k` / `j`: Scroll the article.
* `PgUp` / `PgDn`: Scroll a page; `Ctrl+u` / `Ctrl+d`: Scroll half a page.
* `Home` / `End` or `g` / `G`: Jump to the top or bottom.
* `m`: Toggle Markdown rendering and plain text for the current reader. The initial mode follows `render_markdown`; this toggle does not change your saved settings.
* `e`: Edit the article's summary and content using the article form.
* `r`: Reload the article.
* `Esc` / `Backspace`: Return to the article list, preserving its filter and selection.
* `q` / `Q`: Follow the global hide/quit behavior (also available in the article list). Both letters remain text while typing a filter. `Ctrl+C` always quits the application.

The **article form** follows the issue form workflow:

* `Tab` / `Shift+Tab`: Move between fields. Arrow keys navigate fields or move within the content textarea, as in the issue form.
* `Ctrl+g` on **Content**: Edit the draft in the same `$EDITOR` used for issues, with `vim` as the fallback. Returning from the editor updates the draft; it does not save the article to YouTrack.
* `Ctrl+s`: Save and submit. On success, return to the previous screen with refreshed article data.
* `Esc`: Cancel and return. Save errors retain the draft for correction or retry.

Knowledge Base access uses the current connection and its article permissions. Empty lists and API errors are displayed in the TUI. Article comments and attachment management remain available in YouTrack's web interface. It also works in the Neovim floating terminal without plugin changes.

### 📋 Issues List View
* `↑` / `↓` (or `k` / `j`): Scroll issues table.
* `Enter`: View details of the selected issue.
* `K`: Open the current project's **Knowledge Base**. Available in a project-specific issues list.
* `/`: Activate search/filter mode. Type keywords to filter issues by summary/ID. Press `Esc` or `Enter` to close search mode.
* `Space`: Open the custom quick Actions popup to quickly update the selected issue using a templated sequence (either select via list or hit shortcut number).
* `F`: Open the filter panel for State and Priority, plus **Type** and **Tags** in a project-specific issues list. Use arrow keys/hjkl to navigate, Tab/Shift+Tab to switch sections, Space to toggle checkboxes, Enter to save, and Esc to cancel. Choose one or several project types, or **All types** to clear the type restriction. In **Tags**, press `/` to search names, then Enter to return to the choices. Escape closes the search first; press it again to discard the filter draft. Narrow terminals show one filter section at a time.
* `s`: Open the Column & Direction sort panel. Use arrow keys/hjkl to navigate, Space to select choices, Enter to save, and Esc to cancel.
* `n`: Create a new issue in the current project context.
* `r`: Clear cache and force reload issues list.
* `Esc` / `Backspace`: Go back to the dashboard/previous screen.

The Type filter obtains its choices from the project's YouTrack field configuration, including archived types that existing issues can still use; it does not use a fixed list of Bug, Story, Task, or Epic. Saving a type selection reloads issues using a server-side query across the whole project, before pagination and `max_issues` apply. The text search and State/Priority filters further narrow the loaded rows. The list shows visible and loaded counts and indicates when the configured issue cap is reached; these counts are not a total of all matching issues on the server.

If the project's type settings cannot be read, the panel shows only saved or observed values and reports that its options are unavailable. A restricted type filter is not applied until these settings can be read, and an existing restriction is never silently removed. Close the panel and press `r` to retry, or choose **All types** to browse without a type restriction.

The standard layout shows the **Type** column after ID. Column widths adapt to the terminal while preserving your configured column order. Recognized legacy standard layouts migrate automatically. An explicit personal `fields` list remains intact; add `"Type"` to that list, or remove the setting to inherit the full current standard layout. An issue with no type is shown as `—`. If your project uses another field name, configure `type_field_name` for that project. If `filtered_types` is absent, all types are included; an explicit empty project selection also includes all types even if a `default` restriction exists.

For `filtered_types` and `type_field_name`, a project ShortName entry takes precedence over its numeric project ID entry, followed by `default`. Saving the type filter uses the project's ShortName and removes its older ID entry so both ways of opening the project use the same selection.

The **Tags** section lists native YouTrack tags that the connected account can see, including personal and shared tags. The choices come from the server's complete paginated tag list, independent of the issue loading limit; tags can appear even when they have no issues in the current project. Search is case-insensitive and keeps selections made before the search. Multiple selected tags match **any** selected tag (OR), combined with the project and Type filter (AND). **No restriction** includes issues with and without tags. Saving reloads the matching issues on the server before pagination and `max_issues` apply, and the active selection appears above the table.

Tag selections are saved per account, server, and project, with ShortName taking precedence over ID and the account's `default` selection. YouTrack searches tags by name; visible tags with the same name are presented as one choice. If a selected tag disappears, is renamed, or cannot be read, the app reports the problem instead of silently removing the restriction. Refresh with `r` to retry, or remove the unavailable selection in `F`. Tag names containing braces, backslashes, wildcard characters (`*`, `?`), or control characters cannot be used as filters; attempting to save them reports an error. A custom field named “Labels” is separate from native YouTrack tags. Tag filtering is available in project issue lists; tags are shown in issue details without adding a table column or tag sorting.

Saved-view favorites are temporarily disabled: `f` no longer saves or opens a favorite, and the header star is removed. Existing `favorite_view` and `favorite_views` values are retained. Configured quick-action shortcuts still work. Native YouTrack tags such as `Star` can be viewed and filtered; this feature does not add/remove tags or change issue subscriptions.

### 🔍 Issue Detail View
Native tags appear as a wrapped list in the scrollable Description pane, so long tag lists remain readable without reducing the issue table's space.

* `0` / `1` / `2` / `3`: Jump directly to **[0] Description**, **[1] Comments**, **[2] Links**, or **[3] Attachments**. The numbers stay the same in full-view mode. These shortcuts apply while browsing; numbers retain their normal meaning in text inputs and action popups.
* `Tab` / `Shift+Tab`: Cycle focus forwards/backwards between the Description, Comments, Links (Parents/Children), and Attachments viewports.
* `↑` / `↓` (or `k` / `j`): Scroll/navigate within the active viewport. In the Comments, Links, and Attachments viewports, this moves the selection cursor.
* `Enter` (in Links viewport): Jump directly to the highlighted parent/child task.
* `Enter` (in Attachments viewport): Download the highlighted attachment and open it with `xdg-open`.
* `d` (in Links viewport): Delete the highlighted link (with confirmation).
* `d` (in Attachments viewport): Delete the highlighted attachment (with confirmation).
* `Ctrl+f`: Open the file browser popup to pick and attach files from your computer to the issue immediately. Press `z` in the file browser to jump to a directory from [zoxide](https://github.com/ajeetdsouza/zoxide) (`zoxide` must be on `PATH`). Type to filter, `↑`/`↓` to move, `Enter` to jump, `Esc` to return to the browser.
* `Ctrl+g`: View the description (when focusing the Description viewport) or the currently selected comment (when focusing the Comments viewport) in your preferred external editor without saving changes.
* `Space`: Open the custom quick Actions popup to quickly update the issue using a templated sequence (either select via list or hit shortcut number). Includes a built-in `[f]` Toggle Full View action to expand/collapse the active pane.
* `c`: Add a comment. Type your comment and press `Enter` to submit, `Alt+Enter` to insert a newline (multiline), or `Esc` to cancel. Type `@` to open a **mention autocomplete** dropdown (same project members/users as assign). Filter by typing after `@`; use `↑`/`↓` to navigate, `Tab` or `Enter` to insert `@login`, and `Esc` to dismiss the popup without cancelling the comment. You can also press `Ctrl+v` to paste and upload an image from the system clipboard, `Ctrl+f` to open the file browser popup to pick and attach files from your computer, or `Ctrl+g` to write/edit the comment in your preferred external editor.
* `s`: Transition issue state (opens state input prompt; e.g. type `In Progress` or `Fixed` and hit `Enter`).
* `R`: Select and update the custom `Repo` field options (opens selection menu; Left/Right to choose, Enter to save, Esc to cancel).
* `B`: Edit **Boards** / sprint assignment (opens a multi-select checklist when available). Uses agile board sprints when no custom Boards field exists on the project. Use `↑`/`↓` to navigate, `Space` to toggle sprints, `Enter` to save, `Esc` to cancel.
* `a`: Assign issue (opens an assignee input with a **live user suggestion dropdown**). As you type, matching project members and YouTrack users appear below the input. Use `↑`/`↓` to navigate the list and `Tab` or `Enter` to apply the highlighted name. You can also type a username, `me`, or `unassigned` directly and press `Enter`.
* `e`: Edit/update this issue's details (Summary, Description, Priority, Type, Assignee). When focusing the Comments viewport and a comment is selected, this edits the selected comment instead (supporting `@` mention autocomplete, `Alt+Enter` for newlines, `Ctrl+v` image paste, `Ctrl+f` computer file attachment, and `Ctrl+g` external editor inside the comment editor).
* `E`: Edit the issue's **Estimation** field (opens a small modal popup). Type a duration in `1w 1d 1h 1m` format and press `Enter` to save or `Esc` to cancel. Leave the input empty to clear the estimation.
* Press `n` to create a linked issue in the same project. Choose how the new issue relates to this one, then fill in the form.
* `C`: Clone this issue. Pre-populates the new issue form with this ticket's details, and automatically links the new issue back to the original under the "Is clone of" relationship.
* `y`: Start a yanking motion to copy issue details to the clipboard. Follow with:
  - `i`: Copy just the task ID to the clipboard.
  - `s`: Copy the task ID and summary (one line, separated by space) to the clipboard.
  - `d`: Copy the task description to the clipboard.
  - `u`: Copy issue URLs (extracts URLs from description, the YouTrack issue URL itself, linked issues, and attachment download URLs; copies if only 1 is found, or shows a selection popup if multiple exist).
  - `c`: Copy the currently selected comment to the clipboard (only visible/available when focusing the Comments viewport and a comment is selected).
  - Pressing any other key cancels the yanking motion.
* `o`: Show a list of unique URLs found in the description, comments, links, and attachments (URLs pointing to the ticket you're currently viewing are skipped). YouTrack issue links on this server are listed first and each entry is prefixed `[YouTrack]` — selecting one opens that task directly inside the app, just like jumping to a linked task. All other entries are prefixed `[Link]`; highlight and select one to open it. Regular links open via `browser_command` (defaults to `xdg-open`). If the URL is a GitLab merge request and `gitlab_command` is configured (e.g. to `gitlab-tui`), it opens the URL using that command in an interactive popup instead. You can find the GitLab TUI repository at [gitlab-tui](https://github.com/nospor/gitlab-tui).
* `m`: Toggle formatting the issue description between plain text and markdown (choice is remembered in configuration).
* `t`: Track time (opens a popup with an interactive calendar to select a date, duration input in `1w 1d 1h 1m` format, work type selection, and comment textarea).
* `F` (in Comments viewport): Open the Activity filter panel (Comments, Spent Time, VCS Changes, Change History, and sort by date). Use arrow keys/hjkl to navigate, Space to toggle filter checkboxes or select sort direction (`Ascending` / `Descending`), Enter to save, and Esc to cancel.
* `r`: Force refresh issue details, comments, links, and attachments.
* `Esc` / `Backspace`: Go back to the issues list (or the previous issue if navigated via links).

### 📝 Issue Form (Create / Clone / Edit)
* `Tab` / `Shift+Tab` / `↑` / `↓`: Move focus between form fields (Project, Summary, Description, Type, Priority, Assignee). When the **Assignee** field is focused and suggestions are visible, `↑`/`↓` navigates the suggestion list instead.
* On the **Assignee** field: a **live autocomplete dropdown** appears as you type, filtering project members and YouTrack users. Press `Tab` or `Enter` while a suggestion is highlighted to apply its actual YouTrack login. Editing and cloning also preserve the assignee's login, including capitalization and accented characters. You can still type any name freely if the list is empty or the person is not listed; manually entered multi-word names use the configured `username_separator`.
* `←` / `→` (or `h` / `l`): Cycle options in dropdown fields (Project, Type, Priority).
* `a`-`z` (on dropdown fields): Pressing the first letter of an option jumps directly to that choice.
* `Ctrl+g` (on Description field): Open preferred external editor (using the `$EDITOR` environment variable) to write/edit the description. Set `EDITOR=nvim` to use Neovim with your usual configuration and mappings. Save and exit the editor to return to the draft; `Ctrl+s` submits it to YouTrack. Navigation and shortcuts remain available after returning from the editor.
* `Ctrl+v` (on Description field): Paste an image directly from the system clipboard (Linux `xclip`/`wl-paste`, macOS, Windows). This injects standard Markdown image syntax and uploads the image to YouTrack on form submission.
* `Ctrl+f` (on Description field): Open the file browser popup to pick and attach files from your computer. Supports sorting results by name/datetime (pressing `s`) and order asc/desc (pressing `o`). Press `z` to open a [zoxide](https://github.com/ajeetdsouza/zoxide) directory jump overlay (`zoxide` must be on `PATH`): type to filter, `↑`/`↓` to move, `Enter` to jump, `Esc` to return to the browser. Last directory and sorting options are persisted in your config file.
* Press `Ctrl+s` to submit the form.
* `Esc`: Cancel and discard changes.

#### Create a linked issue

1. **Starting from an existing issue?** Open it and press `n`. The project and link target are prefilled. **Already filling out a new issue?** Tab to **Links** and press `Enter`.
2. Choose a relationship with `↑` or `↓` and press `Enter`. Read it from the new issue’s perspective. For example, to create a Story under an existing Epic, select **subtask of** if your YouTrack instance offers that relationship.
3. If the target is still empty, enter an issue ID such as `APP-42` or a YouTrack search query. Press `Enter` to search, select a result and press `Enter` to add it.
4. Fill in the summary and any other fields, then press `Ctrl+s`. To review links before submitting, open **Links** again. Choose **Add another link** to add one, or highlight an existing link and press `d` to remove it.

If YouTrack creates the issue but a link or attachment fails, the form displays the saved issue ID. Press `Enter` to retry the remaining work on that issue, or `Esc` to return.

## Development Workflow

`main` is this fork's stable development and installation branch. Start focused `feat/*`, `fix/*`, or `docs/*` branches from an up-to-date `main`, then open pull requests back to `main`. Before merging, run the checks below and a build; exercise the affected TUI flow when behavior changes. Delete working branches after their changes are fully merged into `main`.

### Continuous Integration

The active [Test workflow](.github/workflows/test.yml) runs on pull requests targeting `main` and pushes to `main`. [Workflow runs](https://github.com/JonasLewe/yt-tui/actions/workflows/test.yml) show the results. It uses GitHub's `ubuntu-latest` runner and the Go version declared in [go.mod](go.mod).

| Check | Purpose |
| --- | --- |
| `gofmt -l .` | Fails the job if any Go source files need formatting. |
| `go vet ./...` | Checks all packages for common programming mistakes. |
| `go test -v -race ./...` | Runs all Go tests and checks the executed test paths for data races. |

The suite covers configuration defaults and migration, saved filters, combined Type/Tag queries, pagination, asynchronous responses, keyboard handling, and file selection. Tests use isolated configuration and local HTTP servers instead of a real YouTrack account. CI currently tests on Linux; full interactive terminal smoke tests are performed separately. The test job compiles the packages for testing but does not publish a binary.

The Test and Release workflows use checkout and Go-setup actions with native Node.js 24 support. Node.js is used by GitHub Actions and is not a local installation requirement for yt-tui.

Run the checks and an additional build locally with:

```bash
test -z "$(gofmt -l .)" &&
go vet ./... &&
go test -race ./... &&
go build -o yt-tui .
```

### Upstream Updates

Track the original project separately as `upstream`. In a checkout without that remote, configure it once:

```bash
git remote add upstream https://github.com/nospor/yt-tui.git
```

For upstream updates, fetch the original repository, create a temporary `sync/*` branch from `main`, and merge the desired upstream changes there. Resolve conflicts and run the same checks before opening a pull request to `main`. Preserve the fork's features and existing commit history when integrating upstream changes.

### Releases

Merging to `main` runs CI; binary releases are a separate maintainer operation. The existing [Release workflow](.github/workflows/release.yml) starts when a `v*` tag is pushed. [GoReleaser](.goreleaser.yaml) builds Linux, macOS, and Windows binaries for AMD64 and ARM64, embeds the release version, and publishes archives plus `checksums.txt` as GitHub release assets.

The fork has no published binary releases yet. The current release workflow runs independently of the Test workflow and does not enforce a successful test result. Before the first fork release, it needs a test gate and updated release-note/changelog handling: the inherited [cliff.toml](cliff.toml) still links to upstream, and the workflow currently commits the generated changelog directly to `main`. Follow [this fork's releases](https://github.com/JonasLewe/yt-tui/releases) for future downloads; releases do not automatically update installed copies.

## License

YouTrack TUI is licensed under the MIT License. The original copyright and license are preserved in [LICENSE](LICENSE).

## Thanks For Visiting
Thanks to [nospor](https://github.com/nospor) for the original application. You can [support the upstream author](https://www.buymeacoffee.com/nospor).

