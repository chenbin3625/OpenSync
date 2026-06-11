package service

import "opensync/internal/model"

func panicPublic(msg string) {
	panic(model.PublicError(msg))
}
