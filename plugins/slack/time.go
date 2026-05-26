package slack

import "time"

// nowFn is the time source used by the plugin. Tests override it for
// deterministic FetchedAt timestamps.
var nowFn = time.Now
