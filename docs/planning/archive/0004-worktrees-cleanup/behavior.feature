# prdash — expected behavior: review worktree cleanup.
#
# The UNIQUE source of the expected behavior of this feature. It is not Cucumber
# (no step definitions nor runner). The executor will derive real tests from
# here (unit/integration) in BDD/ATDD style.
#
# Gherkin in English; the descriptions are in English too.
#
# Design context: the review worktrees belong to the user and prdash keeps them
# on purpose — there is NO deletion on closing the app (hard restriction in
# cmd/prdash/worktrees.go). This feature opens two cleanup paths, and only two,
# both justified by an observable fact:
#
#   A) `prdash worktrees remove --orphans`: deletes in batch ONLY the worktrees
#      that `Audit` itself marks as orphaned (source repo unreachable). It is
#      exclusive with explicit paths. The source of truth of "orphan" is the
#      one `worktrees list` already uses: nothing is reimplemented here.
#
#   B) Self-deletion on merge FROM prdash: when a merge action launched by
#      prdash finishes well, the worktree of that item is already trash. But
#      the checkout can have uncommitted work, so it is deleted ONLY if it is
#      clean; if it is dirty, it is KEPT and a warning is issued with
#      "merged, but the worktree has uncommitted changes — kept". A merge that
#      did not go well (failure, conflict, permission, not mergeable), or any
#      other action (approve, retarget), triggers NOTHING.
#
# Invariant crossing A and B: every cleanup goes through the ownership guards
# (`worktree.Owned`: label or path name with the `prdash-` prefix) and through
# the requirement that the path lives under the managed root. A foreign
# worktree is untouchable on every path.

Feature: Review worktree cleanup in prdash

  Background:
    Given a managed worktree root where prdash provisions its own ones
    And there are prdash's own worktrees (name "prdash-…") and foreign worktrees coexisting under that root
    And a worktree is "orphaned" when the source repo its link points at is no longer accessible

  # ═══════════════════ A — batch cleanup of orphans ═══════════════════

  @cli @worktrees @orphans
  Scenario: --orphans deletes all the orphans and only the orphans
    Given the root has two orphaned own worktrees and one healthy own worktree
    When I run "prdash worktrees remove --orphans"
    Then the two orphans disappear from disk
    And the healthy own worktree stays on disk
    And the output names every worktree that was deleted
    And the command finishes with exit code 0

  @cli @worktrees @orphans
  Scenario: --orphans deletes exactly what "worktrees list" marks as orphaned
    Given "prdash worktrees list" reports N worktrees with state "orphaned"
    When I run "prdash worktrees remove --orphans"
    Then it deletes exactly those N
    And it deletes none of the ones reported with state "ok"

  @cli @worktrees @orphans @security
  Scenario: --orphans never touches a foreign worktree
    Given the root has a foreign worktree next to an own orphan
    When I run "prdash worktrees remove --orphans"
    Then the foreign worktree is still intact on disk
    And the own orphan disappears

  @cli @worktrees @orphans @security
  Scenario: a healthy own worktree is never deleted with --orphans
    Given an own worktree whose source repo is still accessible
    When I run "prdash worktrees remove --orphans"
    Then that worktree stays on disk
    And it keeps being listed with state "ok"

  @cli @worktrees @orphans
  Scenario: --orphans with zero orphans is the happy path, not an error
    Given the root has no orphaned worktree
    When I run "prdash worktrees remove --orphans"
    Then it deletes nothing
    And it reports that there are no orphans
    And the command finishes with exit code 0
    And it writes nothing to stderr

  @cli @worktrees @orphans @dry-run
  Scenario: --dry-run shows the exact batch it would delete and deletes nothing
    Given the root has two orphaned own worktrees, one healthy own worktree and one foreign worktree
    When I run "prdash worktrees remove --orphans --dry-run"
    Then it prints the paths of exactly the two orphans (the same batch it would delete without --dry-run)
    And it deletes no worktree
    And the healthy own worktree and the foreign one stay intact
    And the command finishes with exit code 0

  @cli @worktrees @orphans @dry-run
  Scenario: --dry-run with zero orphans is still the happy path
    Given the root has no orphaned worktree
    When I run "prdash worktrees remove --orphans --dry-run"
    Then it prints no orphan
    And it deletes nothing
    And the command finishes with exit code 0
    And it writes nothing to stderr

  @cli @worktrees @orphans @dry-run @usage
  Scenario: --dry-run without --orphans is a usage error
    Given an own orphan and a healthy own worktree
    When I run "prdash worktrees remove --dry-run"
    Then it explains on stderr that --dry-run requires --orphans
    And the command finishes with exit code 2
    And it deletes nothing, neither the orphan nor any worktree

  @cli @worktrees @orphans @usage
  Scenario: --orphans is exclusive with explicit paths
    Given an own orphan and a healthy own worktree
    When I run "prdash worktrees remove --orphans <path>"
    Then it deletes nothing, neither the orphan nor the named path
    And it explains on stderr that the two modes do not mix
    And the command finishes with exit code 2

  @cli @worktrees @orphans @usage
  Scenario: an unknown flag in remove is a usage error
    When I run "prdash worktrees remove --bogus"
    Then it deletes nothing
    And it explains on stderr that the flag is not recognized
    And the command finishes with exit code 2

  @cli @worktrees @usage
  Scenario: a dash-leading token is a flag, never a path
    Given a healthy own worktree
    When I run "prdash worktrees remove --orphan"
    Then it deletes nothing
    And it explains on stderr that it does not recognize the flag
    And the command finishes with exit code 2

  @cli @worktrees
  Scenario: remove with explicit paths keeps behaving as before
    Given a healthy own worktree and a foreign worktree
    When I run "prdash worktrees remove <own-path> <own-path-2>"
    Then it deletes the named own paths
    And the foreign worktree stays intact
    And a rejection (foreign or nonexistent path) touches nothing and exits with 1

  @cli @worktrees @cleanup
  Scenario: an orphan with an unresolvable .git link is deleted by explicit path
    Given an own worktree whose .git file declares no gitdir and whose source repo no longer exists
    When I run "prdash worktrees remove <its-path>"
    Then it deletes it anyway: there is no repo to resolve and only its checkout is left to delete
    And the command finishes with exit code 0

  @cli @worktrees @orphans @cleanup
  Scenario: an orphan with an unresolvable .git does not take down the --orphans batch
    Given the root has an orphan with the unresolvable .git and another orphan with a well-formed .git
    When I run "prdash worktrees remove --orphans"
    Then it deletes both orphans
    And the command finishes with exit code 0

  @cli @worktrees @usage
  Scenario: remove without paths nor --orphans is still a usage error
    When I run "prdash worktrees remove"
    Then it explains on stderr that at least one path is missing
    And the command finishes with exit code 2
    And it deletes nothing

  # ═══════════════ B — self-deletion of the worktree on merge from prdash ═══════════════

  @tui @merge @cleanup
  Scenario: an OK merge from prdash deletes the item's clean worktree
    Given an item with its review worktree mounted and no uncommitted changes
    When I merge that item from prdash and the merge finishes well
    Then the worktree of that item disappears from disk
    And the notice says both that the merge went well and that the worktree was deleted
    And that item stops having a mounted review

  @tui @merge @cleanup
  Scenario: an OK merge with a dirty worktree keeps it and says so
    Given an item with its review worktree mounted and with uncommitted changes
    When I merge that item from prdash and the merge finishes well
    Then the worktree stays on disk
    And the notice says "merged, but the worktree has uncommitted changes — kept"
    And that item still has its mounted review

  @tui @merge @cleanup
  Scenario: a new untracked file also counts as dirty
    Given an item with its mounted worktree whose only change is an untracked file
    When I merge that item from prdash and the merge finishes well
    Then the worktree is kept (a new file is uncommitted work)
    And the notice says so

  @tui @merge @cleanup
  Scenario: on an unreadable git state the worktree is kept
    Given an item with its mounted worktree whose git state cannot be read
    When I merge that item from prdash and the merge finishes well
    Then the worktree is kept (when in doubt, it is not deleted)
    And the notice says that its state could not be checked

  @tui @merge @cleanup
  Scenario: an OK merge without a mounted worktree reports no error
    Given an item with no mounted review worktree
    When I merge that item from prdash and the merge finishes well
    Then the merge finishes the same way
    And no cleanup error is reported

  @tui @merge @cleanup
  Scenario: if the worktree is no longer on disk, it is a no-op without error
    Given an item whose mounted review points at a path that no longer exists
    When I merge that item from prdash and the merge finishes well
    Then the merge finishes the same way
    And no error is reported

  @tui @merge @cleanup
  Scenario: the merge notice keeps all the truths at once
    Given an item with a dirty worktree whose merge goes well but the branch was not deleted
    When I merge that item from prdash
    Then the notice says all of the following at once: merge ok, branch not deleted, and worktree kept
    And no fact is lost by showing only the last one

  @tui @merge @cleanup @negative
  Scenario: approve deletes no worktree
    Given an item with its review worktree mounted
    When I approve that item from prdash
    Then the worktree stays on disk

  @tui @retarget @cleanup @negative
  Scenario: retarget deletes no worktree
    Given an item with its review worktree mounted
    When I change its target branch from prdash
    Then the worktree stays on disk

  @tui @merge @cleanup @negative
  Scenario: a merge that does not go well deletes no worktree
    Given an item with its review worktree mounted
    When I try to merge it and the merge fails, is blocked, conflicts or is not mergeable
    Then the worktree stays on disk
    And no deletion notice appears

  @tui @quit @cleanup @negative
  Scenario: closing the app deletes no worktree
    Given there are mounted review worktrees
    When I close the TUI with "q" or "ctrl+c"
    Then all the worktrees stay on disk
    And no deletion runs on exit

  @tui @merge @cleanup @negative
  Scenario: a PR merged directly on the forge does not trigger the cleanup
    Given an item whose PR was merged outside prdash
    When prdash refreshes the inbox and sees it merged
    Then its worktree is NOT deleted implicitly
    And it stays available to clean up by hand or with --orphans if it became an orphan

  # ═══════════════════ Common guards for the two paths ═══════════════════

  @cli @tui @security
  Scenario: no cleanup path touches a foreign worktree or one outside the root
    Given a foreign worktree and an own worktree located outside the managed root
    When either of the two is deleted by explicit path, by --orphans or by B
    Then neither of the two is deleted
    And only the own one that also lives inside the managed root can be deleted

  @cli @worktrees @security
  Scenario: the managed-root guard applies on both provisionings
    Given an own worktree located outside the managed root
    When I try to delete it by explicit path, with direct git or with the Herdr native provisioning
    Then it is not deleted
    And the rejection is an error that touches nothing, not even with native deletion

  @cli @tui @security
  Scenario: a nonexistent path is rejected without touching anything
    When I ask to delete a path that does not exist, through any path
    Then nothing is deleted
    And no other worktree is created nor modified
