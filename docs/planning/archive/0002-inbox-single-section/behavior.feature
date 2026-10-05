# prdash — expected behavior: single-section Inbox.
#
# The UNIQUE source of the expected behavior of this feature. It serves for the
# user to confirm that we captured what they want; it is NOT Cucumber (no step
# definitions nor runner). The executor will derive real tests from here
# (unit/integration) in BDD/ATDD style.
#
# Gherkin in English; the descriptions are in English too.
#
# Design context (ADR 0002): the reason for showing a single section at a time
# is to keep and make the most of the common path prefix. With a single section
# visible all rows share a section, so the prefix is computed on the active
# section and is still shown, even though the internal header with title and
# count disappears.

Feature: Single-section inbox with a count legend and common path prefix

  Background:
    Given there are items in the three sections of the inbox (Mine = authored, Assigned = review, Mentioned = mentions)
    And the section labels are "Mine", "Assigned" and "Mentioned"
    And I run "prdash" inside a TUI

  # ─────────────────────── Active section and initial state ───────────────────────

  @inbox @navigation
  Scenario: On opening, the active section is Assigned
    Given I open the TUI
    When the Inbox is painted for the first time
    Then the active section is "Assigned"
    And only the items of Assigned are seen in the list
    And the items of Mine and Mentioned do not appear in the list

  @inbox @navigation
  Scenario: Only one section is painted at a time
    Given the active section is "Assigned"
    When I look at the Inbox
    Then the list contains items of Assigned only
    And there is no internal section header with the title and the count inside the list

  @inbox @navigation
  Scenario: tab cycles Assigned → Mentioned → Mine → Assigned
    Given the active section is "Assigned"
    When I press "tab"
    Then the active section becomes "Mentioned" and the list shows its items
    When I press "tab"
    Then the active section becomes "Mine" and the list shows its items
    When I press "tab"
    Then the active section goes back to "Assigned"
    And the cycle repeats indefinitely in that order

  @inbox @navigation
  Scenario: tab also cycles when a section is empty
    Given the section "Mentioned" has no items
    And the active section is "Assigned"
    When I press "tab"
    Then the active section becomes "Mentioned" anyway
    And the list shows its empty state

  @inbox @keybindings
  Scenario: The cycle key comes from the config, not from a hardcoded "tab"
    Given the config rebinds "section-next" to another key
    When I press that key
    Then the active section changes following the cycle
    And "tab" stops changing section

  # ───────────────────────────── Count legend ─────────────────────────────

  @inbox @legend
  Scenario: The top-left border legend replaces the "Inbox" title
    Given the sections have 9, 4 and 0 items respectively
    When the Inbox is painted
    Then the top-left border of the Inbox box shows "Mine (9) · Assigned (4) · Mentioned (0)"
    And the "Inbox" title is no longer shown

  @inbox @legend
  Scenario: The active section is highlighted in the legend
    Given the active section is "Assigned"
    When the legend is painted
    Then "Assigned" and its count are shown highlighted (color/bold)
    And "Mine" and "Mentioned" are shown dimmed

  @inbox @legend @refresh
  Scenario: The legend counts reflect the deduped section
    Given an item that would appear in more than one section
    When the legend is painted
    Then that item is counted once, in the section with the highest authority
    And each count is the number of items that section has in the list

  # ────────────────────────── Common path prefix (ADR 0002) ──────────────────────────

  @inbox @prefix
  Scenario: The common prefix of the active section is still shown
    Given the active section has several items whose project shares "APPCITTI/vsocial/backend"
    When the list is painted
    Then the common path prefix "APPCITTI/vsocial/backend/" appears only once in the view
    And the ITEM cells of the rows show only the suffix (e.g. "api-gateway#100"), without the prefix

  @inbox @prefix
  Scenario: The prefix shown is the active section's one
    Given "Mine" and "Assigned" have different common path prefixes
    When the active section is "Assigned"
    Then the view shows the common prefix of "Assigned"
    And it does not show the one of "Mine"
    When I press "tab" up to "Mine"
    Then the view switches to show the common prefix of "Mine"

  @inbox @prefix
  Scenario: A section without common prefix does not invent one
    Given the active section has a single item, or its items do not share a directory
    When the list is painted
    Then no common prefix is shown
    And every ITEM cell carries the full path, truncated from the tail if it does not fit

  @inbox @prefix
  Scenario: The prefix does not break the table width
    Given the active section has a common path prefix
    When the list is painted at any terminal width
    Then the prefix is truncated if it does not fit, without misaligning the columns
    And the ITEM cells keep the project leaf and the "#number"

  # ───────────────────────────── Position per section ─────────────────────────────

  @inbox @cursor
  Scenario: Each section remembers its cursor and its scroll
    Given in "Assigned" I move the cursor and scroll the list
    When I press "tab" to "Mentioned" and move its cursor elsewhere
    And I come back with "tab" to "Assigned"
    Then "Assigned" recovers the cursor and the scroll it had
    When I come back to "Mentioned"
    Then "Mentioned" recovers its own cursor and scroll

  @inbox @cursor @refresh
  Scenario: A refresh keeps the position of each section
    Given I have a position in each section
    When an inbox refresh finishes
    Then the active section keeps its cursor and its scroll, bounded to the new content

  # ───────────────────────── States of the active section ─────────────────────────

  @inbox @state
  Scenario: The empty active section is marked as empty
    Given the active section has no items and its query did not fail
    When the list is painted
    Then the list shows "(empty)"
    And the legend shows 0 for that section

  @inbox @pagination
  Scenario: The "loading more…" belongs to the active section
    Given the active section has pending pages
    When the list is painted
    Then the list shows "loading more…"
    And if the section that was paginating is not the active one, its indicator is not shown
    And on returning to that section, the indicator is shown again

  @inbox @degradation
  Scenario: Query warnings are shown for the active section
    Given the query of a section failed or returned partial data
    And that section is the active one
    When the list is painted
    Then the list shows its warning "⚠ <forge>: could not be queried (…)"
    And if that section is not the active one, its warning is not painted (its count stays visible in the legend)

  # ─────────────────────────── No-regression of other modes ───────────────────────────

  @inbox @print
  Scenario: The --print mode does not change
    When I run "prdash --print"
    Then it prints the three sections with their long names ("Created by me", "Review / assigned", "Mentions")
    And each section lists its items in the usual order

  @inbox @legend
  Scenario: On a narrow terminal the legend is truncated without breaking the box
    Given a terminal width smaller than the full legend
    When the Inbox is painted
    Then the legend is cut on the right
    And the top line still measures exactly the box width
