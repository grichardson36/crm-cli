# Keys

Rebind any key of the terminal UI. Every action has an id, default keys and a place in the palette.

## Change a key

```sh crm-example
crm keys list
crm keys list --group Contact
crm keys set undo ctrl+z
crm keys set favorite f "*"
crm keys reset favorite
crm keys path
```

`crm keys set ACTION KEY...` replaces the default keys of that action. Keys: a letter (`a`, `T`), a symbol (`?`), `enter`, `esc`, `tab`, `shift+tab`, `space`, arrows, `pgup`, `pgdown`, `f1` to `f12`, `ctrl+<letter>`, `alt+<key>`. Use `--dry-run` to check a binding without saving it.

A key that another action already uses in the same context is refused, and the message names that action. Rebind that one first. In the TUI use the palette actions "Keys: customize" (pick an action, press the new key, confirm) and "Keys: reset".

## The file

Bindings are the `[keys]` table of `~/.crm/config.toml`, next to the theme. One key or a list per action id:

```toml
[keys]
undo = "ctrl+z"
favorite = ["f", "*"]
```

A bad entry (unknown action, bad key name, clash) is skipped and reported in the status line and by `crm keys list`. The rest still works. Defaults: undo `ctrl+z`, redo `ctrl+y`.

See also: tui-keys, tui, theme.
