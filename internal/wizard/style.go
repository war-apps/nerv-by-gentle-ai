package wizard

// style formats the wizard's output: section headers bold cyan, question
// labels bold, help and current values grey, warnings yellow. Off, every
// method returns its text unchanged, so piped, scripted and test output
// stays plain.
type style struct{ on bool }

func (st style) wrap(code, text string) string {
	if !st.on || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// heading is a section header ("--- User config ---").
func (st style) heading(text string) string { return st.wrap("1;36", text) }

// subheading is a group header inside a section ("-- git --").
func (st style) subheading(text string) string { return st.wrap("1", text) }

// label is a question's own text.
func (st style) label(text string) string { return st.wrap("1", text) }

// muted is help text and the current or allowed values of a question.
func (st style) muted(text string) string { return st.wrap("90", text) }

// warn is a re-prompt after an invalid answer.
func (st style) warn(text string) string { return st.wrap("33", text) }
