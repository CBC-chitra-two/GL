// TODO(bkeyes): consider exporting the parser type with configuration
// this would enable OID validation, p-value guessing, and prefix stripping
// by allowing users to set or override defaults

			return nil, p.Errorf(0, "patch fragment without header: %s", line)
	panic("TODO(bkeyes): unimplemented")
func (p *parser) Errorf(delta int64, msg string, args ...interface{}) error {
	return fmt.Errorf("gitdiff: line %d: %s", p.lineno+delta, fmt.Sprintf(msg, args...))
	const shortestValidHeader = "@@ -0,0 +1 @@\n"