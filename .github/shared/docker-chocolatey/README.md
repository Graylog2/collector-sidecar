# Chocolatey docker action

This runs provided commands in a slightly modified chocolatey container.

## Inputs

## `command`

** Required ** The command to execute within the container


## Example usage

uses ./.github/shared/docker-chocolatey
with:
  command: make package-chocolatey
