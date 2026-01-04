package bridge

import "errors"

// ErrInvalidMailConfig is returned when a Namecoin name value cannot be parsed
// as valid mail configuration JSON or is missing required fields.
var ErrInvalidMailConfig = errors.New("invalid mail configuration")
