# repo-conformity-enforcer

This tool enables teams to manage their organisational GitHub repos in an automated way. Once you surpass even only a handful of repos - it becomes very difficult to keep the settings aligned.

**This script is best run by someone/service account with administrator access to either the whole Organisation or who has admin on every repo.**

## Current Goals

The goal of this tool is to be idempotent, you can rerun it as many times as you like and still get the same result every time.

> :warning: It will run against every repo in the organisation you specify, automatically.

It currently has a number of goals:

* checkHooks
  * Adds a webhook to every repository
* checkLabels
  * This creates the mandatory labels on every repo (if they are missing)
* checkTeams
  * We want the permissions on every repository to be identical, this checks and updates if they are wrong (stuff like "Dev" gets write permissions, "ReadOnly" gets read permissions)
* checkRepoSettings
  * This sets up a bunch of standard settings on every repo: disable Issues, disable Wiki, only allow Squash merge.
* checkReleases
  * We use automation to bump our semver, but this requires an existing release to start from. This creates an initial release on every repo to get started (v0.0.1)
* checkBranchProtection
   * All `master` branches should be protected from merging unless the specified checks are passing, and PRs also require at least one approval.
* checkSigningProtection
  * We enforce GPG signed commits on every PR, so this makes sure the `master` branch also has that enabled. 

## Configure me

There's an initial configuration piece you need to do before starting. This is done in at the top of the code, with comments to explain each variable.

> :warning: It's recommended to understand what this tool will do before applying it to your entire organisation - as it _will_ unapologetically change all of your settings **irreversibly**. 

Pick which goals you would like to run, and add them in one by one. e.g. you could start with only `checkRepoSettings`.

## Run me

Ensure you've setup the GITHUB_TOKEN in your environment, then go for it:

```
export GITHUB_TOKEN=xxx
go build && ./repo-conformity-enforcer
```

## Troubleshooting

404 errors will appear if you don't have admin access to a specific repo. This is particularly relevant to the /hooks and /teams endpoints.

e.g.
```
GET https://api.github.com/repos/florx/repo-conformity-enforcer/hooks: 404 Not Found []
GET https://api.github.com/repos/florx/repo-conformity-enforcer/teams: 404 Not Found []
```

Success for a new repo (at time of writing) looks like this:

```
Processing florx/repo-conformity-enforcer ...
Creating webhook for repo-conformity
Didn't find label 'major' on repo-conformity-enforcer so creating it
Didn't find label 'minor' on repo-conformity-enforcer so creating it
Didn't find label 'patch' on repo-conformity-enforcer so creating it
Didn't find team 'Dev' or they had wrong permission on repo-conformity-enforcer so creating it
Didn't find team 'ReadOnly' or they had wrong permission on repo-conformity-enforcer so creating it
Didn't find any releases on repo-conformity-enforcer so creating a base one
Repo settings for 'repo-conformity' are incorrect
```
