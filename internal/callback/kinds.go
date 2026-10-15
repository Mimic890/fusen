package callback

// Callback kinds. Keep them short: every byte counts toward the 64-byte limit.
const (
	KindHome           = "h"  // home screen
	KindReopen         = "o"  // reopen the live screen from a stale message
	KindArchive        = "ar" // archived projects list
	KindProject        = "p"  // p:<id> project screen
	KindProjectNew     = "pn" // prompt for a new project name
	KindProjectRename  = "pr" // pr:<id> prompt for a new name
	KindProjectArchive = "pa" // pa:<id>:<1|0> archive or restore
	KindProjectDelete  = "pd" // pd:<id> ask, pd:<id>:y confirm
)

// Confirm is the argument that confirms a destructive action.
const Confirm = "y"
