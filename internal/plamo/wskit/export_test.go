package wskit

import "time"

// NextWait exposes how long the watch waits before its next recheck, so the
// suite can check the spread without timing real connections.
func (w *CallerWatch) NextWait() time.Duration { return w.nextWait() }
