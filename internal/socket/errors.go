package socket

import "errors"

var errNoPayload = errors.New("socket: packet carries no payload")
