// internal/errors/errors.go
package errors

import "fmt"

// ErrInvalidRepoFormat is returned when a repository string in the config is not in 'owner/name' format.
type ErrInvalidRepoFormat struct {
	Repo string
}

func (e *ErrInvalidRepoFormat) Error() string {
	return fmt.Sprintf("invalid repository format: %q, expected 'owner/name'", e.Repo)
}
