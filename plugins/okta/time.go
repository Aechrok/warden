package okta

import "time"

// nowFn is the time source used by the plugin. Tests replace it with a fixed
// time so identity fetch timestamps are deterministic.
var nowFn = time.Now
