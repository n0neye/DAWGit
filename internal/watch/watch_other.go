//go:build !windows

package watch

import "context"

func watch(ctx context.Context, root string, events chan<- string) error { return ErrUnsupported }
