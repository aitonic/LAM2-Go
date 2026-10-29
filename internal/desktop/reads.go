package desktop

import "context"

// Read navigation is independent of a mutation job. New navigation cancels the
// preceding read and ignores late results instead of leaving the new page empty.
func (u *UI) readTask(work func(context.Context) (any, error), done func(any)) {
	if u.win == nil || u.core == nil || u.closed.Load() {
		return
	}
	if u.readCancel != nil {
		u.readCancel()
	}
	u.readGeneration++
	generation := u.readGeneration
	ctx, cancel := context.WithCancel(u.ctx)
	u.readCancel = cancel
	u.reading = true
	win := u.win
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		defer cancel()
		value, e := work(ctx)
		if ctx.Err() != nil || u.closed.Load() {
			return
		}
		win.Update(func() {
			if u.closed.Load() || u.readGeneration != generation {
				return
			}
			u.reading = false
			u.readCancel = nil
			if e != nil {
				u.fail(e)
				return
			}
			done(value)
			if u.trayWin != nil {
				u.trayWin.Invalidate()
			}
		})
	}()
}
