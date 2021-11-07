command faker

[![Go Report Card](https://goreportcard.com/badge/github.com/shu-go/f)](https://goreportcard.com/report/github.com/shu-go/f)
![MIT License](https://img.shields.io/badge/License-MIT-blue)

# Usage

## add (replace) a command

```
f --add gitinit git init
f --add goinit go mod init
```

## list commands

```
f
```

## run a command

```
f gitinit
```

## remove a command

```
f --remove gitinit
```

## make another faker

```
copy f another.exe
another --add gitinit echo hoge hoge
```

<!-- vim: set et ft=markdown sts=4 sw=4 ts=4 tw=0 : -->
