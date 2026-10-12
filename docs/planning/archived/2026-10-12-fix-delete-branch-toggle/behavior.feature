# Expected behaviour — fix/delete-branch-toggle (task #16)
#
# Source of truth for WHAT the user must see after the fix. Not wired to a
# runner; swe-executor turns these scenarios into real Go tests.
#
# Bug: on a GitHub repo with "Automatically delete head branches"
# (deleteBranchOnMerge), the merge confirmation's `tab` toggle `delete branch:
# no` was not honoured — GitHub deletes the head branch on merge regardless. The
# fix is honest signalling, not enforcement: prdash cannot keep a "no" there, so
# it says so. User decision: keep the compact label `delete branch: yes|no (tab)`,
# caveat only.

Feature: The delete-branch toggle tells the truth when the forge will delete anyway

  Background:
    Given an open pull request or merge request with the merge confirmation armed
    And the confirmation shows `delete branch: yes (tab)` by default

  # ---- the bug being fixed -----------------------------------------------

  Scenario: GitHub repo auto-deletes and the request is no — the confirmation says so
    Given a GitHub item whose repository has "automatically delete head branches" enabled
    When the user presses `tab` to set the delete request to no
    Then the confirmation reads `delete branch: no (tab)` with the caveat `repo auto-deletes head branches`

  Scenario: GitHub repo auto-deletes but the request is yes — no caveat
    Given a GitHub item whose repository auto-deletes head branches
    When the merge confirmation is armed and the request is still yes
    Then the confirmation reads `delete branch: yes (tab)` with no caveat
    Because the request and the repository agree, there is nothing to warn about

  # ---- boundaries: the caveat must appear ONLY where it is true ----

  Scenario: GitHub repo policy explicitly off and the request is no — no caveat
    Given a GitHub item whose repository reports "automatically delete head branches" as off
    When the user sets the delete request to no
    Then the confirmation reads `delete branch: no (tab)` with no caveat
    Because the branch really will survive, so claiming otherwise would be a new lie

  Scenario: GitHub repo policy unknown and the request is no — no caveat
    Given a GitHub item whose repository did not report the auto-delete policy
    When the user sets the delete request to no
    Then the confirmation reads `delete branch: no (tab)` with no caveat
    Because not knowing the policy is not the same as the policy being off

  # ---- the other forge keeps working as before ----

  Scenario: GitLab is unchanged
    Given a GitLab merge request
    When the user sets the delete request to no
    Then the confirmation reads `delete branch: no (tab)` with no caveat
    Because GitLab's per-merge-request `remove source branch` flag is honoured there

  # ---- the command sent to the forge stays honest ----

  Scenario: The request still drives the client-side delete flag
    Given the merge confirmation with the delete request set to no
    When the user picks a merge strategy
    Then no delete flag is sent to the forge (`--delete-branch` on GitHub, `--remove-source-branch` on GitLab)
    And with the request on, the flag is sent as before

  # ---- the toggle semantics are untouched ----

  Scenario: The toggle still turns off without disarming or firing
    Given the merge confirmation armed
    When the user presses `tab`
    Then the request flips and nothing is executed, and the merge stays armed
    And the value persists for the rest of the session

  # ---- honest degradation on a narrow terminal ----

  Scenario: A narrow terminal degrades honestly
    Given a terminal narrow enough that the caveat does not fit
    When the confirmation is painted
    Then the box is either painted whole or not painted halfway
    And no text is corrupted or overwritten by the longer label

  # ---- the policy costs nothing extra ----

  Scenario: The policy travels with the item data
    Given a GitHub inbox refresh
    Then the auto-delete policy is read from the same query that already fetches the item
    And no extra network round-trip is spent
