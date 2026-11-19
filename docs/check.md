# Checking before you push: `git clarity check`

```sh
git clarity check && git push
```

`check` answers one question — *is CI red on the trunk?* — as an exit code, so it
composes with anything. Exit `0` means go ahead. Exit `1` means the trunk's newest
resolved CI status is `failed` and the push would land on top of a break. Nothing is
printed on green, so the conjunction costs nothing to adopt everywhere.

On red it writes the reason to stderr:

```
red trunk — ci failed on the tip

  a3f2c1d  fix(mobile): the registry survives two writers
  Steven Borrie, 4m 12s ago

whoever pushed that commit owns the fix. if that is not you, wait for green
rather than landing on top of it — your commits will bury the break and turn
a one-commit revert into a range.

  git clarity    watch for green
```

`--limit N` and `--cache-dir` behave as they do on `git clarity` itself. There is no
flag to pass the check: it is a speed bump at the moment of pushing, not enforcement,
and the design section explains why that is the whole point — see
[Why `check` is a convention, not a hook](design/check.md#why-check-is-a-convention-not-a-hook).
