# psync

[![build](https://github.com/plesk/psync/workflows/build/badge.svg)](https://github.com/plesk/psync/actions)
[![codecov](https://codecov.io/gh/plesk/psync/graph/badge.svg?token=uqWks9veLc)](https://codecov.io/gh/plesk/psync)

A CLI utility to automatic synchronization local source tree for Plesk or Plesk extension with remote machine.

# Installation

Installation using Homebrew:
```
brew install plesk/psync/psync
```

If you have Go toolchain installed, you can use the following command to install `psync`:
```
go install github.com/plesk/psync@latest
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
2026/07/12 22:12:27 watcher is ready...
```

The utility will watch the specified directory and will send the changed files to the specified remote host. Files deleted or renamed locally are removed from the remote host as well.

To upload the files that are currently changed according to `git status` (without starting the watcher):
```
REMOTE_HOST=10.66.1.1 psync diff
```

Deleted files are removed from the remote host as well. For renamed files, the new path is uploaded and the old one is removed.

# Limitations

* Utility runs only on macOS.
* Allowed remote destination platform is Linux only.
