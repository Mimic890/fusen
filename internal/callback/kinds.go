package callback

// Callback kinds. Keep them short: every byte counts toward the 64-byte limit.
const (
	KindNoop    = "n"  // inert button, e.g. a page indicator
	KindHome    = "h"  // home screen
	KindReopen  = "o"  // reopen the live screen from a stale message
	KindArchive = "ar" // archived projects list

	KindProject        = "p"  // p:<id> project board, first column
	KindProjectManage  = "pm" // pm:<id> project management screen
	KindProjectNew     = "pn" // prompt for a new project name
	KindProjectRename  = "pr" // pr:<id> prompt for a new name
	KindProjectArchive = "pa" // pa:<id>:<1|0> archive or restore
	KindProjectDelete  = "pd" // pd:<id> ask, pd:<id>:y confirm

	KindBoard = "b" // b:<column id>[:<page>] board in the user's view; page and column apply to the buttons view
	KindView  = "v" // v:<column id>:<b|t|a> switch the board view

	KindColumns      = "cl" // cl:<project id> column settings
	KindColumn       = "c"  // c:<id> column screen
	KindColumnNew    = "cn" // cn:<project id> prompt for a new column name
	KindColumnRename = "cr" // cr:<id> prompt for a new name
	KindColumnMove   = "cm" // cm:<id>:<l|r>
	KindColumnDone   = "cd" // cd:<id>:<1|0> mark as done column or not
	KindColumnDelete = "cx" // cx:<id> ask, cx:<id>:y confirm

	KindTaskNew      = "tn" // tn:<column id> prompt for task titles
	KindTask         = "t"  // t:<id> task card
	KindTaskMove     = "tm" // tm:<id>:<l|r> move to the neighbouring column
	KindTaskEdit     = "te" // te:<id> edit menu
	KindTaskTitle    = "tt" // tt:<id> prompt for a new title
	KindTaskDesc     = "td" // td:<id> prompt, td:<id>:x clear
	KindTaskTags     = "tg" // tg:<id> prompt, tg:<id>:x clear
	KindTaskPriority = "tp" // tp:<id> picker, tp:<id>:<0-3> set
	KindTaskDelete   = "tx" // tx:<id> ask, tx:<id>:y confirm

	KindSettings = "s"  // settings screen
	KindSetLang  = "sl" // sl:<en|ru>
	KindSetView  = "sv" // sv:<b|t|a>
	KindTimezone = "st" // st picker (also awaits typed input), st:<IANA name> set
	KindDigest   = "sd" // sd picker (also awaits typed input), sd:<HHMM|off> set
	KindRemind   = "sr" // sr picker, sr:<minutes> set

	KindChecklist    = "tc" // tc:<task id>[:d] checklist, optionally in delete mode
	KindChecklistAdd = "ta" // ta:<task id> prompt for new items
	KindItemToggle   = "ii" // ii:<item id>
	KindItemDelete   = "id" // id:<item id>
)

// Arguments.
const (
	Confirm    = "y" // confirms a destructive action
	Clear      = "x" // clears a field
	DeleteMode = "d" // checklist delete mode
	Off        = "off"
	DirLeft    = "l"
	DirRight   = "r"
)
