naminator
=========

[![Go](https://github.com/babarot/naminator/actions/workflows/build.yaml/badge.svg)](https://github.com/babarot/naminator/actions/workflows/build.yaml)

Renaming multiple photo files (JPG, PNG, TIFF, GIF, RAW, ...) based on their EXIF metadata, such as date/time or location.

This tool is heavily inspired by [Photo Naminator](https://apps.apple.com/us/app/photo-naminator/id1598189356).

![](./docs/demo.gif)

## Dependencies

[exiftool](https://exiftool.org/)

```bash
brew install exiftool
```

## Installation

Using `brew`

```console
brew install babarot/tap/naminator
```

Using `go`

```console
go install github.com/babarot/naminator@latest
```

Using [`afx`](https://github.com/babarot/afx)

```yaml
github:
- name: babarot/naminator
  description: Bulk-rename w/ EXIF metadata
  owner: babarot
  repo: naminator
  release:
    name: naminator
    tag: v0.2.7
  command:
    link:
    - from: naminator
```

```console
afx install
```

## Usage

Pass the directories (or the files) of your photos:

```console
$ naminator ~/Pictures/import
```

In a terminal, naminator opens a screen that shows the path each photo would get, as their EXIF data is read. Change the format or the preset and the paths follow; nothing is renamed until you press `enter` and then `y`, and `q` quits without renaming.

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

### Format

The format is the path photos are renamed to, with placeholders for their values. `/` makes directories, and the extension is added at the end. A format that cannot be read says what is wrong, and `ctrl+s` keeps it as the default.

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

The path starts from the directory each argument is in: for `~/Pictures/import`, it is `~/Pictures`, so `{date}/{date}_{time}` puts the photos in `~/Pictures/2026-10-04/` next to `import`. With `--dest-dir`, it starts there. Photos taken in the same second get `_1`, `_2`, ... in the order they were taken, and no file is overwritten.

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

`v` shows the directories as they will be after the renames. A long run of files is folded to its ends, photos numbered `_1` and those without a shooting time stay in sight, and with `--clean` the directories left empty are shown as removed.

![](./docs/demo-tree.gif)

A photo without a shooting time in its EXIF, such as a screenshot, is skipped; `m` on it uses the file's modification time instead.

### Without the screen

`--yes`, or running without a terminal for input and output (in a pipe, with input from a file, in a script or CI), renames without asking. `--format` and `--preset` choose the format, and `--dry-run` shows what would be renamed without renaming.

![](./docs/demo-yes.gif)

```console
$ naminator --yes --preset "by date" --clean ~/Pictures/import
$ naminator --yes --format '{ext}/{date}/{date}_{time}' 10551007 10651008
```

### Configuration

`~/.config/naminator/config.yaml` (under `$XDG_CONFIG_HOME` when it is set) sets the format used when none is given, whether to remove the directories left empty, and presets of your own:

```yaml
format: "{ext}/{date}/{date}_{time}"
clean: true
presets:
  - name: for the blog
    format: "blog/{date}/{date}_{orig}"
```

Without the file, or without `format`, photos are renamed in place as before.

Sidecar files (`.xmp`, `.aae`, `.dop`, `.pp3`) named after a photo, as `DSC05501.xmp` or `DSC05501.ARW.xmp`, are renamed with it, so they keep matching it. When a raw file and a JPEG or HEIF share a sidecar (`DSC05501.ARW` and `DSC05501.JPG` with `DSC05501.xmp`), it goes with the raw file; when no single raw file owns it, it is left where it is and reported at the end. A photo whose sidecar would go to a path that is taken is not renamed. Other files are left where they are, and a directory that still has them is not removed by `--clean`.

### Options

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
