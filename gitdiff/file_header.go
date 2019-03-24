package gitdiff

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

func (p *parser) ParseGitFileHeader(f *File, header string) error {
	header = strings.TrimPrefix(header, fileHeaderPrefix)
	defaultName, err := parseGitHeaderName(header)
	if err != nil {
		return p.Errorf(0, "git file header: %v", err)
	}

	for {
		line, err := p.PeekLine()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		end, err := parseGitHeaderData(f, line, defaultName)
		if err != nil {
			return p.Errorf(1, "git file header: %v", err)
		}
		if end {
			break
		}
		p.Line()
	}

	if f.OldName == "" && f.NewName == "" {
		if defaultName == "" {
			return p.Errorf(0, "git file header: missing filename information")
		}
		f.OldName = defaultName
		f.NewName = defaultName
	}

	if (f.NewName == "" && !f.IsDelete) || (f.OldName == "" && !f.IsNew) {
		return p.Errorf(0, "git file header: missing filename information")
	}

	return nil
}

func (p *parser) ParseTraditionalFileHeader(f *File, oldLine, newLine string) error {
	oldName, _, err := parseName(strings.TrimPrefix(oldLine, oldFilePrefix), '\t', 0)
	if err != nil {
		return p.Errorf(-1, "file header: %v", err)
	}

	newName, _, err := parseName(strings.TrimPrefix(newLine, newFilePrefix), '\t', 0)
	if err != nil {
		return p.Errorf(0, "file header: %v", err)
	}

	switch {
	case oldName == devNull || hasEpochTimestamp(oldLine):
		f.IsNew = true
		f.NewName = newName
	case newName == devNull || hasEpochTimestamp(newLine):
		f.IsDelete = true
		f.OldName = oldName
	default:
		// if old name is a prefix of new name, use that instead
		// this avoids picking variants like "file.bak" or "file~"
		if strings.HasPrefix(newName, oldName) {
			f.OldName = oldName
			f.NewName = oldName
		} else {
			f.OldName = newName
			f.NewName = newName
		}
	}
	return nil
}

// parseGitHeaderName extracts a default file name from the Git file header
// line. This is required for mode-only changes and creation/deletion of empty
// files. Other types of patch include the file name(s) in the header data.
// If the names in the header do not match because the patch is a rename,
// return an empty default name.
func parseGitHeaderName(header string) (string, error) {
	firstName, n, err := parseName(header, -1, 1)
	if err != nil {
		return "", err
	}

	if n < len(header) && (header[n] == ' ' || header[n] == '\t') {
		n++
	}

	secondName, _, err := parseName(header[n:], -1, 1)
	if err != nil {
		return "", err
	}

	if firstName != secondName {
		return "", nil
	}
	return firstName, nil
}

// parseGitHeaderData parses a single line of metadata from a Git file header.
// It returns true when header parsing is complete; in that case, line was the
// first line of non-header content.
func parseGitHeaderData(f *File, line, defaultName string) (end bool, err error) {
	if line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}

	for _, hdr := range []struct {
		prefix string
		end    bool
		parse  func(*File, string, string) error
	}{
		{fragmentHeaderPrefix, true, nil},
		{oldFilePrefix, false, parseGitHeaderOldName},
		{newFilePrefix, false, parseGitHeaderNewName},
		{"old mode ", false, parseGitHeaderOldMode},
		{"new mode ", false, parseGitHeaderNewMode},
		{"deleted file mode ", false, parseGitHeaderDeletedMode},
		{"new file mode ", false, parseGitHeaderCreatedMode},
		{"copy from ", false, parseGitHeaderCopyFrom},
		{"copy to ", false, parseGitHeaderCopyTo},
		{"rename old ", false, parseGitHeaderRenameFrom},
		{"rename new ", false, parseGitHeaderRenameTo},
		{"rename from ", false, parseGitHeaderRenameFrom},
		{"rename to ", false, parseGitHeaderRenameTo},
		{"similarity index ", false, parseGitHeaderScore},
		{"dissimilarity index ", false, parseGitHeaderScore},
		{"index ", false, parseGitHeaderIndex},
	} {
		if strings.HasPrefix(line, hdr.prefix) {
			if hdr.parse != nil {
				err = hdr.parse(f, line[len(hdr.prefix):], defaultName)
			}
			return hdr.end, err
		}
	}

	// unknown line indicates the end of the header
	// this usually happens if the diff is empty
	return true, nil
}

func parseGitHeaderOldName(f *File, line, defaultName string) error {
	name, _, err := parseName(line, '\t', 1)
	if err != nil {
		return err
	}
	if f.OldName == "" && !f.IsNew {
		f.OldName = name
		return nil
	}
	return verifyGitHeaderName(name, f.OldName, f.IsNew, "old")
}

func parseGitHeaderNewName(f *File, line, defaultName string) error {
	name, _, err := parseName(line, '\t', 1)
	if err != nil {
		return err
	}
	if f.NewName == "" && !f.IsDelete {
		f.NewName = name
		return nil
	}
	return verifyGitHeaderName(name, f.NewName, f.IsDelete, "new")
}

func parseGitHeaderOldMode(f *File, line, defaultName string) (err error) {
	f.OldMode, err = parseMode(line)
	return
}

func parseGitHeaderNewMode(f *File, line, defaultName string) (err error) {
	f.NewMode, err = parseMode(line)
	return
}

func parseGitHeaderDeletedMode(f *File, line, defaultName string) error {
	f.IsDelete = true
	f.OldName = defaultName
	return parseGitHeaderOldMode(f, line, defaultName)
}

func parseGitHeaderCreatedMode(f *File, line, defaultName string) error {
	f.IsNew = true
	f.NewName = defaultName
	return parseGitHeaderNewMode(f, line, defaultName)
}

func parseGitHeaderCopyFrom(f *File, line, defaultName string) (err error) {
	f.IsCopy = true
	f.OldName, _, err = parseName(line, -1, 0)
	return
}

func parseGitHeaderCopyTo(f *File, line, defaultName string) (err error) {
	f.IsCopy = true
	f.NewName, _, err = parseName(line, -1, 0)
	return
}

func parseGitHeaderRenameFrom(f *File, line, defaultName string) (err error) {
	f.IsRename = true
	f.OldName, _, err = parseName(line, -1, 0)
	return
}

func parseGitHeaderRenameTo(f *File, line, defaultName string) (err error) {
	f.IsRename = true
	f.NewName, _, err = parseName(line, -1, 0)
	return
}

func parseGitHeaderScore(f *File, line, defaultName string) error {
	score, err := strconv.ParseInt(strings.TrimSuffix(line, "%"), 10, 32)
	if err != nil {
		nerr := err.(*strconv.NumError)
		return fmt.Errorf("invalid score line: %v", nerr.Err)
	}
	if score <= 100 {
		f.Score = int(score)
	}
	return nil
}

func parseGitHeaderIndex(f *File, line, defaultName string) error {
	const sep = ".."

	// note that git stops parsing if the OIDs are too long to be valid
	// checking this requires knowing if the repository uses SHA1 or SHA256
	// hashes, which we don't know, so we just skip that check

	parts := strings.SplitN(line, " ", 2)
	oids := strings.SplitN(parts[0], sep, 2)

	if len(oids) < 2 {
		return fmt.Errorf("invalid index line: missing %q", sep)
	}
	f.OldOIDPrefix, f.NewOIDPrefix = oids[0], oids[1]

	if len(parts) > 1 {
		return parseGitHeaderOldMode(f, parts[1], defaultName)
	}
	return nil
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
// unquoted and term is non-negative, parsing stops at the first occurance of
// term. Otherwise parsing of unquoted names stops at the first space or tab.
//
// If the name is exactly "/dev/null", no further processing occurs. Otherwise,
// if dropPrefix is greater than zero, that number of prefix components
// separated by forward slashes are dropped from the name and any duplicate
// slashes are collapsed.
func parseName(s string, term rune, dropPrefix int) (name string, n int, err error) {
	if len(s) > 0 && s[0] == '"' {
		name, n, err = parseQuotedName(s)
	} else {
		name, n, err = parseUnquotedName(s, term)
	}
	if err != nil {
		return "", 0, err
	}
	if name == devNull {
		return name, n, nil
	}
	return cleanName(name, dropPrefix), n, nil
}

func parseQuotedName(s string) (name string, n int, err error) {
	for n = 1; n < len(s); n++ {
		if s[n] == '"' && s[n-1] != '\\' {
			n++
			break
		}
	}
	if n == 2 {
		return "", 0, fmt.Errorf("missing name")
	}
	if name, err = strconv.Unquote(s[:n]); err != nil {
		return "", 0, err
	}
	return name, n, err
}

func parseUnquotedName(s string, term rune) (name string, n int, err error) {
	for n = 0; n < len(s); n++ {
		if s[n] == '\n' {
			break
		}
		if term >= 0 && rune(s[n]) == term {
			break
		}
		if term < 0 && (s[n] == ' ' || s[n] == '\t') {
			break
		}
	}
	if n == 0 {
		return "", 0, fmt.Errorf("missing name")
	}
	return s[:n], n, nil
}

// verifyGitHeaderName checks a parsed name against state set by previous lines
func verifyGitHeaderName(parsed, existing string, isNull bool, side string) error {
	if existing != "" {
		if isNull {
			return fmt.Errorf("expected %s, but filename is set to %s", devNull, existing)
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

// cleanName removes double slashes and drops prefix segments.
func cleanName(name string, drop int) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] == '/' {
			if i < len(name)-1 && name[i+1] == '/' {
				continue
			}
			if drop > 0 {
				drop--
				b.Reset()
				continue
			}
		}
		b.WriteByte(name[i])
	}
	return b.String()
}

// hasEpochTimestamp returns true if the string ends with a POSIX-formatted
// timestamp for the UNIX epoch after a tab character. According to git, this
// is used by GNU diff to mark creations and deletions.
func hasEpochTimestamp(s string) bool {
	const posixTimeLayout = "2006-01-02 15:04:05.9 -0700"

	start := strings.IndexRune(s, '\t')
	if start < 0 {
		return false
	}

	ts := strings.TrimSuffix(s[start+1:], "\n")

	// a valid timestamp can have optional ':' in zone specifier
	// remove that if it exists so we have a single format
	if ts[len(ts)-3] == ':' {
		ts = ts[:len(ts)-3] + ts[len(ts)-2:]
	}

	t, err := time.Parse(posixTimeLayout, ts)
	if err != nil {
		return false
	}
	if !t.Equal(time.Unix(0, 0)) {
		return false
	}
	return true
}