naminator
=========

[![Go](https://github.com/babarot/naminator/actions/workflows/build.yaml/badge.svg)](https://github.com/babarot/naminator/actions/workflows/build.yaml)

naminator renames photos by when they were taken. It reads the shooting time from their EXIF data and moves each photo to the path you choose, such as `2026-10-04/2026-10-04_14-22-09.arw`. Before anything moves, it shows you every new path.

![](./docs/demo.gif)

- See every new path before renaming, as a list or as a tree of the directories they make
- Choose the path with placeholders (the date and time, the camera, the original name, ...), or pick one of the presets
- Never overwrite a file: photos taken in the same second get `_1`, `_2`, ...
- Keep sidecar files (`.xmp`, `.aae`, ...) with their photos
- Remove the directories left empty
- Run it in scripts without the screen

It renames any photo exiftool finds a shooting time in: JPEG, HEIF and raw files such as ARW, CR3 and NEF, among others.

The name is borrowed from [Photo Naminator](https://apps.apple.com/us/app/photo-naminator/id1598189356), a GUI app for renaming photos. naminator is a separate command-line tool.

## Installation

naminator needs [exiftool](https://exiftool.org/):

```console
$ brew install exiftool
```

Then install naminator with Homebrew:

```console
$ brew install babarot/tap/naminator
```

Or with Go:

```console
$ go install github.com/babarot/naminator@latest
```

## Quick start

Pass the directories (or the files) of your photos:

```console
$ naminator ~/Pictures/import
```

A screen opens and lists the path each photo would get, as their EXIF data is read. Press `tab` to try another preset, `v` to see the directories as a tree, and `enter` then `y` to rename. `q` quits without renaming anything.

## The screen

| Key | |
|---|---|
| `tab` / `shift+tab` | Go through the presets |
| `i` / `a` | Edit the format (`esc` or `enter` to finish) |
| `j` / `k`, `g` / `G` | Move |
| `ctrl+d` / `ctrl+u` | Move a page down or up |
| `v` | Switch between the list and a tree of the directories as they will be |
| `h` / `l` | Fold or open a directory in the tree |
| `m` | Use the file's modification time for a photo without a shooting time |
| `ctrl+s` | Save the format as the default in the configuration |
| `enter` | Rename, after asking |


The mouse works too: the wheel moves, a click selects a line (a second click on a directory folds or opens it), and a click on the format, the preset or `view:` does what `i`, `tab` and `v` do. Renaming stays on `enter` and `y`.

### Formats

A format is the path photos are renamed to, with placeholders for their values. `/` makes directories, and the extension is added at the end. Press `i` to edit it: the paths follow each key, and a format that cannot be read says what is wrong. `ctrl+s` keeps it as the default.

![](./docs/demo-format.gif)

| Placeholder | Value |
|---|---|
| `{date}` `{time}` | `2026-10-04` `14-22-09` |
| `{year}` `{month}` `{day}` `{hour}` `{min}` `{sec}` | `2026` `10` `04` `14` `22` `09` |
| `{orig}` | The original name without the extension, as `DSC05501` |
| `{ext}` | The extension in lower case, as `arw` |
| `{camera}` | The camera model, as `ILCE-7CM2` (spaces become `_`) |
| `{seq}` | `001`, `002`, ... among the photos that would get the same path, in the order they were taken |
| `{dir}` | The directory of the photo, from where the path starts |

The path starts from the directory each argument is in. For `~/Pictures/import`, that is `~/Pictures`, so `{date}/{date}_{time}` puts the photos in `~/Pictures/2026-10-04/`, next to `import`. With `--dest-dir`, the path starts there instead.

The presets that `tab` goes through:

| Preset | Format |
|---|---|
| `date and time` | `{dir}/{date}_{time}` |
| `by date` | `{date}/{date}_{time}` |
| `by extension` | `{ext}/{date}_{time}` |
| `by date, then extension` | `{date}/{ext}/{date}_{time}` |
| `by extension, then date` | `{ext}/{date}/{date}_{time}` |
| `keep the original name` | `{date}/{time}_{orig}` |
| `numbered by camera` | `{camera}/{date}_{seq}` |

### Tree

`v` shows the directories as they will be after renaming. A long run of files is folded to its ends, and the files worth a look stay in sight: those numbered `_1`, and those without a shooting time. With `--clean`, the directories left empty are shown as removed.

![](./docs/demo-tree.gif)

## Without the screen

`--yes` renames without asking. So does running without a terminal for input and output: in a pipe, with input from a file, in a script or in CI. Choose the format with `--format` or `--preset`, and add `--dry-run` to see what would be renamed without renaming.

![](./docs/demo-yes.gif)

```console
$ naminator --yes --preset "by date" --clean ~/Pictures/import
$ naminator --yes --format '{ext}/{date}/{date}_{time}' 10551007 10651008
```

## Configuration

`~/.config/naminator/config.yaml` (under `$XDG_CONFIG_HOME` when it is set) sets the format used when none is given, whether to remove the directories left empty, and presets of your own, which `tab` goes through after the built-in ones:

```yaml
format: "{ext}/{date}/{date}_{time}"
clean: true
presets:
  - name: for the blog
    format: "blog/{date}/{date}_{orig}"
```

Without the file, or without `format`, photos are renamed in place, as `{dir}/{date}_{time}`.

## How photos are renamed

- Same second: photos taken in the same second get `_1`, `_2`, ... in the order they were taken. No file is ever overwritten.
- No shooting time: a photo without a shooting time in its EXIF, such as a screenshot, is skipped. On the screen, `m` uses the file's modification time for it instead.
- Sidecars: files named after a photo, as `DSC05501.xmp` or `DSC05501.ARW.xmp` (`.xmp`, `.aae`, `.dop`, `.pp3`), move with it. When a raw file and a JPEG or HEIF share one, it goes with the raw file; when no single raw file owns it, it stays where it is and is reported at the end. A photo whose sidecar would land on a path that is taken is not renamed.
- Other files stay where they are, and `--clean` keeps a directory that still has any.

## Options

```console
$ naminator --help
Usage:
  naminator [OPTIONS] [files... | dirs...]

Application Options:
  -f, --format=           The path to rename photos to, as
                          {ext}/{date}/{date}_{time}
  -p, --preset=           The format of a preset, by name, as "by extension,
                          then date"
  -d, --dest-dir=         The directory path where renamed photos will be moved
  -n, --dry-run           Simulate the command's actions without executing them
  -c, --clean             Remove empty directories after renaming
  -y, --yes               Rename without asking, also in a terminal
  -t, --group-by-date     Deprecated: use --preset "by date"
  -e, --group-by-ext      Deprecated: use --preset "by extension"
  -E, --group-ext-first   Deprecated: with -t and -e, use --preset "by
                          extension, then date"

Meta Options:
      --debug=[full|live] View debug logs (default: "full")
  -v, --version           Show version

Help Options:
  -h, --help              Show this help message
```

`-t`, `-e` and `-E` still work as they always have, and will be removed. They use the formats of these presets, from the directory above each photo's own rather than the one each argument is in, which is the same for a folder of photos:

| Flags | Preset |
|---|---|
| `-t` | `by date` |
| `-e` | `by extension` |
| `-t -e` | `by date, then extension` |
| `-t -e -E` | `by extension, then date` |

For example, `-tecE` is `--preset "by extension, then date" --clean`.

## Debugging

The `--debug` option controls the behavior of logging during execution:

- `--debug=full`: This option will read the entire log file from the beginning and output it to `stdout`, then continue to follow and display any new log entries in real-time (similar to `tail -f`).
- `--debug=live`: This option will skip the initial log file content and only display new log entries as they are written, in real-time.


```bash
naminator --debug
```

Use `--debug` to either get a full view of the logs or focus only on newly generated logs as the application runs.

## License

MIT
