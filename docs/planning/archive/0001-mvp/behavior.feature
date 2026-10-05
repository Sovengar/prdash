# prdash — expected behavior of the MVP (F1 + F2) and F3 as a milestone.
#
# The UNIQUE source of the expected behavior. It serves for the user to confirm
# that we captured what they want; it is NOT Cucumber (no step definitions nor
# runner). The executor derives real tests from here (unit/integration) BDD/ATDD.
#
# Gherkin in English; the descriptions are in English too.

Feature: Multi-forge PRs/MR inbox and review orchestrator over Herdr

  Background:
    Given prdash's XDG config defines "roots", the forges to query and the refresh cadence
    And "gh" is authenticated on github.com and "glab" on the self-managed GitLab
    And I run "prdash" inside a TUI

  # ─────────────────────────── F1 — Inbox cross-forge ───────────────────────────

  @F1 @inbox
  Scenario: The inbox shows the three sections with data from both forges
    Given there are PRs/MRs created by me, with review requested or assigned, and mentions
    When the TUI finishes the first refresh
    Then I see the sections "Created by me", "Review / assigned" and "Mentions"
    And each section shows items from GitHub and from the self-managed GitLab
    And each item indicates its forge and its host

  @F1 @inbox
  Scenario: "Created by me" only includes what I opened
    Given there are PRs opened by other people
    When I open the section "Created by me"
    Then only items whose author is me appear
    And other people's PRs do not appear in that section

  @F1 @inbox
  Scenario: "Review / assigned" includes review requested and assignments
    Given I was requested review of a PR on GitHub and I am assigned to an MR on the self-managed GitLab
    When I open the section "Review / assigned"
    Then both items appear in that section
    And each item indicates whether it is review requested or an assignment

  @F1 @inbox
  Scenario: "Mentions" uses the right source per forge
    Given I am mentioned in a GitHub PR
    And I am mentioned in an MR on the self-managed GitLab
    When I open the section "Mentions"
    Then the GitHub mention comes from the mention search via GraphQL
    And the GitLab mention comes from the GitLab Todos API
    And I see no mentions that do not include me

  @F1 @inbox
  Scenario: The item reflects the rich forge state without leaving the TUI
    Given a GitHub PR with "reviewDecision" and checks state
    And an MR on the self-managed GitLab with approval state
    When the inbox paints those items
    Then the GitHub item shows its review decision and the state of its checks
    And the GitLab item shows its approval state
    And I do not need to open the browser to see it

  @F1 @inbox
  Scenario: The same item is not duplicated across sections or queries
    Given a PR that meets more than one condition (e.g. created by me and where I am mentioned)
    When the inbox consolidates the results
    Then the PR appears only once, under the section with the highest relevance
    And the identity is resolved by forge, host, project and number

  @F1 @detail
  Scenario: Open the detail of an item
    Given an item selected in the inbox
    When I press the detail key
    Then I see the item's title, author, source/target branches, number and URL
    And I see its review state and checks/approval state
    And I can go back to the inbox without losing the selection

  @F1 @refresh
  Scenario: Manual refresh updates and stamps the time
    Given the inbox has already shown data
    When I press the refresh key
    Then prdash queries both forges again
    And the "last update" indicator reflects the moment of the refresh

  @F1 @refresh
  Scenario: Automatic refresh follows the configured cadence
    Given the refresh cadence is configured (60s by default)
    When the interval elapses without my interaction
    Then the inbox refreshes by itself
    And the "last update" indicator is updated

  @F1 @refresh
  Scenario: A refresh does not overwrite an action in progress
    Given I am running an approve/merge on an item
    When the automatic refresh fires
    Then the state of my action in progress is not reverted
    And the rest of the items do get updated

  @F1 @degradation
  Scenario: A forge that is down or without auth does not empty the inbox
    Given glab has no valid token for gitlab.com (responds 401)
    When the inbox queries the forges
    Then the gitlab.com section shows an explicit error state
    And the GitHub and self-managed GitLab items stay visible
    And the TUI does not abort

  @F1 @degradation
  Scenario: Distinguish "empty section" from "could not be queried"
    Given a section with no results on a forge that responded fine
    And another section whose forge returned an error
    When I paint the inbox
    Then the section with no results says it is empty
    And the failed section says it could not be queried
    And I never show "empty" when there was actually an error

  @F1 @degradation
  Scenario: Bitbucket is present but not operational
    Given the config enables the Bitbucket adapter
    When the inbox tries to query Bitbucket
    Then Bitbucket is reported as unsupported in this version
    And prdash makes no network call to Bitbucket

  @F1 @actions
  Scenario: Fast approve/merge from the inbox
    Given a reviewable item on GitHub or on the self-managed GitLab
    When I run approve (or merge when applicable) from the inbox
    Then the action runs via direct "gh"/"glab", or it is delegated to "tuicr" when appropriate
    And the item reflects the new state after refreshing

  @F1 @actions
  Scenario: The item changed on the forge between refresh and action
    Given an item that was closed or merged after the last refresh
    When I try an action on it
    Then prdash shows a clear conflict/not-found error
    And it refreshes the item instead of leaving it in an inconsistent state

  # ─────────────────────── F2 — Review orchestrator ───────────────────────

  @F2 @worktree
  Scenario: Worktree from an already local repo
    Given an item whose repo is already cloned and resolved in a configured root
    When I trigger "mount review" on the item
    Then prdash creates a worktree of the PR/MR branch
    And the worktree is recorded as the active review of that item

  @F2 @worktree
  Scenario: A repo that is not cloned gets cloned as bare and the worktree comes from the clone
    Given an item whose repo is not cloned in any root
    When I trigger "mount review"
    Then prdash creates a bare clone at "~/.local/share/prdash/repos/<forge>/<host>/<owner>/<repo>" (configurable)
    And it creates the worktree of the PR/MR branch from that bare clone
    And the clone and worktree destination is configurable

  @F2 @worktree
  Scenario: A fork PR whose branch does not exist on origin
    Given a fork item whose source branch is not a normal origin branch
    When I mount the review
    Then prdash fetches the PR ref ("refs/pull/N/head" on GitHub or "refs/merge-requests/N/head" on GitLab)
    And it creates a local working branch from that ref
    And it creates the worktree on that local branch

  @F2 @worktree
  Scenario: Reuse an item's existing worktree
    Given I already mounted the review of an item before
    When I mount it again
    Then prdash reuses the existing worktree
    And it does not create a duplicated worktree for the same item

  @F2 @worktree
  Scenario: Several PRs of the same repo do not clash
    Given I mount the review of two different items of the same repo
    Then each item has its own worktree, at a different path
    And both worktrees can coexist

  @F2 @worktree @degradation
  Scenario: Without clone or fetch permissions, the failure is clear and leaves no trash
    Given an item whose repo is private and I have no clone/fetch permissions
    When I try to mount the review
    Then prdash shows a clear error with the cause
    And it leaves neither a bare clone nor a half-done worktree

  @F2 @layout
  Scenario: The review layout mounts the three panes over the worktree
    Given an item with a created worktree
    And I am inside Herdr
    When the review layout opens
    Then a pane with TUICR over that PR/MR appears
    And a pane with Hunk showing the diff appears
    And a pane with an opencode agent appears
    And the three panes work on the item's worktree

  @F2 @layout @degradation
  Scenario: A missing tool does not take down the layout
    Given the Hunk binary is not installed
    When I open the review layout
    Then the Hunk pane is skipped with a warning
    And the other panes stay operational

  @F2 @layout
  Scenario: Link handler for PR URLs
    Given I am inside Herdr with a visible PR URL
    When I Ctrl+click the URL
    Then prdash resolves that URL to an item of the inbox
    And it mounts (or focuses) the review of that item

  @F2 @loop
  Scenario: The comment loop reaches the agent
    Given the review layout open with a pending comment
    When I write a comment in TUICR or in Hunk
    Then the opencode agent can read that comment
    And it applies the requested change in the worktree
    And I see the agent's result without switching tools

  @F2 @degradation
  Scenario: Outside Herdr, F1 keeps working and F2 reports itself
    Given I run prdash without "HERDR_ENV=1"
    When I open the TUI
    Then the inbox (F1) works normally
    And the mount-review action reports that it requires Herdr
    And prdash neither hangs nor leaves orphan processes

  # ─────────────── F3 — Auto-review with gate and allowlist (post-MVP) ───────────────

  @F3
  Scenario: A repo not on the allowlist does not self-approve
    Given auto-review mode enabled
    And the item's repo is not on the allowlist
    When the agent finishes the analysis with no critical findings
    Then prdash does NOT approve the PR/MR
    And it records the reason (allowlist)

  @F3
  Scenario: Self-approval with the gate satisfied
    Given auto-review mode enabled with the gate enabled
    And the item's repo is on the allowlist
    When the agent finishes the analysis with 0 critical findings
    Then prdash approves the PR/MR via the corresponding forge
    And it notifies via Herdr that it approved

  @F3
  Scenario: A failed or partial analysis never approves
    Given auto-review mode enabled
    When the agent's analysis fails or stays incomplete
    Then prdash does not approve the PR/MR
    And it reports that the analysis was not conclusive

  @F3 @degradation
  Scenario: Herdr notification without Herdr
    Given Herdr is not available
    When an event that had to be notified occurs
    Then prdash does not break and logs the event locally
