package main

// colorOutput reports whether the wizard styles its output: only when
// opts.EnableColor says stdout is a terminal that shows ANSI colors, and
// never with NO_COLOR set (https://no-color.org) or TERM=dumb.
func colorOutput(opts options) bool {
	if opts.EnableColor == nil {
		return false
	}
	if opts.Getenv != nil && (opts.Getenv("NO_COLOR") != "" || opts.Getenv("TERM") == "dumb") {
		return false
	}
	return opts.EnableColor()
}
