Feature: A merge leaves no branch behind and never reaches a doomed merge

  After a merge that goes well, prdash removes the PR branch wherever it knows
  the branch lives, and before arming it refuses a PR the forge cannot merge.

  Scenario: A successful merge deletes the branch in every clone
    Given a PR whose review was mounted and whose worktree is clean
    When the merge finishes well
    Then the review worktree is removed
    And the review branch prdash/pr-N and its tracking ref are deleted from the review clone
    And the PR source branch is deleted from the local clone, unless a worktree holds it
    And the remote branch is deleted by the forge

  Scenario: A dirty worktree keeps its branch
    Given a mounted review with uncommitted changes
    When the merge finishes well
    Then the worktree is kept
    And the review branch is not deleted
    And the notice says what was kept and why

  Scenario: A conflicting PR is refused before arming
    Given the forge reports that the branch conflicts with the target
    When the user presses merge
    Then the row shows a conflict marker in its own gutter
    And the detail card says no (conflicts with the target)
    And the merge is blocked with the target branch named

  Scenario: An unknown mergeability is never painted as mergeable
    Given the forge has not computed mergeability (UNKNOWN)
    When the user presses merge
    Then the row is unmarked and the detail card says unknown
    And the merge still arms but warns that the forge may refuse

  Scenario: Deleting a branch fails without failing the merge
    Given a branch that cannot be deleted (checked out elsewhere or a git refusal)
    When the cleanup runs
    Then the failure is a warning in the notice
    And the merge is still reported as ok
