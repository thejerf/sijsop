package sijsop

import (
	"errors"
	"fmt"
)

// ErrWrongType is returned when using Unmarshal but the wrong type
// is passed in. The ExpectedType and ReceivedType will be the
// JSONType of the types.
type ErrWrongType struct {
	ExpectedType string
	ReceivedType string
}

// Error implements the Error interface.
func (ewt ErrWrongType) Error() string {
	return fmt.Sprintf("incoming type for unmarshaling is wrong; expected %s, got %s", ewt.ExpectedType, ewt.ReceivedType)
}

// ErrClosed is returned when the Handler is currently closed.
var ErrClosed = errors.New("can't send/receive on closed stream")

// ErrUnknownType is returned when the message couldn't be returned by
// ReceiveNext because it's an unregistered type.
type ErrUnknownType struct {
	ReceivedType string
}

// Error implements the Error interface.
func (eut ErrUnknownType) Error() string {
	return fmt.Sprintf("unknown incoming type: %s", eut.ReceivedType)
}

// ErrJSONTooLarge is returned when the encoded JSON of a message is above
// the legal threshold.
type ErrJSONTooLarge struct {
	Size  int
	Limit int
}

func (ejtl ErrJSONTooLarge) Error() string {
	return fmt.Sprintf("incoming JSON packet too large: %d of max %d",
		ejtl.Size, ejtl.Limit)
}

// ErrNoUnmarshalTarget is returned when you attempt to Unmarshal(nil).
var ErrNoUnmarshalTarget = errors.New("can't unmarshal a nil message")

// ErrTypeTooLong is returned if you attempt to register a type that claims
// a name that is more than 255 characters long.
var ErrTypeTooLong = errors.New("sijsop type name too long")

// ErrTypeAlreadyRegistered is returned if you attempt to register a type
// when the SijsopType() has already been registered.
var ErrTypeAlreadyRegistered = errors.New("sijsop type already registered")
