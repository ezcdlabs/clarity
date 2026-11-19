# Troubleshooting

## Troubleshooting: `report` fails with a rename or bad object error

A report step that fails with one of these lost a race with `git gc`:

```
error: build tree: rename .git/objects/pack/tmp_obj_1741553381
  .git/objects/9f/9edc86...: no such file or directory
```

```
push events ref: fatal: bad tree object 5a8315cd2bc0...
error: remote unpack failed: eof before pack header was fully read
 ! [remote rejected] refs/clarity/events -> refs/clarity/events (unpacker error)
```

Both are fixed — **upgrade the pinned action ref to v0.1.6 or later**. Nothing needs configuring; older versions are affected regardless of settings.

CI is where this shows up because a fresh `actions/checkout` has every object in a packfile and no `.git/objects/<xx>` directories at all, so nearly every event write has to create one. See [Surviving a concurrent gc](internals/implementation-notes.md#surviving-a-concurrent-gc) for the mechanism. Versions before v0.1.3 are the most exposed: the racing gc there is one clarity spawns itself, so the failure needs no other git activity in the workflow at all.
