# Mezha Herdr plugin

Manage the current Herdr workspace's Mezha sandbox through an interactive
terminal dashboard. The plugin requires `mezha` on the environment inherited by
Herdr.

## Development

Build Mezha and make it available on `PATH`, then link the plugin:

```sh
make build
export PATH="$PWD/bin:$PATH"
herdr plugin link ./herdr
```

Install the published plugin with:

```sh
herdr plugin install odzhu/mezha/herdr
```

## Dashboard

The `dev.mezha.dashboard` action opens a Herdr-managed overlay pane. Like
herdr-plus, the action is only a launcher; the interactive Bubble Tea interface
runs in the plugin pane declared by the manifest. The manifest also exposes
first-class Herdr actions for Mezha's public commands, including lifecycle,
synchronization, remote repair, logs, and sandbox or volume listing. Actions
that need a terminal use their corresponding plugin pane.

The dashboard provides:

- editing for the active Mezha configuration, including `MEZHA_HOME` project,
  worktree, and sandbox configuration layers
- an interactive sandbox shell in a new tab
- sandbox and persistent-volume listings
- pull the latest native devenv image
- sandbox create, start, and stop operations
- repository synchronization status
- upload, download, pull, and push operations
- sandbox destruction with confirmation
- a command entry for every Mezha CLI command and argument

Select **Command…** to enter the arguments that follow `mezha`, for example
`--sandbox shared-dev -- git status` or `sandbox logs --tail 200 --source sandbox`.
Quoted arguments and escaped spaces are supported.

The plugin does not install default keybindings. Invoke the action through Herdr
or bind `dev.mezha.dashboard` with the `plugin_action` keybinding type.

Operation output is available through the plugin command log:

```sh
herdr plugin log list --plugin dev.mezha
```
