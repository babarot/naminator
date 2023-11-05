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
$ naminator --parent-dir Oct --dry-run 20231011 20231012
[INFO] Checking exif on photos... 100% [====================]
[INFO] (dryrun): Renaming "20231011/DSC00822.ARW" to "Oct/2023-10-10-001.arw"
[INFO] (dryrun): Renaming "20231011/DSC00823.ARW" to "Oct/2023-10-10-002.arw"
[INFO] (dryrun): Renaming "20231011/DSC00824.ARW" to "Oct/2023-10-10-003.arw"
[INFO] (dryrun): Renaming "20231011/DSC00825.ARW" to "Oct/2023-10-10-004.arw"
[INFO] (dryrun): Renaming "20231011/DSC00826.ARW" to "Oct/2023-10-10-005.arw"
[INFO] (dryrun): Renaming "20231012/DSC00827.ARW" to "Oct/2023-10-11-001.arw"
[INFO] (dryrun): Renaming "20231012/DSC00828.ARW" to "Oct/2023-10-11-002.arw"
[INFO] (dryrun): Renaming "20231012/DSC00829.ARW" to "Oct/2023-10-11-003.arw"
```

## License

MIT
