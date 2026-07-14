# psync

[![build](https://github.com/plesk/psync/workflows/build/badge.svg)](https://github.com/plesk/psync/actions)
[![codecov](https://codecov.io/gh/plesk/psync/graph/badge.svg?token=uqWks9veLc)](https://codecov.io/gh/plesk/psync)

A CLI utility to automatic synchronization local source tree for Plesk or Plesk extension with remote machine.

# Installation

Go toolchain is required to build and install the binary. Clone the repository and run the following commands:

```
go build
go install
```

# Usage

Basic usage:

```
cd ~/projects/plesk/extensions/ext-log-browser
REMOTE_HOST=10.66.1.1 psync
```

The output can be like the following:

```
2026/07/12 22:12:27 Plesk detected
2026/07/12 22:12:27 Watcher is ready...
```

The utility will watch the specified directory and will send the changed files to the specified remote host.

# Limitations

* Utility runs only on macOS.
* Allowed remote destination platform is Linux only.
