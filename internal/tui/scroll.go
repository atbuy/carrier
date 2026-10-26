package tui

// scrollTo returns the first of total rows to show, rows at a time, so the
// cursor stays in view. The window only moves when the cursor would leave it,
// so moving back up a long list moves the cursor, not the list.
func scrollTo(offset, cursor, rows, total int) int {
	if rows <= 0 {
		return 0
	}
	switch {
	case cursor < offset:
		offset = cursor
	case cursor >= offset+rows:
		offset = cursor - rows + 1
	}
	return max(0, min(offset, total-rows))
}
