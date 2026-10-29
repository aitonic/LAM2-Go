package app

import "context"

// Keep the database lazy for auth/launcher modes. The migration coordinator
// journals metadata and publication independently and resumes an interrupted
// database publication with the same immutable batch ID.
type usageImport struct{ core *Core }

func (i *usageImport) Stage(ctx context.Context, path, id string) error {
	i.core.dataMu.Lock()
	defer i.core.dataMu.Unlock()
	db, e := i.core.DB(ctx)
	if e != nil {
		return e
	}
	return db.Stage(ctx, path, id)
}
func (i *usageImport) Publish(ctx context.Context, id string) error {
	i.core.dataMu.Lock()
	defer i.core.dataMu.Unlock()
	db, e := i.core.DB(ctx)
	if e != nil {
		return e
	}
	return db.Publish(ctx, id)
}
func (i *usageImport) RollbackImport(ctx context.Context, id string) error {
	i.core.dataMu.Lock()
	defer i.core.dataMu.Unlock()
	db, e := i.core.DB(ctx)
	if e != nil {
		return e
	}
	return db.RollbackImport(ctx, id)
}
