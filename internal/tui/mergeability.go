package tui

import "context"

// A lazy forge can keep answering UNKNOWN; the cap stops a tick from calling it
// forever. A refresh can overwrite a resolved value back to UNKNOWN, so the
// count resets as soon as a real value arrives.
const maxMergeabilityChecks = 3

// GitHub and GitLab compute mergeability lazily: the inbox list can carry
// UNKNOWN, and the arm-time gate would then be blind. The same tick that
// refreshes the selected item's comments resolves it in the background.
func (m *Model) requestMergeability() {
	it, ok := m.selected()
	if !ok || it.Mergeable.Known {
		return
	}
	id := it.ID()
	if m.mergeInFlight[id] || m.mergeAttempts[id] >= maxMergeabilityChecks {
		return
	}
	a := m.byForge[it.Forge]
	if a == nil {
		return
	}
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		return
	}
	m.mergeInFlight[id] = true
	m.mergeAttempts[id]++

	appCtx, events := m.ctx, m.events
	ref, number := it.Ref, it.Number
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, mergeabilityTimeout)
		defer cancel()
		// Warnings leave the item unknown, which the arm-time notice already
		// covers; there is nothing extra to say here.
		fresh, _ := a.ItemState(ctx, ref, number)
		sendEvent(appCtx, events, mergeabilityMsg{id: id, item: fresh})
	}()
}

func (m *Model) applyMergeability(msg mergeabilityMsg) {
	// The result always arrives, so the in-flight entry is pruned here instead
	// of being left to grow for the TUI's lifetime; the attempt count is only
	// needed while the item stays UNKNOWN.
	delete(m.mergeInFlight, msg.id)
	if msg.item.Mergeable.Known {
		delete(m.mergeAttempts, msg.id)
	}
	if msg.item.Number == 0 {
		return
	}
	// ItemState does not know the inbox section; applyItemUpdate keeps it.
	m.applyItemUpdate(msg.item)
}
