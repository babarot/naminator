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

Pass directories containing images as arguments, and rename the images based on their EXIF data. The renaming will occur within the same directories.

In a terminal, naminator first opens a screen that lists the path each photo would get, as the EXIF data is read. Nothing is renamed until you press `enter` and then `y`; `q` quits without renaming.

| Key | |
|---|---|
| `tab` / `shift+tab` | Go through the presets |
| `i` / `a` | Edit the format (`esc` or `enter` to finish) |
| `j` / `k`, `g` / `G` | Move |
| `t` | Switch between the list and a tree of the directories as they will be |
| `h` / `l` | Fold or open a directory in the tree |
| `m` | Use the file's modification time for a photo without a shooting time |
| `ctrl+s` | Save the format as the default in the configuration |
| `enter` | Rename, after asking |

The mouse works too: the wheel moves, a click selects a line (a second click on a directory folds or opens it), and a click on the format, the preset or `view:` does what `i`, `tab` and `t` do. Renaming stays on `enter` and `y`.

`--yes`, or running without a terminal for input and output (in a pipe, with input from a file, in a script or CI), renames without asking.

### Format

`--format` sets the path photos are renamed to, with placeholders for their values. `/` makes directories, and the extension is added at the end:

```console
$ naminator --format '{ext}/{date}/{date}_{time}' 10551007 10651008
```

| Placeholder | Value |
|---|---|
| `{date}` `{time}` | `2026-10-04` `14-22-09` |
| `{year}` `{month}` `{day}` `{hour}` `{min}` `{sec}` | `2026` `10` `04` `14` `22` `09` |
| `{orig}` | The original name without the extension, as `DSC05501` |
| `{ext}` | The extension in lower case, as `arw` |
| `{camera}` | The camera model, as `ILCE-7CM2` (spaces become `_`) |
| `{seq}` | `001`, `002`, ... among the photos that would get the same path, in the order they were taken |
| `{dir}` | The directory of the photo, from where the path starts |

The path starts from the directory each argument is in: for `10551007` above, its parent, so photos of both folders go to `arw/2026-10-04/...` next to them. With `--dest-dir`, it starts there. Photos taken in the same second get `_1`, `_2`, ... in the order they were taken, and no file is overwritten.

`--preset` uses a format by name:

| Preset | Format |
|---|---|
| `date and time` | `{dir}/{date}_{time}` |
| `by date` | `{date}/{date}_{time}` |
| `by extension` | `{ext}/{date}_{time}` |
| `by date, then extension` | `{date}/{ext}/{date}_{time}` |
| `by extension, then date` | `{ext}/{date}/{date}_{time}` |
| `keep the original name` | `{date}/{time}_{orig}` |
| `numbered by camera` | `{camera}/{date}_{seq}` |

`-t`, `-e` and `-E` still work as before, and will be removed: `-tecE` is `--preset "by extension, then date" -c`.

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

```console
$ naminator -tec ./DCIM/10550129

  Renaming done. Time: 1.37s (22 OK, 0 failed, 22 total)

  A7C00563.HIF: OK Renamed to DCIM/2025-01-29/heif/2025-01-29_19-32-50.heif
  A7C00570.ARW: OK Got exif data 1.31s
  A7C00570.ARW: OK Renamed to DCIM/2025-01-29/arw/2025-01-29_21-07-43.arw
  A7C00569.ARW: OK Got exif data 1.33s
  A7C00569.ARW: OK Renamed to DCIM/2025-01-29/arw/2025-01-29_21-07-41.arw
  A7C00571.HIF: OK Got exif data 1.34s
  A7C00571.HIF: OK Renamed to DCIM/2025-01-29/heif/2025-01-29_21-07-46.heif
  A7C00570.HIF: OK Got exif data 1.34s
  A7C00564.ARW: OK Got exif data 1.34s
  A7C00570.HIF: OK Renamed to DCIM/2025-01-29/heif/2025-01-29_21-07-43.heif
  A7C00564.ARW: OK Renamed to DCIM/2025-01-29/arw/2025-01-29_19-32-55.arw
  A7C00567.ARW: OK Got exif data 1.34s
  A7C00567.ARW: OK Renamed to DCIM/2025-01-29/arw/2025-01-29_21-02-35.arw
  A7C00562.ARW: OK Got exif data 1.36s
  A7C00562.ARW: OK Renamed to DCIM/2025-01-29/arw/2025-01-29_19-32-49.arw
  A7C00569.HIF: OK Got exif data 1.36s
  A7C00569.HIF: OK Renamed to DCIM/2025-01-29/heif/2025-01-29_21-07-41.heif
  A7C00566.ARW: OK Got exif data 1.36s
  A7C00565.ARW: OK Got exif data 1.36s
  A7C00565.ARW: OK Renamed to DCIM/2025-01-29/arw/2025-01-29_19-32-59.arw
  A7C00566.ARW: OK Renamed to DCIM/2025-01-29/arw/2025-01-29_21-02-24.arw
  10550129: OK Removed because empty
```

## Examples

Suppose your photos are organized as follows:

```
├── 10050111
│   ├── A7C00138.ARW
│   ├── A7C00138.HIF
│   ├── A7C00139.ARW
│   ├── A7C00139.HIF
│   ├── A7C00140.ARW
│   ├── A7C00140.HIF
│   ├── A7C00141.ARW
│   └── A7C00141.HIF
└── 10150112
    ├── A7C00221.ARW
    ├── A7C00221.HIF
    ├── A7C00222.ARW
    ├── A7C00222.HIF
    └── A7C00223.ARW
```

### 1. Create a destination directory: `-d`, `--dest-dir`

```console
naminator --dest-dir=myPhotos ./10050111 ./10150112
```

<table>
<thead>
<tr>
<th>Before</th>
<th>After</th>
</tr>
</thead>
<tbody>
<tr>
<td>

```
├── 10050111
│   ├── A7C00138.ARW
│   ├── A7C00138.HIF
│   ├── A7C00139.ARW
│   ├── A7C00139.HIF
│   ├── A7C00140.ARW
│   ├── A7C00140.HIF
│   ├── A7C00141.ARW
│   └── A7C00141.HIF
└── 10150112
    ├── A7C00221.ARW
    ├── A7C00221.HIF
    ├── A7C00222.ARW
    ├── A7C00222.HIF
    └── A7C00223.ARW
```

</td>
<td>

```
├── 10050111
├── 10150112
└── myPhotos
    ├── 2025-01-11_11-12-43.arw
    ├── 2025-01-11_11-12-43.heif
    ├── 2025-01-11_11-15-50.arw
    ├── 2025-01-11_11-15-50.heif
    ├── 2025-01-11_11-15-52.arw
    ├── 2025-01-11_11-15-52.heif
    ├── 2025-01-11_11-16-06.arw
    ├── 2025-01-11_11-16-06.heif
    ├── 2025-01-12_13-04-00.arw
    ├── 2025-01-12_13-04-00.heif
    ├── 2025-01-12_13-04-06.arw
    ├── 2025-01-12_13-04-06.heif
    └── 2025-01-12_13-13-56.arw
```

</td>
</tr>
</tbody>
</table>

### 2. Organize by date: `-t`, `--group-by-date`

```console
naminator --dest-dir=myPhotos --group-by-date ./10050111 ./10150112
```

<table>
<thead>
<tr>
<th>Before</th>
<th>After</th>
</tr>
</thead>
<tbody>
<tr>
<td>

```
├── 10050111
│   ├── A7C00138.ARW
│   ├── A7C00138.HIF
│   ├── A7C00139.ARW
│   ├── A7C00139.HIF
│   ├── A7C00140.ARW
│   ├── A7C00140.HIF
│   ├── A7C00141.ARW
│   └── A7C00141.HIF
└── 10150112
    ├── A7C00221.ARW
    ├── A7C00221.HIF
    ├── A7C00222.ARW
    ├── A7C00222.HIF
    └── A7C00223.ARW
```

</td>
<td>

```
├── 10050111
├── 10150112
└── myPhotos
    ├── 2025-01-11
    │   ├── 2025-01-11_11-12-43.arw
    │   ├── 2025-01-11_11-12-43.heif
    │   ├── 2025-01-11_11-15-50.arw
    │   ├── 2025-01-11_11-15-50.heif
    │   ├── 2025-01-11_11-15-52.arw
    │   ├── 2025-01-11_11-15-52.heif
    │   ├── 2025-01-11_11-16-06.arw
    │   └── 2025-01-11_11-16-06.heif
    └── 2025-01-12
        ├── 2025-01-12_13-04-00.arw
        ├── 2025-01-12_13-04-00.heif
        ├── 2025-01-12_13-04-06.arw
        ├── 2025-01-12_13-04-06.heif
        └── 2025-01-12_13-13-56.arw
```

</td>
</tr>
</tbody>
</table>

### 3. Organize by file extension: `-e`, `--group-by-ext`

```console
naminator --dest-dir=myPhotos --group-by-date --group-by-ext ./10050111 ./10150112
```

<table>
<thead>
<tr>
<th>Before</th>
<th>After</th>
</tr>
</thead>
<tbody>
<tr>
<td>

```
├── 10050111
│   ├── A7C00138.ARW
│   ├── A7C00138.HIF
│   ├── A7C00139.ARW
│   ├── A7C00139.HIF
│   ├── A7C00140.ARW
│   ├── A7C00140.HIF
│   ├── A7C00141.ARW
│   └── A7C00141.HIF
└── 10150112
    ├── A7C00221.ARW
    ├── A7C00221.HIF
    ├── A7C00222.ARW
    ├── A7C00222.HIF
    └── A7C00223.ARW
```

</td>
<td>

```
├── 10050111
├── 10150112
└── myPhotos
    ├── 2025-01-11
    │   ├── heif
    │   │   ├── 2025-01-11_11-12-43.heif
    │   │   ├── 2025-01-11_11-15-50.heif
    │   │   ├── 2025-01-11_11-15-52.heif
    │   │   └── 2025-01-11_11-16-06.heif
    │   └── arw
    │       ├── 2025-01-11_11-12-43.arw
    │       ├── 2025-01-11_11-15-50.arw
    │       ├── 2025-01-11_11-15-52.arw
    │       └── 2025-01-11_11-16-06.arw
    └── 2025-01-12
        ├── heif
        │   ├── 2025-01-12_13-04-00.heif
        │   └── 2025-01-12_13-04-06.heif
        └── arw
            ├── 2025-01-12_13-04-00.arw
            ├── 2025-01-12_13-04-06.arw
            └── 2025-01-12_13-13-56.arw
```

</td>
</tr>
</tbody>
</table>

### 4. Clean up empty directories: `-c`, `--clean`

```console
naminator --dest-dir=myPhotos --group-by-date --group-by-ext --clean ./10050111 ./10150112
```


<table>
<thead>
<tr>
<th>Before</th>
<th>After</th>
</tr>
</thead>
<tbody>
<tr>
<td>

```
├── 10050111
│   ├── A7C00138.ARW
│   ├── A7C00138.HIF
│   ├── A7C00139.ARW
│   ├── A7C00139.HIF
│   ├── A7C00140.ARW
│   ├── A7C00140.HIF
│   ├── A7C00141.ARW
│   └── A7C00141.HIF
└── 10150112
    ├── A7C00221.ARW
    ├── A7C00221.HIF
    ├── A7C00222.ARW
    ├── A7C00222.HIF
    └── A7C00223.ARW
```

</td>
<td>

```
└── myPhotos
    ├── 2025-01-11
    │   ├── heif
    │   │   ├── 2025-01-11_11-12-43.heif
    │   │   ├── 2025-01-11_11-15-50.heif
    │   │   ├── 2025-01-11_11-15-52.heif
    │   │   └── 2025-01-11_11-16-06.heif
    │   └── arw
    │       ├── 2025-01-11_11-12-43.arw
    │       ├── 2025-01-11_11-15-50.arw
    │       ├── 2025-01-11_11-15-52.arw
    │       └── 2025-01-11_11-16-06.arw
    └── 2025-01-12
        ├── heif
        │   ├── 2025-01-12_13-04-00.heif
        │   └── 2025-01-12_13-04-06.heif
        └── arw
            ├── 2025-01-12_13-04-00.arw
            ├── 2025-01-12_13-04-06.arw
            └── 2025-01-12_13-13-56.arw
```

</td>
</tr>
</tbody>
</table>

## 5. Extension-First Grouping: `-E`, `--group-ext-first`

```console
naminator --dest-dir=myPhotos --group-by-date --group-by-ext --group-ext-first ./10050111 ./10150112
```

or with short options:

```console
naminator -tecE ./10050111 ./10150112
```

<table>
<thead>
<tr>
<th>Before</th>
<th>After</th>
</tr>
</thead>
<tbody>
<tr>
<td>

```
├── 10050111
│   ├── A7C00138.ARW
│   ├── A7C00138.HIF
│   ├── A7C00139.ARW
│   ├── A7C00139.HIF
│   ├── A7C00140.ARW
│   ├── A7C00140.HIF
│   ├── A7C00141.ARW
│   └── A7C00141.HIF
└── 10150112
    ├── A7C00221.ARW
    ├── A7C00221.HIF
    ├── A7C00222.ARW
    ├── A7C00222.HIF
    └── A7C00223.ARW
```

</td>
<td>

```
├── 10050111
├── 10150112
└── myPhotos
    ├── arw
    │   ├── 2025-01-11
    │   │   ├── 2025-01-11_11-12-43.arw
    │   │   ├── 2025-01-11_11-15-50.arw
    │   │   ├── 2025-01-11_11-15-52.arw
    │   │   └── 2025-01-11_11-16-06.arw
    │   └── 2025-01-12
    │       ├── 2025-01-12_13-04-00.arw
    │       ├── 2025-01-12_13-04-06.arw
    │       └── 2025-01-12_13-13-56.arw
    └── heif
        ├── 2025-01-11
        │   ├── 2025-01-11_11-12-43.heif
        │   ├── 2025-01-11_11-15-50.heif
        │   ├── 2025-01-11_11-15-52.heif
        │   └── 2025-01-11_11-16-06.heif
        └── 2025-01-12
            ├── 2025-01-12_13-04-00.heif
            └── 2025-01-12_13-04-06.heif
```

</td>
</tr>
</tbody>
</table>

When using both `--group-by-date` and `--group-by-ext` options, files are organized by date first, then by extension by default. The `--group-ext-first` option reverses this hierarchy, organizing files by extension first, then by date.

This option is especially useful when you prioritize file type organization over chronological organization. For example, when you work with different file formats separately (editing RAW files first, then reviewing JPEG/HEIF versions).

**Note**: The `--group-ext-first` option requires both `--group-by-date` and `--group-by-ext` to be specified.

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
