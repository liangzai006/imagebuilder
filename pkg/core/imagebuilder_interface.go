package core

import "context"

type ImageBuilderAction interface {
	Commit(ctx context.Context, commitId, to string) error
	Push(ctx context.Context, ref, user, pwd string) error
}
