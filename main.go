package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v29/github"
	"golang.org/x/oauth2"
)

/** ----- Configuration section ----------------------------------------------- **/

// List of repos to skip applying the rules to
var skippedRepos = []string{"repo-conformity-enforcer"}

// Name of the organisation to search
var organisationName = "florx"

// Default status check every repo should have
var defaultStatusCheck = "pr-label-check"

// Additional status checks (specified below) will apply to any repos containing this name
var additionalStatusCheckContains = "service"

// Additional status checks to apply to the repos that contain the name above
var additionalStatusChecks = []string{
	"build",
	"test",
}

// Branch to protect
var branchToProtect = "master"

// Webhook add to every repo
var webhookURL string
var webhookSecret string

// You also need to update the `checkTeams` method with the teams and IDs you want

/** ------- End of configuration ----------------------------------------------- **/

var client *github.Client
var ctx context.Context

func main() {

	_, exists := os.LookupEnv("GITHUB_TOKEN")
	if !exists {
		panic("The ENV variable GITHUB_TOKEN is not set.")
	}

	ctx = context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	client = github.NewClient(tc)

	var allRepos []*github.Repository

	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	print("Getting all repos: ")

	for {
		print(".")
		repos, resp, err := client.Repositories.ListByOrg(ctx, organisationName, opt)
		if err != nil {
			println(err.Error())
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	println("\nGot all repos, processing one by one...")

	for _, repo := range allRepos {

		//skip archived and specified repos
		if !repo.GetArchived() && !contains(skippedRepos, repo.GetName()) {
			processRepo(repo)
		}
	}

}

func processRepo(repo *github.Repository) {
	fmt.Printf("Processing %s ...\n", repo.GetFullName())

	checkHooks(repo)
	checkLabels(repo)
	checkTeams(repo)
	checkRepoSettings(repo)
	checkReleases(repo)
	checkBranchProtection(repo)
	checkSigningProtection(repo)
}

// checkSigningProtection enforces GPG signed commits on the special branch
func checkSigningProtection(repo *github.Repository) {

	protection, _, err := client.Repositories.GetSignaturesProtectedBranch(ctx, repo.GetOwner().GetLogin(), repo.GetName(), branchToProtect)
	if err != nil {
		// we don't care about 404 responses, as this just tells us it's not protected
		println("Could not check signing branch protection", err.Error())
		return
	}

	if !protection.GetEnabled() {
		println("Signing protection is disbled, so enabling it")
		client.Repositories.RequireSignaturesOnProtectedBranch(ctx, repo.GetOwner().GetLogin(), repo.GetName(), branchToProtect)
	}

}

// checkBranchProtection enables branch protection on our special branch, requires at least one review, and that various status checks are passing
func checkBranchProtection(repo *github.Repository) {

	needsUpdate := false
	reviewContexts := []string{defaultStatusCheck}

	protection, response, err := client.Repositories.GetBranchProtection(ctx, repo.GetOwner().GetLogin(), repo.GetName(), branchToProtect)
	if err != nil && response.StatusCode != 404 {
		// we don't care about 404 responses, as this just tells us it's not protected
		println("Could not check branch protection", err.Error())
		return
	}

	if response.StatusCode == 404 {
		needsUpdate = true
	} else {
		reviews := protection.GetRequiredPullRequestReviews()
		if reviews == nil || reviews.RequiredApprovingReviewCount != 1 {
			needsUpdate = true
		}

		statusChecks := protection.GetRequiredStatusChecks()
		if statusChecks == nil || statusChecks.Strict != true {
			needsUpdate = true
		}

		if statusChecks == nil || !contains(statusChecks.Contexts, defaultStatusCheck) {
			needsUpdate = true
		}

		if strings.Contains(repo.GetName(), additionalStatusCheckContains) {

			if len(statusChecks.Contexts) != len(append(additionalStatusChecks, reviewContexts...)) {
				needsUpdate = true
				reviewContexts = append(additionalStatusChecks, reviewContexts...)
			} else {
				for _, reviewContext := range additionalStatusChecks {
					if !contains(statusChecks.Contexts, reviewContext) {
						needsUpdate = true
						reviewContexts = append(additionalStatusChecks, reviewContexts...)
						break
					}
				}
			}
		}
	}

	if needsUpdate {
		println("Branch protection isn't correct, so updating it")

		protectionRequest := &github.ProtectionRequest{
			RequiredStatusChecks: &github.RequiredStatusChecks{
				Strict:   true,
				Contexts: reviewContexts,
			},
			RequiredPullRequestReviews: &github.PullRequestReviewsEnforcementRequest{
				DismissStaleReviews:          false,
				RequireCodeOwnerReviews:      false,
				RequiredApprovingReviewCount: 1,
			},
			EnforceAdmins: false,
		}
		client.Repositories.UpdateBranchProtection(ctx, repo.GetOwner().GetLogin(), repo.GetName(), branchToProtect, protectionRequest)
	}
}

// checkReleases ensures that we have a base release for our automated semver release script to function properly
func checkReleases(repo *github.Repository) {

	releases, _, err := client.Repositories.ListReleases(ctx, repo.GetOwner().GetLogin(), repo.GetName(), nil)
	if err != nil {
		println(err.Error())
		return
	}

	if len(releases) == 0 {
		//create an initial release

		fmt.Printf("Didn't find any releases on %s so creating a base one\n", repo.GetName())

		release := github.RepositoryRelease{
			TagName:         github.String("v0.0.1"),
			TargetCommitish: github.String("master"),
			Name:            github.String("v0.0.1 - Initial Release"),
			Body:            github.String("This is the initial semver release base number."),
			Draft:           github.Bool(false),
			Prerelease:      github.Bool(false),
		}
		client.Repositories.CreateRelease(ctx, repo.GetOwner().GetLogin(), repo.GetName(), &release)
	}
}

//checkHooks ensures that the pr-label-check webhook has been added to every repo.
func checkHooks(repo *github.Repository) {

	if len(webhookURL) == 0 {
		return
	}

	hooks, _, err := client.Repositories.ListHooks(ctx, repo.GetOwner().GetLogin(), repo.GetName(), nil)
	if err != nil {
		println(err.Error())
		return
	}

	foundHook := false
	for _, hook := range hooks {
		if hook.Config["url"] == webhookURL {
			foundHook = true
		}
	}

	if !foundHook {
		fmt.Printf("Creating webhook for %s\n", repo.GetName())
		hook := &github.Hook{
			Config: map[string]interface{}{
				"url":          github.String(webhookURL),
				"content_type": github.String("json"),
				"secret":       github.String(webhookSecret),
			},
			Events: []string{"pull_request"},
		}
		_, _, err := client.Repositories.CreateHook(ctx, repo.GetOwner().GetLogin(), repo.GetName(), hook)
		if err != nil {
			println(err.Error())
			return
		}
	}
}

//checkLabels ensures that the major/minor/patch labels for the pr-label-check webhook are created
func checkLabels(repo *github.Repository) {
	labels, _, err := client.Issues.ListLabels(ctx, repo.GetOwner().GetLogin(), repo.GetName(), nil)
	if err != nil {
		println(err.Error())
		return
	}

	checkForLabel(repo, labels, "major", "b60205")
	checkForLabel(repo, labels, "minor", "e8894a")
	checkForLabel(repo, labels, "patch", "b5d3ff")

}

func checkForLabel(repo *github.Repository, labels []*github.Label, labelName string, colour string) {
	for _, label := range labels {
		if label.GetName() == labelName {
			return
		}
	}

	fmt.Printf("Didn't find label '%s' on %s so creating it\n", labelName, repo.GetName())

	label := &github.Label{
		Name:        github.String(labelName),
		Color:       github.String(colour),
		Description: github.String(fmt.Sprintf("%s change", labelName)),
	}

	_, _, err := client.Issues.CreateLabel(ctx, repo.GetOwner().GetLogin(), repo.GetName(), label)
	if err != nil {
		println(err.Error())
		return
	}

}

//checkTeams ensures that the correct teams have been added to every repo, and they have the correct permissions
func checkTeams(repo *github.Repository) {

	teams, _, err := client.Repositories.ListTeams(ctx, repo.GetOwner().GetLogin(), repo.GetName(), nil)
	if err != nil {
		println(err.Error())
		return
	}

	// update the values here, 3rd is the teamName, 4th is the teamID, 5th is the permission
	// permission can be one of pull|push|admin
	// https://developer.github.com/v3/teams/#add-or-update-team-repository
	checkForTeam(repo, teams, "Dev", 12345, "push")
	checkForTeam(repo, teams, "ReadOnly", 54321, "pull")
	checkForTeam(repo, teams, "AllAdmins", 99999, "admin")

}

func checkForTeam(repo *github.Repository, teams []*github.Team, teamName string, teamID int64, permission string) {

	for _, team := range teams {
		if team.GetID() == teamID &&
			team.GetPermission() == permission {
			return
		}
	}

	fmt.Printf("Didn't find team '%s' or they had wrong permission on %s so creating it\n", teamName, repo.GetName())

	opt := &github.TeamAddTeamRepoOptions{
		Permission: permission,
	}

	_, err := client.Teams.AddTeamRepo(ctx, teamID, repo.GetOwner().GetLogin(), repo.GetName(), opt)
	if err != nil {
		println(err.Error())
		return
	}

}

//checkRepoSettings ensures that the wiki/issues are disabled on every repo, and that only squash merging is permitted.
func checkRepoSettings(repo *github.Repository) {

	if repo.GetHasWiki() || repo.GetHasIssues() || repo.GetAllowMergeCommit() || repo.GetAllowRebaseMerge() {
		fmt.Printf("Repo settings for '%s' are incorrect, so updating them\n", repo.GetName())

		repo.HasWiki = github.Bool(false)
		repo.HasIssues = github.Bool(false)
		repo.HasProjects = nil //you can set this to false if you didn't already disable at the org level.
		repo.AllowMergeCommit = github.Bool(false)
		repo.AllowRebaseMerge = github.Bool(false)
		repo.AllowSquashMerge = github.Bool(true)

		_, _, err := client.Repositories.Edit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), repo)
		if err != nil {
			println(err.Error())
			return
		}
	}

}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
