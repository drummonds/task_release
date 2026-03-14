# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

## [0.1.61] - 2026-03-14

 - fixing nothing to commit causing failures

## [0.1.60] - 2026-03-14

 - Adding linter

## [0.1.59] - 2026-03-14

 - adding RC stage

## [0.1.58] - 2026-03-14

 - fix bug

## [0.1.57] - 2026-03-13

 - Adding agent overview

## [0.1.56] - 2026-03-12

 - adding release to codeberg

## [0.1.55] - 2026-03-12

 - Adding pyproject.toml update on release

## [0.1.54] - 2026-03-12

 - Add colored banner to md2html and auto-prefix worktree names with WT

## [0.1.53] - 2026-03-12

 - Improving workree release

## [0.1.52] - 2026-03-12

 - removing mt2html --index and createing documentationg with updateble sections

## [0.1.51] - 2026-03-12

 - Adding combine

## [0.1.50] - 2026-03-11

 - Adding dual tagging

## [0.1.49] - 2026-03-11

 - combine rather than migrate

## [0.1.48] - 2026-03-10

 - Fixing release

## [0.1.47] - 2026-03-10

 - bug fix and merge readme feature

## [0.1.46] - 2026-03-09

 - Improving if no go.mod

## [0.1.45] - 2026-03-09

 - Update md2html

## [0.1.44] - 2026-03-08

 - Fix release bug

## [0.1.43] - 2026-03-08

 - Remote docs repo

## [0.1.42] - 2026-03-08

 - Checking for repos

## [0.1.41] - 2026-03-08

 - Improved docs and worktree

## [0.1.40] - 2026-03-08

 - Improving documentaiton

## [0.1.39] - 2026-03-07

 - Rereleasing to codeberge

## [0.1.38] - 2026-03-07

 - Docs to a second repo

## [0.1.37] - 2026-03-07

 - wt to use single VS code instance

## [0.1.36] - 2026-03-07

 - Adding check

## [0.1.35] - 2026-03-06

 - Adding forge support for codeberge

## [0.1.34] - 2026-03-06

 - Adding statichost.eu pages deployment

## [0.1.33] - 2026-03-06

 - Release static host version

## [0.1.32] - 2026-03-06

 - Add static host files

## [0.1.31] - 2026-03-06

 - Cleaning up worktree implementation

## [0.1.30] - 2026-03-05

 - Adding README to docs folder

## [0.1.29] - 2026-03-05

 - Refreshing documentation

## [0.1.28] - 2026-03-05

 - Adding worktree commands

## [0.1.27] - 2026-03-05

 - Adding release workflow docs and worktree commands

## [0.1.26] - 2026-03-04

 - Adding fork releases and adding documentation about releases

## [0.1.25] - 2026-03-04

 - Adding custom install process

## [0.1.24] - 2026-03-03

 - Adding remote check

## [0.1.23] - 2026-03-03

 - Check for new versio and add version editor

## [0.1.22] - 2026-03-03

 - Adding a post:release task stage

## [0.1.21] - 2026-03-03

 - Adding md2html and self update

- Added `md2html` subcommand (converts markdown to Bulma-styled HTML)
- Added `self update` subcommand (installs latest version via go install)

## [0.1.20] - 2026-03-03

 - Removing install wihtout a goreleaser option

## [0.1.19] - 2026-03-03

 - Allowing main.go at root level as an install pattern

## [0.1.18] - 2026-03-02

 - Adding multiple forges

## [0.1.17] - 2026-03-02

 - Adding retries for 500 errors

## [0.1.16] - 2026-03-02

 - Making release deletes cleaner

## [0.1.15] - 2026-03-02

 - Adding task precheck to make quicker

## [0.1.14] - 2026-03-01

 - Server incrmementally skips ports to find one open

## [0.1.13] - 2026-03-01

 - Improving retracted version detectinog

## [0.1.12] - 2026-03-01

 - Improving install

## [0.1.11] - 2026-03-01

 - Use debug.ReadBuildInfo for version when installed via go install

## [0.1.10] - 2026-03-01

 - Installing latest version locally

## [0.1.9] - 2026-03-01

 - Removing retracted releases and update installed version

## [0.1.8] - 2026-03-01

 - updating git config

## [0.1.7] - 2026-03-01

 - Trying after clean

## [0.1.6] - 2026-03-01

 - Cleaning goreleaser

## [0.1.5] - 2026-03-01

 - Working on goreleaser

## [0.1.4] - 2026-03-01

 - Changing goreleaser

## [0.1.3] - 2026-03-01

 - Switching to task-plus and adding server

### Changed

- Renamed from task-release to task-plus
- Added subcommand architecture: `task-plus release`, `task-plus pages`
- Config file renamed from `task-release.yml` to `task-plus.yml`
- Added `pages` subcommand to serve docs/ directory
- Added `-a` flag to list available commands
- Taskfile guard: `release` subcommand refuses if Taskfile.yml has a `release:` task

## [0.1.2] - 2026-03-01

 - Ignoring /dist

## [0.1.1] - 2026-03-01

 - Adding install stage and modifiying change format

## [0.1.0] - 2026-02-28

### Changed

- First

