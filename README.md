# goreplace
Replace cli utility written in golang

Inspired by https://github.com/piranha/goreplace

## Usage
```
Usage:
  goreplace

Application Options:
  -s, --search=      search term
  -r, --replace=     replacement term
  -i, --ignore-case  ignore pattern case
      --whole-line   replace whole line
      --regex        treat pattern as regex
  -v, --verbose      verbose mode
      --dry-run      dry run mode
  -V, --version      show version and exit
  -h, --help         show this help message
```

### Examples

| Command                                                               | Description                                                                                   |
|-----------------------------------------------------------------------|-----------------------------------------------------------------------------------------------|
| `goreplace -s foobar -r barfoo file1 file2`                           | Replaces `foobar` to `barfoo` in file1 and file2                                              |
| `goreplace --regex -s 'foo.*' -r barfoo file1 file2`                  | Replaces the regex `foo.*` to `barfoo` in file1 and file2                                     |
| `goreplace --regex --ignore-case -s 'foo.*' -r barfoo file1 file2`    | Replaces the regex `foo.*` (and ignore case) to `barfoo` in file1 and file2                   |
| `goreplace --whole-line -s 'foobar' -r barfoo file1 file2`            | Replaces all lines with content `foobar` to `barfoo` (whole line) in file1 and file2          |
