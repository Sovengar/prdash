# prdash — expected behavior: selectable and toggleable path prefix.
#
# The UNIQUE source of the expected behavior of this feature. It is not Cucumber
# (no step definitions nor runner). The executor will derive real tests from
# here (unit/integration) in BDD/ATDD style.
#
# Gherkin in English; the descriptions are in English too.
#
# Design context: ADR 0002 split the ITEM path into a common prefix (on a fixed
# line, since ADR 0004) and a per-cell suffix. ADR 0004 left open the seam of
# being able to toggle it. This feature closes it with THREE modes cycled by one
# key, global and not persisted.
#
# About the hint: the shortcuts bar is derived from `hintOrder` (config), but the
# label of `p` has to name the CURRENT mode, which is state of the TUI. The seam
# chosen is that `Config.Hints()` receives that state: `hintOrder` keeps being
# the source of list, order and default label, and the rebind of
# `[keybindings]` keeps being generic. These scenarios pin that contract.

Feature: Selectable and toggleable path prefix in the ITEM column

  Background:
    Given the active section "Assigned" has items from the repos "APPCITTI/vsocial/backend/api-gateway#100" and "APPCITTI/vsocial/web-app#101"
    And I run "prdash" inside a TUI
    And the prefix mode is "common" (the default value)

  # ─────────────────────────── Mode cycle ───────────────────────────

  @inbox @prefix @keybindings
  Scenario: p cycles common → full → leaf → common
    Given the prefix mode is "common"
    When I press "p"
    Then the prefix mode becomes "full"
    When I press "p"
    Then the prefix mode becomes "leaf"
    When I press "p"
    Then the prefix mode goes back to "common"
    And the cycle repeats indefinitely in that order

  @inbox @prefix @keybindings
  Scenario: The cycle key comes from the config, not from a hardcoded "p"
    Given the config rebinds "prefix-mode" to another key
    When I press that key
    Then the prefix mode cycles the same way
    And "p" stops changing the prefix mode

  @inbox @prefix @keybindings
  Scenario: p does not trigger any other action
    Given there is a selected item and the TUI is in its normal state
    When I press "p"
    Then nothing is approved, nothing is merged, no review is mounted, nothing is refreshed and nothing is quit
    And the merge is not armed

  @inbox @prefix @navigation
  Scenario: The mode is global and the section change does not reset it
    Given the prefix mode is "leaf"
    When I press "tab" up to "Mine" and back to "Assigned"
    Then the prefix mode is still "leaf"
    And the cursor and the scroll of each section are still remembered

  @inbox @prefix @persistence
  Scenario: The mode is not persisted between runs
    Given I choose the mode "full" with "p"
    When I close the TUI and open it again
    Then the prefix mode is "common"

  # ────────────────────────────── Mode common ──────────────────────────────

  @inbox @prefix
  Scenario: common shows the common prefix on its line and the suffix in the cell
    Given the prefix mode is "common"
    When the list is painted
    Then a dimmed line at the start of the body declares the common prefix "APPCITTI/vsocial/"
    And the ITEM cells show only the suffix ("backend/api-gateway#100"), without the prefix
    And the prefix appears only once in the view

  @inbox @prefix
  Scenario: common is the behavior as always, unchanged with respect to today
    Given the prefix mode is "common"
    When the list is painted with any set of items
    Then the view is identical to the one before this feature

  # ─────────────────────────────── Mode full ───────────────────────────────

  @inbox @prefix
  Scenario: full does not paint a prefix line and puts the full reference in the cell
    Given the prefix mode is "full"
    And the active section has the items "acme/one#7" and "other/two#8", whose references fit
    When the list is painted
    Then there is no prefix line anywhere in the body of the list
    And every ITEM cell carries the full reference, untruncated: "acme/one#7" and "other/two#8"

  @inbox @prefix @truncation
  Scenario: In full, what does not fit is truncated from the tail and the #number survives
    Given the prefix mode is "full"
    And the active section is the one from Background, with "APPCITTI/vsocial/backend/api-gateway#100"
    When the list is painted
    Then the ITEM cell does not fit whole and is truncated from the left with "…"
    And the cell is "…/vsocial/backend/api-gateway#100": it ends in "#100" and keeps the project leaf and the number

  @inbox @prefix @layout
  Scenario: full recovers the height line the prefix used to take
    Given the prefix mode is "full"
    And the active section has a common prefix
    When the list is painted
    Then the body of the list has one line less than in mode "common"
    And the inbox box does not shrink: it keeps taking the reserved height

  # ─────────────────────────────── Mode leaf ───────────────────────────────

  @inbox @prefix
  Scenario: leaf does not paint a prefix line and puts only the project leaf
    Given the prefix mode is "leaf"
    When the list is painted
    Then there is no prefix line anywhere in the body of the list
    And every ITEM cell carries the last segment of the project plus "#n" ("api-gateway#100", "web-app#101")

  @inbox @prefix @layout
  Scenario: leaf gives the maximum density of the ITEM column
    Given the prefix mode is "leaf"
    When the list is painted
    Then the ITEM column is narrower than in "full"
    And it still reserves at least the minimum width so it does not stick to the neighboring column

  @inbox @prefix
  Scenario: leaf cannot disambiguate two repos with the same leaf
    Given the prefix mode is "leaf"
    And the active section has the items "acme/one#7" and "other/one#8", which do not share a prefix
    When the list is painted
    Then the two ITEM cells show "one#7" and "one#8"
    And the detail and --print keep showing the full path of each one

  # ────────────────────────── Width recomputation ──────────────────────────

  @inbox @prefix @layout
  Scenario: The ITEM width is recomputed in each mode
    Given the prefix mode is "common"
    When the list is painted
    Then the ITEM column measures the longest suffix + gap (24 for the Background fixture)
    When I change the mode to "full"
    Then the ITEM column measures the full reference + gap, bounded to the cap (34)
    When I change the mode to "leaf"
    Then the ITEM column measures the longest leaf + gap (16)
    And in the three modes the width stays within [6, 34]

  @inbox @prefix @layout
  Scenario: The width is computed once per render and all rows match
    Given the prefix mode is "leaf"
    When the list is painted
    Then all rows.Items start at the same column
    And the column header and the rows share the same grid
    And the table does not dance when typing on top

  @inbox @prefix @layout
  Scenario: Changing mode does not leave the cursor outside the window
    Given the prefix mode is "common"
    And the list is scrolled and the cursor is on a visible row
    When I press "p" to go to "full"
    Then the cursor is still on the same item and still visible in the window

  @inbox @prefix @layout
  Scenario: An empty item or an empty project do not break the width
    Given the prefix mode is "leaf"
    And the active section includes an item with an empty project
    When the list is painted
    Then the cell of that item is only "#<number>"
    And the rest of the table does not go out of alignment

  # ───────────────────── Degradation without common prefix ─────────────────────

  @inbox @prefix @degradation
  Scenario: common degrades to full if the section has no common prefix
    Given the prefix mode is "common"
    And the active section has a single item, or items that do not share a directory
    When the list is painted
    Then no prefix line is shown anywhere
    And every ITEM cell carries the full path, truncated from the tail if it does not fit
    And mode "common" looks exactly the same as mode "full"

  @inbox @prefix @degradation
  Scenario: The degradation does not invent a prefix nor repeat the path
    Given the prefix mode is "common"
    And the active section has the items "acme/one#7" and "other/one#8"
    When the list is painted
    Then the common prefix is not painted anywhere
    And the full path does not appear neither on the prefix line nor duplicated in the cell

  # ────────────────────────── Hint of the current mode ──────────────────────────

  @inbox @prefix @hints
  Scenario: The hint of p names the current mode, not only the key
    Given the prefix mode is "common"
    When the shortcuts bar is painted
    Then it shows "p prefix: common"
    And it does not show the bare key ("p prefix")
    When I press "p" to go to "full"
    Then the bar shows "p prefix: full"
    When I press "p" to go to "leaf"
    Then the bar shows "p prefix: leaf"

  @inbox @prefix @hints @keybindings
  Scenario: The hint of the mode follows the key rebind
    Given the config rebinds "prefix-mode" to "P"
    And the prefix mode is "leaf"
    When the shortcuts bar is painted
    Then it shows "P prefix: leaf"

  @inbox @prefix @hints
  Scenario: Every registered keybind comes out on the bar
    Given the default config
    When the shortcuts bar is painted
    Then one entry comes out for each action of [keybindings], including "prefix-mode"

  @inbox @prefix @hints
  Scenario: With the merge armed the Confirmation replaces the bar
    Given the prefix mode is "leaf"
    And the merge is armed
    When the shortcuts bar is painted
    Then the merge Confirmation is seen and not the shortcuts bar

  # ─────────────────────── No-regression of other modes ───────────────────────

  @inbox @prefix @print
  Scenario: The --print mode does not change
    When I run "prdash --print"
    Then it prints the full path "project/subgroup#number" of each item
    And its three sections with their long names ("Created by me", "Review / assigned", "Mentions")
    And it applies no prefix mode nor any prefix line

  @inbox @prefix
  Scenario: The detail and the legend keep showing the full path
    Given the prefix mode is "leaf"
    When an item is selected and its card is looked at
    Then the title of the detail box is the full reference
    And the count legend on the border does not change format
