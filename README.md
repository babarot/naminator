naminator
===

Renaming multiple photo files (JPG, PNG, TIFF, GIF, RAW, ...) based on their EXIF attributes, like date/time or location.

This tool is heavily inspired by [Photo Naminator](https://apps.apple.com/us/app/photo-naminator/id1598189356)

<!--
`naminator` is a CLI command that read EXIF data of photos (like JPEG, PNG, GIF or RAW format files) as command-line arguments and renames their filenames based on metadata associated with those photos, like the date and time, or location or with what type of lense a photo was taken.
-->


## Dependency

- [exiftool](https://exiftool.org/)

## Install

```console
go install github.com/babarot/naminator@latest
```

## Usage


```console
$ naminator --help
Usage:
  naminator

Application Options:
  -d, --dest-dir=      Directory path to move renamed photos
  -n, --dry-run        Displays the operations that would be performed using the specified command without actually running them
  -t, --group-by-date  Create a date directory and classify the photos for each date
  -e, --group-by-ext   Create an extension directory and classify the photos for each ext
  -c, --clean          Clean up directories after renaming
      --with-index     Include index in a file name
  -h, --help           Show help message
  -v, --version        Show version
```

Take directories having images as arguments, and rename images with EXIF data. Renaming will be processed in the same directories.

```console
$ naminator --dry-run ./11740919 /11840920
[INFO] Checking exif on photos... 100% [====================]
[INFO] (dryrun): Renaming "11740919/A7C08228.HIF" to "11740919/2024-09-19_09-17-20.heif"
[INFO] (dryrun): Renaming "11740919/A7C08228.ARW" to "11740919/2024-09-19_09-17-20.arw"
[INFO] (dryrun): Renaming "11740919/A7C08231.HIF" to "11740919/2024-09-19_09-17-38.heif"
[INFO] (dryrun): Renaming "11740919/A7C08231.ARW" to "11740919/2024-09-19_09-17-38.arw"
[INFO] (dryrun): Renaming "11740919/A7C08232.HIF" to "11740919/2024-09-19_09-17-45.heif"
[INFO] (dryrun): Renaming "11740919/A7C08232.ARW" to "11740919/2024-09-19_09-17-45.arw"
[INFO] (dryrun): Renaming "11740919/A7C08236.HIF" to "11740919/2024-09-19_18-11-56.heif"
[INFO] (dryrun): Renaming "11740919/A7C08236.ARW" to "11740919/2024-09-19_18-11-56.arw"
[INFO] (dryrun): Renaming "11840920/A7C08607.HIF" to "11840920/2024-09-20_13-24-36.heif"
[INFO] (dryrun): Renaming "11840920/A7C08607.ARW" to "11840920/2024-09-20_13-24-36.arw"
[INFO] (dryrun): Renaming "11840920/A7C08608.HIF" to "11840920/2024-09-20_13-24-48.heif"
[INFO] (dryrun): Renaming "11840920/A7C08608.ARW" to "11840920/2024-09-20_13-24-48.arw"
[INFO] (dryrun): Renaming "11840920/A7C08609.HIF" to "11840920/2024-09-20_13-24-50.heif"
[INFO] (dryrun): Renaming "11840920/A7C08609.ARW" to "11840920/2024-09-20_13-24-50.arw"
```

## Examples

Let's say your photos are located like below:

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

### 1. Make destination directory: `-d`, `--dest-dir`

<details><summary>Run command</summary>

```console
$ naminator --dest-dir=myPhotos ./10050111 ./10150112
[INFO] Checking exif on photos... 100% [====================]
[INFO] Renaming "10050111/A7C00138.HIF" to "myPhotos/2025-01-11_11-12-43.heif"
[INFO] Renaming "10050111/A7C00138.ARW" to "myPhotos/2025-01-11_11-12-43.arw"
[INFO] Renaming "10050111/A7C00139.HIF" to "myPhotos/2025-01-11_11-15-50.heif"
[INFO] Renaming "10050111/A7C00139.ARW" to "myPhotos/2025-01-11_11-15-50.arw"
[INFO] Renaming "10050111/A7C00140.ARW" to "myPhotos/2025-01-11_11-15-52.arw"
[INFO] Renaming "10050111/A7C00140.HIF" to "myPhotos/2025-01-11_11-15-52.heif"
[INFO] Renaming "10050111/A7C00141.ARW" to "myPhotos/2025-01-11_11-16-06.arw"
[INFO] Renaming "10050111/A7C00141.HIF" to "myPhotos/2025-01-11_11-16-06.heif"
[INFO] Renaming "10150112/A7C00221.HIF" to "myPhotos/2025-01-12_13-04-00.heif"
[INFO] Renaming "10150112/A7C00221.ARW" to "myPhotos/2025-01-12_13-04-00.arw"
[INFO] Renaming "10150112/A7C00222.ARW" to "myPhotos/2025-01-12_13-04-06.arw"
[INFO] Renaming "10150112/A7C00222.HIF" to "myPhotos/2025-01-12_13-04-06.heif"
[INFO] Renaming "10150112/A7C00223.ARW" to "myPhotos/2025-01-12_13-13-56.arw"
```

</details>

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

### 2. Make the datetime directory: `-t`, `--group-by-date`


<details><summary>Run command</summary>

```console
$ naminator --dest-dir=myPhotos --group-by-date ./10050111 ./10150112
[INFO] Checking exif on photos... 100% [====================]
[INFO] Renaming "10050111/A7C00138.ARW" to "myPhotos/2025-01-11/2025-01-11_11-12-43.arw"
[INFO] Renaming "10050111/A7C00138.HIF" to "myPhotos/2025-01-11/2025-01-11_11-12-43.heif"
[INFO] Renaming "10050111/A7C00139.HIF" to "myPhotos/2025-01-11/2025-01-11_11-15-50.heif"
[INFO] Renaming "10050111/A7C00139.ARW" to "myPhotos/2025-01-11/2025-01-11_11-15-50.arw"
[INFO] Renaming "10050111/A7C00140.ARW" to "myPhotos/2025-01-11/2025-01-11_11-15-52.arw"
[INFO] Renaming "10050111/A7C00140.HIF" to "myPhotos/2025-01-11/2025-01-11_11-15-52.heif"
[INFO] Renaming "10050111/A7C00141.ARW" to "myPhotos/2025-01-11/2025-01-11_11-16-06.arw"
[INFO] Renaming "10050111/A7C00141.HIF" to "myPhotos/2025-01-11/2025-01-11_11-16-06.heif"
[INFO] Renaming "10150112/A7C00221.ARW" to "myPhotos/2025-01-12/2025-01-12_13-04-00.arw"
[INFO] Renaming "10150112/A7C00221.HIF" to "myPhotos/2025-01-12/2025-01-12_13-04-00.heif"
[INFO] Renaming "10150112/A7C00222.HIF" to "myPhotos/2025-01-12/2025-01-12_13-04-06.heif"
[INFO] Renaming "10150112/A7C00222.ARW" to "myPhotos/2025-01-12/2025-01-12_13-04-06.arw"
[INFO] Renaming "10150112/A7C00223.ARW" to "myPhotos/2025-01-12/2025-01-12_13-13-56.arw"
```

</details>

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

### 3. Make the extension directory: `-e`, `--group-by-ext`

<details><summary>Run command</summary>

```console
$ naminator --dest-dir=myPhotos --group-by-date --group-by-ext ./10050111 ./10150112
[INFO] Checking exif on photos... 100% [====================]
[INFO] Renaming "10050111/A7C00138.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-12-43.heif"
[INFO] Renaming "10050111/A7C00138.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-12-43.arw"
[INFO] Renaming "10050111/A7C00139.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-15-50.arw"
[INFO] Renaming "10050111/A7C00139.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-15-50.heif"
[INFO] Renaming "10050111/A7C00140.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-15-52.heif"
[INFO] Renaming "10050111/A7C00140.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-15-52.arw"
[INFO] Renaming "10050111/A7C00141.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-16-06.arw"
[INFO] Renaming "10050111/A7C00141.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-16-06.heif"
[INFO] Renaming "10150112/A7C00221.HIF" to "myPhotos/2025-01-12/heif/2025-01-12_13-04-00.heif"
[INFO] Renaming "10150112/A7C00221.ARW" to "myPhotos/2025-01-12/raw/2025-01-12_13-04-00.arw"
[INFO] Renaming "10150112/A7C00222.HIF" to "myPhotos/2025-01-12/heif/2025-01-12_13-04-06.heif"
[INFO] Renaming "10150112/A7C00222.ARW" to "myPhotos/2025-01-12/raw/2025-01-12_13-04-06.arw"
[INFO] Renaming "10150112/A7C00223.ARW" to "myPhotos/2025-01-12/raw/2025-01-12_13-13-56.arw"
```

</details>

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

### 4. Clean up empty directories: `-c`, `--clean`

<details><summary>Run command</summary>

```console
$ naminator --dest-dir=myPhotos --group-by-date --group-by-ext --clean ./10050111 ./10150112
[INFO] Checking exif on photos... 100% [====================]
[INFO] Renaming "10050111/A7C00138.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-12-43.arw"
[INFO] Renaming "10050111/A7C00138.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-12-43.heif"
[INFO] Renaming "10050111/A7C00139.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-15-50.heif"
[INFO] Renaming "10050111/A7C00139.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-15-50.arw"
[INFO] Renaming "10050111/A7C00140.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-15-52.arw"
[INFO] Renaming "10050111/A7C00140.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-15-52.heif"
[INFO] Renaming "10050111/A7C00141.ARW" to "myPhotos/2025-01-11/raw/2025-01-11_11-16-06.arw"
[INFO] Renaming "10050111/A7C00141.HIF" to "myPhotos/2025-01-11/heif/2025-01-11_11-16-06.heif"
[INFO] Renaming "10150112/A7C00221.ARW" to "myPhotos/2025-01-12/raw/2025-01-12_13-04-00.arw"
[INFO] Renaming "10150112/A7C00221.HIF" to "myPhotos/2025-01-12/heif/2025-01-12_13-04-00.heif"
[INFO] Renaming "10150112/A7C00222.HIF" to "myPhotos/2025-01-12/heif/2025-01-12_13-04-06.heif"
[INFO] Renaming "10150112/A7C00222.ARW" to "myPhotos/2025-01-12/raw/2025-01-12_13-04-06.arw"
[INFO] Renaming "10150112/A7C00223.ARW" to "myPhotos/2025-01-12/raw/2025-01-12_13-13-56.arw"
[INFO] Removed "./10050111" because empty
[INFO] Removed "./10150112" because empty
```

</details>

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

## License

MIT
