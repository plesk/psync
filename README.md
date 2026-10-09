# psync

[![build](https://github.com/plesk/psync/workflows/build/badge.svg)](https://github.com/plesk/psync/actions)
[![codecov](https://codecov.io/gh/plesk/psync/graph/badge.svg?token=uqWks9veLc)](https://codecov.io/gh/plesk/psync)

A CLI utility that automatically synchronizes a local source tree of Plesk or a Plesk extension with a remote machine.

# Installation

Installation using Homebrew:
```
brew install plesk/psync/psync
```

If you have the Go toolchain installed, you can use the following command to install `psync`:
```
go install github.com/plesk/psync@latest
```

# Prerequisites

The utility uploads files over `ssh`/`scp` in non-interactive mode, so it cannot ask for a password. Access to the remote host must be configured with SSH keys:

```
ssh-keygen -t ed25519 # if you do not have a key yet
ssh-copy-id root@10.66.1.1
```

On startup, `psync` verifies that the remote host is reachable without a password prompt and exits with an error if SSH key-based authentication is not set up.

# Usage

Basic usage:
```
cd ~/projects/plesk/extensions/ext-broadcast-message
REMOTE_HOST=10.66.1.1 psync
```

The output looks like the following:
```
2026/08/12 11:02:26 Plesk extension broadcast-message detected
2026/08/12 11:02:26 watcher is ready...
...
2026/08/12 11:07:52 updated 10.66.95.33:/usr/local/psa/admin/plib/modules/broadcast-message/hooks/ContentInclude.php
...
```

The utility watches the specified directory and sends the changed files to the specified remote host. Files deleted or renamed locally are removed from the remote host as well.

Changes are applied one batch at a time over a single multiplexed SSH connection: files that changed together (for
example, after a `composer update`) are uploaded as a single `tar` stream and removed with a single `rm` call, instead
of one `scp`/`ssh` session per file.

By default, the current directory is monitored. Use the `-c` (`--chdir`) flag to monitor another directory without changing into it:
```
REMOTE_HOST=10.66.1.1 psync -c ~/projects/plesk/extensions/ext-broadcast-message
```

For a generic worktree without Plesk-specific directory mappings, use `-d` (`--destination`) to sync the entire monitored directory to a remote directory:
```
REMOTE_HOST=10.66.1.1 psync -d /var/www/html
```

To upload the files that are currently changed according to `git status` (without starting the watcher):
```
REMOTE_HOST=10.66.1.1 psync diff
```

Deleted files are removed from the remote host as well. For renamed files, the new path is uploaded and the old one is removed.

To upload the files changed by a particular commit, pass the ref as an argument:

```
REMOTE_HOST=10.66.1.1 psync diff <revision>
REMOTE_HOST=10.66.1.1 psync diff origin/main
psync diff HEAD~5
```

The `REMOTE_HOST` variable can also be stored in a `.env` file in the monitored directory. The value must not contain spaces; use a `user@host` form (for example, `REMOTE_HOST=root@10.66.1.1`) or a host alias from your SSH config.

# Limitations

* The utility runs only on macOS.
* The only supported remote platform is Linux.
