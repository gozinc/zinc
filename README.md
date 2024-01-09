<div align="center">
  <img width="200px" src="https://i.imgur.com/SOq9hKc.png" alt="Zinc Logo" />
</div>

# Zinc

Fullstack Go Tooling

# Installation

Using Go

```bash
go install https://github.com/gozinc/zinc@latest
```

Or install from [releases](https://github.com/gozinc/zinc/releases/latest)

# Usage

Create a new application

```bash
zinc create
```

Run the application

```bash
zinc run .
```

# Flags

1. `--no-git` - Don't initialize git repository

Example

```bash
zinc create --no-git
```

2. `--css` - Tailwind CSS file

```bash
zinc run --css static/css/tailwind.css
```
