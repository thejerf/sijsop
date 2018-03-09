package sijsop

// this is currently tested only on the happy path; the error handling is
// assumed to be correct for prototyping purposes.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestJSONProtocolHappyPath(t *testing.T) {
	// "in" from the point of view of the JSON protocol

	incoming := []byte{
		1,           // the type is one byte
		65,          // which is capital A,
		0, 0, 0, 13, //payload is 13 bytes
	}
	inJSON := `{"a":1,"b":2}`
	incoming = append(incoming, []byte(inJSON)...)
	in := bytes.NewBuffer(incoming)
	var out bytes.Buffer
	rw := &ReadWriter{in, &out}

	protocol := &Definition{}
	protocol.Register(&TestMessage{})
	sp := protocol.Wrap(rw)

	// verify that we can read the incoming messages
	val, err := sp.ReceiveNext()
	if err != nil {
		t.Fatal("Can't read out of the JSON message stream:", err.Error())
	}
	shouldBe := &TestMessage{1, 2}

	if !reflect.DeepEqual(val, shouldBe) {
		t.Fatal("Incorrect value read out of the JSON protocol")
	}

	_, err = sp.ReceiveNext()
	if err != io.EOF {
		t.Fatal("jsonprotocol doesn't notice the end of the stream")
	}

	_ = sp.Close()

	err1 := sp.Send(shouldBe)
	_, err2 := sp.ReceiveNext()
	err3 := sp.Unmarshal(&TestMessage{})

	if err1 != ErrClosed || err2 != ErrClosed || err3 != ErrClosed {
		t.Fatal("The closed handling is incorrect")
	}

	in = bytes.NewBuffer(incoming)
	rw = &ReadWriter{in, &out}
	sp = protocol.Wrap(rw)

	// now we try to read the same value via Unmarshal. Note lack of
	// registration.
	target := &TestMessage{}
	_ = sp.Unmarshal(target)

	if !reflect.DeepEqual(target, shouldBe) {
		t.Fatal("Unmarshal doesn't seem to work correctly.")
	}

	_ = sp.Send(target)
	if !reflect.DeepEqual(out.Bytes(),
		[]byte{1, 65, 0, 0, 0, 13, 123, 34, 97, 34, 58, 49, 44, 34, 98, 34,
			58, 50, 125}) {
		t.Fatal("Does not send the correct data values")
	}
}

func TestCoverage(t *testing.T) {
	// this at least ensures they don't crash, and removes the noise from
	// the coverage chart
	_ = ErrWrongType{"A", "B"}.Error()
	_ = ErrUnknownType{"A"}.Error()
	_ = ErrJSONTooLarge{1}.Error()
}

func TestRegistrationErrors(t *testing.T) {
	sp := &Definition{}
	_ = sp.Register(&TestMessage{})
	err := sp.Register(&TestMessage{})

	if err != ErrTypeAlreadyRegistered {
		t.Fatal("ErrTypeAlreadyRegistered isn't working.")
	}

	// especially useful to check the off-by-one here.
	msg := &GenericMessage{strings.Repeat("a", 256)}
	err = sp.Register(msg)
	if err != ErrTypeTooLong {
		t.Fatal("Detection of type being too long not working")
	}
}

func TestBadSends(t *testing.T) {
	sp := &Definition{}
	_ = sp.Register(&TestMessage{})
	_ = sp.Register(&BadJSON{})
	_ = sp.Register(&StringMessage{})

	badWriterErr := errors.New("bad writer strikes again")
	bw := &BadWriter{0, badWriterErr}

	writer := sp.Writer(bw)

	err := writer.Send(&TestMessage{})
	if err != badWriterErr {
		t.Fatal("Can write to the badwriter at zero bytes!")
	}

	// This involves sending a long enough message that the buffer will
	// flush during the buf.Write at the end of Send(), resulting in the error case
	// being hit in the middle of the for loop, rather than the Flush at
	// the end triggering it. This is just to figure out how much space the
	// bufio uses by default:
	bufio := bufio.NewWriter(&BadWriter{0, nil})
	size := bufio.Available()
	writer = sp.Writer(&BadWriter{0, badWriterErr})
	err = writer.Send(&StringMessage{strings.Repeat("a", size*2)})
	if err != badWriterErr {
		t.Fatal("can write to bad writers")
	}

	// this checks the JSON marshaling clause
	buf := &bytes.Buffer{}
	writer = sp.Writer(buf)
	err = writer.Send(&BadJSON{})
	if err == nil {
		t.Fatal("can write bad json types without error")
	}

	// check the thresholding
	oldThreshold := threshold
	threshold = 4
	buf = &bytes.Buffer{}
	writer = sp.Writer(buf)
	err = writer.Send(&TestMessage{})
	if !reflect.DeepEqual(ErrJSONTooLarge{13}, err) {
		t.Fatal("Threshold detection doesn't work")
	}
	threshold = oldThreshold
}

func TestOtherErrors(t *testing.T) {
	in := &ReaderCloser{&bytes.Buffer{}, nil}
	out := &WriterCloser{&bytes.Buffer{}, nil}
	rw := &RWWithClose{in, out}

	protocol := &Definition{}
	_ = protocol.Register(&TestMessage{})

	sp := protocol.Wrap(rw)

	err := sp.Unmarshal(nil)
	if err != ErrNoUnmarshalTarget {
		t.Fatal("Can unmarshal into nil")
	}

	err = sp.Close()
	if err != nil {
		t.Fatal("Unexpected error: " + err.Error())
	}

	testErr := errors.New("test")

	in = &ReaderCloser{&bytes.Buffer{}, nil}
	out = &WriterCloser{&bytes.Buffer{}, testErr}
	rw = &RWWithClose{in, out}
	sp = protocol.Wrap(rw)
	err = sp.Close()
	if err != testErr {
		t.Fatal("Did not propagate close errors correctly.")
	}

	in = &ReaderCloser{&bytes.Buffer{}, testErr}
	out = &WriterCloser{&bytes.Buffer{}, nil}
	rw = &RWWithClose{in, out}
	sp = protocol.Wrap(rw)
	err = sp.Close()
	if err != testErr {
		t.Fatal("Did not propagate close errors correctly.")
	}
}

func TestReadErrors(t *testing.T) {
	// this test produces various malformed inputs and ensures that they
	// are handled properly in the reading code

	protocol := &Definition{}
	protocol.Register(&TestMessage{})

	for idx, in := range []io.Reader{
		bytes.NewBuffer([]byte{}),

		// dies in the middle of the type string
		bytes.NewBuffer([]byte{4, 65}),

		// dies in the middle of the length specification
		bytes.NewBuffer([]byte{1, 65, 0, 0, 0}),

		// dies in the middle of the JSON
		bytes.NewBuffer([]byte{1, 65, 0, 0, 0, 13, '{'}),

		// bad type
		bytes.NewBuffer([]byte{1, 66, 0, 0, 0, 0}),

		// bad json
		bytes.NewBuffer([]byte{1, 65, 0, 0, 0, 1, 'T'}),
	} {
		r := protocol.Reader(in)
		_, err := r.ReceiveNext()
		if err == nil {
			t.Fatal(fmt.Sprintf("Can't detect end of stream in test case %d", idx))
		}
	}

	in := bytes.NewBuffer(
		append([]byte{1, 65, 0, 0, 0, 13}, []byte(`{"a":1,"b":2}`)...),
	)
	r := protocol.Reader(in)
	// wrong type
	sm := &StringMessage{}
	err := r.Unmarshal(sm)
	if !reflect.DeepEqual(ErrWrongType{"string", "A"}, err) {
		t.Fatal("Can unmarshal the wrong type")
	}
}

type ReadWriter struct {
	io.Reader
	io.Writer
}

type RWWithClose struct {
	io.ReadCloser
	io.WriteCloser
}

func (rw *RWWithClose) Close() error {
	err1 := rw.ReadCloser.Close()
	err2 := rw.WriteCloser.Close()

	if err1 != nil {
		return err1
	}
	return err2
}

type TestMessage struct {
	A int `json:"a"`
	B int `json:"b"`
}

func (tm *TestMessage) SijsopType() string {
	return "A"
}

func (tm *TestMessage) New() Message {
	return &TestMessage{}
}

type BadJSON struct {
	Channel chan struct{} `json:"channel"`
}

func (b *BadJSON) SijsopType() string {
	return "string"
}

func (b *BadJSON) New() Message {
	return &BadJSON{}
}

type StringMessage struct {
	Message string `json:"message"`
}

func (sm *StringMessage) SijsopType() string {
	return "string"
}

func (sm *StringMessage) New() Message {
	return &StringMessage{}
}

type GenericMessage struct {
	Type string
}

func (gm *GenericMessage) SijsopType() string {
	return gm.Type
}

func (gm *GenericMessage) New() Message {
	return &GenericMessage{gm.Type}
}

type BadWriter struct {
	ErrOnByte int
	Error     error
}

func (bw *BadWriter) Write(b []byte) (int, error) {
	l := len(b)
	bw.ErrOnByte -= l

	if bw.ErrOnByte <= 0 {
		return 0, bw.Error
	}

	return l, nil
}

type WriterCloser struct {
	io.Writer
	error
}

func (wc *WriterCloser) Close() error {
	return wc.error
}

type ReaderCloser struct {
	io.Reader
	error
}

func (rc *ReaderCloser) Close() error {
	return rc.error
}
