package nozzle

// Status describes what is known about an operation's submission.
type Status uint8

const (
	// NotAttempted means the operation was not handed to the HTTP client.
	NotAttempted Status = iota
	// Accepted means the API confirmed acceptance, not global cache invalidation.
	Accepted
	// Rejected means the API explicitly refused the operation.
	Rejected
	// Indeterminate means submission began without a usable confirmation.
	Indeterminate
)

// OperationResult associates an execution outcome with independently owned targets.
type OperationResult struct {
	Operation  Operation
	Status     Status
	HTTPStatus int
}
