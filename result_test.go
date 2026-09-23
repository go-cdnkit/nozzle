package nozzle

import "testing"

func TestStatusDistinguishesExecutionOutcomes(t *testing.T) {
	statuses := []Status{NotAttempted, Accepted, Rejected, Indeterminate}
	seen := make(map[Status]bool, len(statuses))
	for _, status := range statuses {
		if seen[status] {
			t.Fatalf("duplicate status: %d", status)
		}
		seen[status] = true
	}
	if NotAttempted != 0 {
		t.Fatalf("NotAttempted = %d, want zero", NotAttempted)
	}
}
