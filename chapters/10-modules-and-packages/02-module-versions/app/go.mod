module example.com/course/greeting-app

go 1.27

require (
	example.com/course/greeting v1.2.0
	example.com/course/greeting/v2 v2.0.0
)

// These local replacements keep this learning lab independent of network access.
// The version labels above are teaching examples, not published release tags.
replace example.com/course/greeting => ../greeting-v1

replace example.com/course/greeting/v2 => ../greeting-v2
