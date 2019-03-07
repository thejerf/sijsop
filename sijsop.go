/*

Package sijsop provides a SImple JSOn Protocol that is easily parsed
by numerous languages, guided by experience in multiple languages.

This protocol has one of the best bang-for-the-buck ratios you can find
for any protocol. It is so easy to implement that you can easily implement
this for another language, yet as powerful as the JSON processing in your
language. There's better protocols on every other dimension, for
compactness, efficiency, safety, etc., but when you just want to bash
out a line protocol quickly and those other concerns aren't all that
pressing, this is a pretty decent choice. It also has the advantage
of not committing you very hard to a particular implementation, because
any network engineer can bang out an implementation of this in another
language in a couple of hours.

Usage In Go

This protocol is defined in terms of messages, which must implement
sijsop.Message. This involves writing two static class methods, one
which declares a string which uniquely identifies this
type for a given Definition, and one which simply returns a new instance
of the given type. These must be defined on the pointer receiver for the
type, because we must be able to modify the values for this code to work.

Once you have a Definition, you can use it to wrap a Reader, which
will permit you to receive messages, a Writer, which will permit
you to send them, or a ReadWriter, which allows for bidirectional
communication.

sijsop is careful to never read more than the next message, so it is
quite legal using this protocol to send some message that indicates
some concrete length, then use the underlying Reader/Writer/ReadWriter
directly, then smoothly resume using the sijsop protocol. See the
example file shown below.

The Wire Protocol

The protocol is a message-based protocol which works as follows:

 * An ASCII number ("1", "342", etc.), followed by a newline (byte 10).
 * That many bytes of arbitrary string indicating the type of the next
   message, the "type tag", followed by a newline. (The newline is
   NOT included in the first number.)
 * An ASCII number ("1", "342", etc.), followed by a newline (byte 10).
 * That much JSON, followed by a newline. (Again, the newline is
   NOT included in the length count, but it must be present.)

Commentary: I've often used variants of this protocol that used just the
JSON portion, but I found myself repeatedly having to parse the JSON
once for the type, and then again with the right type. Giving
statically-typed languages a chance to pick the type out in advance
helps with efficiency. Static languages can just ignore than field
if they like.

Everything else is left up to the next layer. Protocols that are rigid
about message order are easy. Protocols that need to match requests to
responses are responsible for their own next-higher-level mapping.

*/
package sijsop

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
)

const (
	DefaultSizeLimit = 2 << 30
)

// A Definition defines a specific protocol, with specific messages.
//
// Types should be registered with the definition before use; once a
// Reader, Writer or Handler has been created from the Definition it is no
// longer safe to register more types.
type Definition struct {
	types map[string]Message
}

// Register registers the given messages with given Definition.
//
// The last registration for a given .SijsopType() will be the one that is
// used.
func (d *Definition) Register(types ...Message) {
	if d.types == nil {
		d.types = map[string]Message{}
	}

	for _, t := range types {
		ty := t.SijsopType()
		d.types[ty] = t
	}
}

// Wrap creates a new Handler around the given io.ReadWriter that
// implements the protocol given by the Definition.
func (d *Definition) Wrap(rw io.ReadWriter) *Handler {
	return &Handler{Reader: d.Reader(rw), Writer: d.Writer(rw)}
}

// Reader creates a new Reader around the given io.Reader that implements
// the protocol given by the Definition.
func (d *Definition) Reader(r io.Reader) *Reader {
	return &Reader{def: d, in: r}
}

// Writer creates a new Writer around the given io.Writer that implements
// the protocol given by the Definition.
func (d *Definition) Writer(w io.Writer) *Writer {
	return &Writer{def: d, out: w}
}

// A Reader implements the protocol from the Definition used to create it.
//
// If the io.Reader implements io.Closer, it will be closed if you call
// .Close on this object.
type Reader struct {
	def    *Definition
	in     io.Reader
	closed bool
}

// A Writer implements the protocol from the Definition used to create it.
//
// If the io.Writer implements io.Closer, it will be closed when this
// object is closed.
//
// SizeLimit can be set after construction to set a limit on how big a
// message may be on the way out. If -1, there is no limit. If 0, the
// default of DefaultSizeLimit is used, of a gigabyte. If a number,
// anything that size or greater will instead error out.
//
// Prefix and Indent will be passed to SetIndent on the encoder if either
// is non-empty. This can be convenient during debugging to make the
// messages more readable.
type Writer struct {
	SizeLimit int
	def       *Definition
	out       io.Writer
	closed    bool

	Prefix string
	Indent string
}

// A Handler composes a Reader and a Writer into a single object.
type Handler struct {
	*Reader
	*Writer
}

// Message describes messages that can be registered with a Definition, and
// subsequently sent or recieved.
//
// SijsopType should be a unique string for a given Definition. It MUST be
// a constant string, or sijsop does not guarantee correct functioning.
//
// New should return an empty instance of the same struct, for use in the
// unmarshaling. It MUST be the same as what is called, or sijsop does not
// guarantee correct functioning.
type Message interface {
	SijsopType() string

	// Returns a new zero instance of the struct in question.
	New() Message
}

// Send sends the given JSON message.
//
// This method uses a buffer to generate the message. If you are sending
// multiple messages, it is somewhat more efficient to send them all as
// parameters to one Send call, so the same buffer can be used for all of
// them.
//
// If an error is returned, the stream is now in an unknown condition and
// can not be trusted for further use.
func (w *Writer) Send(msgs ...Message) error {
	if w.closed {
		return ErrClosed
	}

	buf := bufio.NewWriter(w.out)
	for _, msg := range msgs {
		ty := msg.SijsopType()

		l := strconv.Itoa(len(ty))

		// These are fairly unlikely to error, because they will generally
		// be eaten by the buffer. There's a slight chance they'll fall
		// across a boundary, but we'll still get the error at the ending
		// write, just a bit less efficiently.
		_, _ = buf.Write([]byte(l))
		_, _ = buf.Write([]byte{10})
		_, _ = buf.Write([]byte(ty))
		_, _ = buf.Write([]byte{10})

		jsonBuf := &bytes.Buffer{}
		encoder := json.NewEncoder(jsonBuf)
		if w.Prefix != "" || w.Indent != "" {
			encoder.SetIndent(w.Prefix, w.Indent)
		}
		err := encoder.Encode(msg)
		if err != nil {
			return err
		}

		json := jsonBuf.Bytes()
		jsonLen := len(json) - 1

		switch {
		case w.SizeLimit == 0:
			if jsonLen >= DefaultSizeLimit {
				return ErrJSONTooLarge{jsonLen, DefaultSizeLimit}
			}
		case w.SizeLimit > 0:
			if jsonLen >= w.SizeLimit {
				return ErrJSONTooLarge{jsonLen, w.SizeLimit}
			}
		default:
			// deliberately do nothing
		}

		_, _ = buf.Write([]byte(strconv.Itoa(jsonLen)))
		_, _ = buf.Write([]byte{10})
		_, err = buf.Write(json)
		// this is the first place we might realistically encounter an error
		if err != nil {
			return err
		}
	}
	return buf.Flush()
}

// Unmarshal takes the given object and attempts to unmarshal the next JSON
// message into that object. If the types do not match, an ErrWrongType
// will be returned.
//
// This allows you to receive a concrete type directly when you know what
// the type will be.
//
// If an error is returned, the stream is now in an unstable condition.
func (r *Reader) Unmarshal(msg Message) error {
	if msg == nil {
		return ErrNoUnmarshalTarget
	}

	_, err := r.receiveNext(msg)
	return err
}

// Close closes this reader, which means it can no longer be used to receive
// messages. If the underlying io.Reader implements io.Closer, the
// io.Reader will have Close called on it as well.
func (r *Reader) Close() error {
	r.closed = true

	closer, isCloser := r.in.(io.Closer)
	if isCloser {
		return closer.Close()
	}
	return nil
}

// Close closes this writer, which means it can no longer be used to send
// message. If the underlying io.Writer implements io.Closer, the io.Writer
// will have Close called on it as well.
func (w *Writer) Close() error {
	w.closed = true

	closer, isCloser := w.out.(io.Closer)
	if isCloser {
		return closer.Close()
	}
	return nil
}

// Close will close the Handler, making it impossible to send or receive
// any more messages.
//
// This is implemented by calling Close on the Reader and the Writer
// component, which will result in the underlying stream being closed twice.
func (jp *Handler) Close() error {
	e1 := jp.Reader.Close()
	e2 := jp.Writer.Close()

	if e1 != nil {
		return e1
	}
	return e2
}

// ReceiveNext will receive the next message from the Handler.
//
// If an error is received, the stream is in an unstable condition.
func (r *Reader) ReceiveNext() (Message, error) {
	return r.receiveNext(nil)
}

func (r *Reader) receiveNext(target Message) (Message, error) {
	if r.closed {
		return nil, ErrClosed
	}

	length, err := getAsciiNum(r.in)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, int(length))
	_, err = io.ReadFull(r.in, buf)
	if err != nil {
		return nil, err
	}
	ty := string(buf)
	// eat the newline after the type
	newlineBuf := make([]byte, 1)
	_, _ = io.ReadFull(r.in, newlineBuf)

	jsonLen, err := getAsciiNum(r.in)
	if err != nil {
		return nil, err
	}

	if target == nil {
		targetType, haveTarget := r.def.types[ty]
		if !haveTarget {
			return nil, ErrUnknownType{ty}
		}
		target = targetType.New()
	} else {
		givenType := target.SijsopType()
		if givenType != ty {
			return nil, ErrWrongType{givenType, ty}
		}
	}

	limitedReader := &io.LimitedReader{R: r.in, N: int64(jsonLen)}
	decoder := json.NewDecoder(limitedReader)

	// must be a Message because of the source it comes from
	err = decoder.Decode(target)
	if err != nil {
		return nil, err
	}

	// if the JSON did not completely fill out the declared space, consume
	// the remainder without examining it. This ensures that we advance to
	// the next message properly.
	_, _ = ioutil.ReadAll(limitedReader)
	// now eat the newline after the JSON
	_, err = io.ReadFull(r.in, newlineBuf)
	if err != nil {
		// despite the message having been read out, the stream is still skunked
		return nil, err
	}

	return target, nil
}

func getAsciiNum(r io.Reader) (int, error) {
	accum := []byte{}
	char := make([]byte, 1)
	i := 0

	for {
		i++

		if i > 20 {
			return 0, errors.New("length claims to be absurdly large")
		}

		n, err := r.Read(char)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			// I'm not sure this is possible
			return 0, errors.New("unexpected error")
		}

		if char[0] == 10 {
			return strconv.Atoi(string(accum))
		}
		if char[0] >= '0' && char[0] <= '9' {
			accum = append(accum, char[0])
			continue
		}
		return 0, fmt.Errorf("unexpected byte in length specification: %d",
			char[0])
	}
}
