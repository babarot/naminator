naminator
=========

[![Go](https://github.com/babarot/naminator/actions/workflows/build.yaml/badge.svg)](https://github.com/babarot/naminator/actions/workflows/build.yaml)

Renaming multiple photo files (JPG, PNG, TIFF, GIF, RAW, ...) based on their EXIF metadata, such as date/time or location.

This tool is heavily inspired by [Photo Naminator](https://apps.apple.com/us/app/photo-naminator/id1598189356).

![](./docs/demo-1.gif)

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
    tag: v0.1.8
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
  -d, --dest-dir=         The directory path where renamed photos will be moved
  -n, --dry-run           Simulate the command's actions without executing them
  -t, --group-by-date     Create a directory for each date and organize photos accordingly
  -e, --group-by-ext      Create a directory for each file extension and organize the photos accordingly
  -c, --clean             Remove empty directories after renaming

Meta Options:
      --debug=[full|live] View debug logs (default: "full")
  -v, --version           Show version

Help Options:
  -h, --help              Show this help message

```

Pass directories containing images as arguments, and rename the images based on their EXIF data. The renaming will occur within the same directories.

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
    │   └── raw
    │       ├── 2025-01-11_11-12-43.arw
    │       ├── 2025-01-11_11-15-50.arw
    │       ├── 2025-01-11_11-15-52.arw
    │       └── 2025-01-11_11-16-06.arw
    └── 2025-01-12
        ├── heif
        │   ├── 2025-01-12_13-04-00.heif
        │   └── 2025-01-12_13-04-06.heif
        └── raw
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
    │   └── raw
    │       ├── 2025-01-11_11-12-43.arw
    │       ├── 2025-01-11_11-15-50.arw
    │       ├── 2025-01-11_11-15-52.arw
    │       └── 2025-01-11_11-16-06.arw
    └── 2025-01-12
        ├── heif
        │   ├── 2025-01-12_13-04-00.heif
        │   └── 2025-01-12_13-04-06.heif
        └── raw
            ├── 2025-01-12_13-04-00.arw
            ├── 2025-01-12_13-04-06.arw
            └── 2025-01-12_13-13-56.arw
```

</td>
</tr>
</tbody>
</table>

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
