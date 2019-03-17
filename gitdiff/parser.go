	"os"
	fileHeaderPrefix = "diff --git "

	devNull = "/dev/null"
	// TODO(bkeyes): parse header line for filename
	// necessary to get the filename for mode changes or add/rm empty files

	for {
		line, err := p.PeekLine()
		if err != nil {
			return err
		}

		end, err := parseGitHeaderLine(f, line)
		if err != nil {
			return p.Errorf("header: %v", err)
		}
		if end {
			break
		}
		p.Line()
	}

	return nil

func parseGitHeaderLine(f *File, line string) (end bool, err error) {
	if line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}

	for _, hdr := range []struct {
		prefix string
		end    bool
		parse  func(*File, string) error
	}{
		{fragmentHeaderPrefix, true, nil},
		{oldFilePrefix, false, parseGitOldName},
		{newFilePrefix, false, parseGitNewName},
		{"old mode ", false, parseGitOldMode},
		{"new mode ", false, parseGitNewMode},
		{"deleted file mode ", false, parseGitDeletedMode},
		{"new file mode ", false, parseGitCreatedMode},
		{"copy from ", false, parseGitCopyFrom},
		{"copy to ", false, parseGitCopyTo},
		{"rename old ", false, parseGitRenameFrom},
		{"rename new ", false, parseGitRenameTo},
		{"rename from ", false, parseGitRenameFrom},
		{"rename to ", false, parseGitRenameTo},
		{"similarity index ", false, parseGitScore},
		{"dissimilarity index ", false, parseGitScore},
		{"index ", false, parseGitIndex},
	} {
		if strings.HasPrefix(line, hdr.prefix) {
			if hdr.parse != nil {
				err = hdr.parse(f, line[len(hdr.prefix):])
			}
			return hdr.end, err
		}
	}

	// unknown line indicates the end of the header
	// this usually happens if the diff is empty
	return true, nil
}

func parseGitOldName(f *File, line string) error {
	name, _, err := parseName(line, '\t')
	if err != nil {
		return err
	}
	if err := verifyName(name, f.OldName, f.IsNew, "old"); err != nil {
		return err
	}
	f.OldName = name
	return nil
}

func parseGitNewName(f *File, line string) error {
	name, _, err := parseName(line, '\t')
	if err != nil {
		return err
	}
	if err := verifyName(name, f.NewName, f.IsDelete, "new"); err != nil {
		return err
	}
	f.NewName = name
	return nil
}

func parseGitOldMode(f *File, line string) (err error) {
	f.OldMode, err = parseMode(line)
	return
}

func parseGitNewMode(f *File, line string) (err error) {
	f.NewMode, err = parseMode(line)
	return
}

func parseGitDeletedMode(f *File, line string) (err error) {
	// TODO(bkeyes): maybe set old name from default?
	f.IsDelete = true
	f.OldMode, err = parseMode(line)
	return
}

func parseGitCreatedMode(f *File, line string) (err error) {
	// TODO(bkeyes): maybe set new name from default?
	f.IsNew = true
	f.NewMode, err = parseMode(line)
	return
}

func parseGitCopyFrom(f *File, line string) (err error) {
	f.IsCopy = true
	f.OldName, _, err = parseName(line, 0)
	return
}

func parseGitCopyTo(f *File, line string) (err error) {
	f.IsCopy = true
	f.NewName, _, err = parseName(line, 0)
	return
}

func parseGitRenameFrom(f *File, line string) (err error) {
	f.IsRename = true
	f.OldName, _, err = parseName(line, 0)
	return
}

func parseGitRenameTo(f *File, line string) (err error) {
	f.IsRename = true
	f.NewName, _, err = parseName(line, 0)
	return
}

func parseGitScore(f *File, line string) error {
	score, err := strconv.ParseInt(line, 10, 32)
	if err != nil {
		nerr := err.(*strconv.NumError)
		return fmt.Errorf("invalid score line: %v", nerr.Err)
	}
	if score <= 100 {
		f.Score = int(score)
	}
	return nil
}

func parseGitIndex(f *File, line string) error {
	panic("TODO(bkeyes): unimplemented")
}

func parseMode(s string) (os.FileMode, error) {
	mode, err := strconv.ParseInt(s, 8, 32)
	if err != nil {
		nerr := err.(*strconv.NumError)
		return os.FileMode(0), fmt.Errorf("invalid mode line: %v", nerr.Err)
	}
	return os.FileMode(mode), nil
}

// parseName extracts a file name from the start of a string and returns the
// name and the index of the first character after the name. If the name is
// unquoted and term is non-0, parsing stops at the first occurance of term.
// Otherwise parsing of unquoted names stops at the first space or tab.
func parseName(s string, term byte) (name string, n int, err error) {
	// TODO(bkeyes): remove double forward slashes in parsed named

	if len(s) > 0 && s[0] == '"' {
		// find matching end quote and then unquote the section
		for n = 1; n < len(s); n++ {
			if s[n] == '"' && s[n-1] != '\\' {
				break
			}
		}
		if n == 1 {
			err = fmt.Errorf("missing name")
			return
		}
		n++
		name, err = strconv.Unquote(s[:n])
		return
	}

	for n = 0; n < len(s); n++ {
		if term > 0 && s[n] == term {
			break
		}
		if term == 0 && (s[n] == ' ' || s[n] == '\t') {
			break
		}
	}
	if n == 0 {
		err = fmt.Errorf("missing name")
	}
	return
}

// verifyName checks parsed names against state set by previous header lines
func verifyName(parsed, existing string, isNull bool, side string) error {
	if existing != "" {
		if isNull {
			return fmt.Errorf("expected %s, got %s", devNull, existing)
		}
		if existing != parsed {
			return fmt.Errorf("inconsistent %s filename", side)
		}
	}
	if isNull && parsed != devNull {
		return fmt.Errorf("expected %s", devNull)
	}
	return nil
}