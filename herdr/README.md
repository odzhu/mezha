# Mezha Herdr plugin

Manage the current Herdr workspace's Mezha sandbox through manifest actions.
The plugin requires `mezha` on the environment inherited by Herdr.

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

## Actions

- `dev.mezha.run` opens an interactive sandbox shell in a new tab.
- `dev.mezha.provision` provisions and registers the sandbox with Herdr.
- `dev.mezha.start` starts the sandbox.
- `dev.mezha.stop` stops the sandbox.
- `dev.mezha.status` displays synchronization status in a popup.
- `dev.mezha.upload` uploads dirty working-tree changes.
- `dev.mezha.download` downloads dirty working-tree changes.
- `dev.mezha.pull` pulls committed changes from the sandbox.
- `dev.mezha.push` pushes committed changes to the sandbox.
- `dev.mezha.destroy` opens a confirmation popup before destroying the sandbox.

Each action can be assigned to a user-selected key with Herdr's
`plugin_action` keybinding type. The plugin does not install default keybindings.

Background actions report completion or failure with Herdr notifications. Full
output is available through the plugin command log:

```sh
herdr plugin log list --plugin dev.mezha
```
