package service

import "opensync/internal/model"

func panicPublic(msg string) {
	panic(model.PublicError(msg))
}

// panicPublicIf panics with msg as a user-facing error when err's text matches
// one of the known public messages; otherwise it panics the raw error, which
// the recovery middleware masks as a generic 500. Use it where a mapper error
// is either a recognizable "not found"/conflict condition or an opaque DB
// failure, so recoverable cases reach the user with an actionable message.
func panicPublicIf(err error, publicMessages ...string) {
	if err == nil {
		return
	}
	for _, m := range publicMessages {
		if err.Error() == m {
			panicPublic(m)
		}
	}
	panic(err.Error())
}
